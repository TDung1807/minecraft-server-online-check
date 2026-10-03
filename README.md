# Minecraft Monitor

Backend Go theo dõi `bora.pikamc.vn:25005` qua Minecraft Java TCP status ping. Một poller chung truy vấn mỗi 5 giây; REST đọc RAM và SSE đẩy snapshot. Giao diện theo token trong `DESIGN.md`: nền navy, thẻ pastel, minh họa khối Minecraft, sao chép IP và danh sách người chơi cập nhật trực tiếp. Frontend được nhúng trong binary và deploy riêng lên GitHub Pages, không cần Node hay database. Hiệu ứng tôn trọng `prefers-reduced-motion`.

Kết quả kiểm chứng local, Docker, HTTPS/SSE và bài tải 30 phút: [docs/verification.md](docs/verification.md).

Triển khai backend AWS và frontend GitHub Pages: [docs/aws-deployment.md](docs/aws-deployment.md).

## Chạy local

Yêu cầu Go **1.27.1**.

```sh
go mod download
go run ./cmd/server
```

Mở http://127.0.0.1:8080. Metrics chỉ bind loopback tại http://127.0.0.1:9090/metrics. Nếu cổng bận, đặt `HTTP_ADDR=127.0.0.1:8081` và `METRICS_ADDR=127.0.0.1:9091`.

```sh
curl http://127.0.0.1:8080/api/v1/status
curl -N http://127.0.0.1:8080/api/v1/events
```

## Cấu hình

| Biến | Mặc định | Ý nghĩa |
| --- | --- | --- |
| MC_HOST | bora.pikamc.vn | Host cố định; không nhập URL HTTP |
| MC_PORT | 25005 | TCP port Minecraft |
| POLL_INTERVAL | 5s | Chu kỳ truy vấn, tối thiểu 1s |
| QUERY_TIMEOUT | 3s | Deadline tổng, phải nhỏ hơn chu kỳ |
| STALE_AFTER | 10s | Tuổi tối đa dữ liệu, không nhỏ hơn chu kỳ |
| HTTP_ADDR | 127.0.0.1:8080 | Địa chỉ phục vụ HTTP |
| METRICS_ADDR | 127.0.0.1:9090 | Listener metrics riêng |
| WEB_ORIGIN | rỗng | Rỗng: cùng origin; có giá trị: origin HTTP(S) chính xác |
| TRUST_PROXY | false | Chỉ bật sau proxy tin cậy, peer loopback/private |
| SITE_DOMAIN | không có | Domain HTTPS cho Docker/Caddy |

Ứng dụng không tự đọc `.env` khi chạy `go run`; Docker Compose đọc `.env`. Số online là số server công bố, không xác minh được plugin làm giả hoặc phạm vi cụm server sau proxy. API trả thêm `players` gồm tên và UUID từ `players.sample`; giao diện hiển thị tên và tự cập nhật qua SSE. Server có thể ẩn hoặc chỉ trả một phần danh sách. Danh sách sample không được dùng để đếm và bị xóa khỏi dữ liệu hiện tại khi trạng thái cũ hoặc không truy vấn được. `queryDurationMs` là thời gian truy vấn từ backend, không phải latency của người xem.

## Trạng thái và API

`unknown` khi chưa có lần truy vấn hoàn tất. `online` khi dữ liệu mới và hợp lệ, bao gồm 0 người chơi. Một lỗi chuyển sang `stale`; 3 lỗi liên tiếp chuyển sang `unreachable`. Một lần thành công khôi phục ngay. Trạng thái lỗi không có số hiện tại (`null`), chỉ giữ kết quả cũ trong `lastKnown`. Kiểm tra tuổi dữ liệu mỗi giây. `unreachable` nghĩa là backend không truy vấn được, không khẳng định Minecraft đã tắt.

REST: [docs/openapi.yaml](docs/openapi.yaml). Realtime: [docs/sse.md](docs/sse.md). `/health/ready` chỉ kiểm tra backend/poller, không phụ thuộc Minecraft online. Sequence khởi động lại từ 0 khi tiến trình restart. Không có lưu trữ lịch sử hay API nhập server tùy ý.

## Triển khai VPS

1. Trỏ DNS domain website về VPS, mở inbound 80/443, cho phép outbound DNS và TCP tới Minecraft port 25005. Domain thật cần thiết cho cấp chứng chỉ HTTPS.
2. Tạo `.env` từ `.env.example`, thay `SITE_DOMAIN` bằng domain thật.
3. Chạy `docker compose up -d --build` và xem `docker compose logs -f backend`.
4. Kiểm tra `/health/ready`, REST và `curl -N https://<domain>/api/v1/events`: frame đầu phải đến ngay, heartbeat mỗi 15 giây.

Docker chạy non-root, root filesystem chỉ đọc, giới hạn 256 MiB/1 CPU, log xoay vòng và restart khi tiến trình dừng. Backend 8080 không publish ra host; metrics 9090 chỉ publish vào loopback. Docker healthcheck báo sức khỏe, không tự restart container chỉ vì trạng thái unhealthy. Triển khai một instance; không scale replica trước khi bổ sung poller leader và fan-out chung.

`TRUST_PROXY=true` chỉ dùng trong mạng backend riêng do bạn kiểm soát. Caddy thiết lập forwarding headers; không mở port backend cho người dùng hay container không tin cậy. Metrics không có xác thực và phải giữ nội bộ. Caddy tắt buffering qua `flush_interval -1`; không thêm CDN/cache phía trước SSE nếu chưa kiểm tra streaming.

Để cập nhật, build image mới, chạy test rồi `docker compose up -d --build`. Lưu image trước khi cập nhật để rollback; không có migration database. Reconnect sẽ nhận snapshot mới, không yêu cầu giữ sequence xuyên restart.

## Giám sát

Log JSON mỗi lần truy vấn gồm trạng thái, thời gian và mã lỗi. Backend ghi cảnh báo khi không có kết quả mới 60 giây. Metrics cung cấp tuổi dữ liệu (`-1` nếu chưa thành công), số lỗi liên tiếp, tổng truy vấn/lỗi, số SSE subscriber, goroutine và heap. `deploy/alerts.yml` là rule mẫu cho Prometheus; cấu hình scrape/Alertmanager trong hệ thống giám sát của VPS để gửi thông báo. Rule scrape dùng job `minecraft-monitor`.

## Kiểm thử

```sh
go test -race -count=1 ./...
go vet ./...
go build -trimpath -o bin/minecraft-monitor ./cmd/server
docker build -t minecraft-monitor:local .
SOAK_DURATION=30m go test -race ./internal/httpapi -run '^TestSoak$' -count=1 -timeout=35m -v
```

Test TCP giả lập kiểm tra 0 người, thiếu sample, thiếu trường, JSON lỗi, số âm/phân số, packet quá lớn, string length giả, timeout, đóng socket, cancel và DNS lỗi. Test REST/SSE kiểm tra dữ liệu cũ, phục hồi, giới hạn kết nối, subscriber cleanup và chống giả forwarding header từ peer public. Soak test dùng server Minecraft TCP giả và 100 SSE khác IP, kiểm tra không khuếch đại truy vấn, REST p95 <100ms và heap tăng không quá 8 MiB sau GC. Có thể dùng `SOAK_DURATION=20s` cho smoke test; nó không thay thế bài chạy 30 phút.

Nghiệm thu VPS: cho người chơi vào/ra và kiểm tra website phản ánh trong 8 giây khi mạng bình thường; thử mất mạng để xác nhận không hiện 0. Status ping lấy mẫu mỗi 5 giây có thể bỏ qua lượt vào/ra ngắn. Các kiểm tra VPS/HTTPS, soak 30 phút và vào/ra thực tế phải được thực hiện trước khi công bố vận hành chính thức.
