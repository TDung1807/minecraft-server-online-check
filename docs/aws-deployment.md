# AWS backend + GitHub Pages frontend

Backend được triển khai tại selected Region `ap-southeast-1` (Singapore).
Frontend được build và deploy từ GitHub Actions; AWS chỉ phục vụ API/health.

## Tài nguyên

- EC2: `i-0707ae5159eb45b12`, Amazon Linux 2023 ARM64, `t4g.small`.
- Một ổ đĩa gp3 mã hóa 8 GB, xóa cùng instance.
- IAM instance profile `MinecraftMonitorSSM`: chỉ có `AmazonSSMManagedInstanceCore`.
- Quản lý máy qua Systems Manager; không có SSH public.
- CloudFront VPC origin: `vo_DLrLI3Avl80D1iyveQN4zq`, truy cập máy bằng IP riêng.
- Backend chỉ bind `127.0.0.1:8080`; metrics chỉ bind `127.0.0.1:9090`.
- Nginx proxy API với buffering/cache tắt, giữ heartbeat SSE mỗi 15 giây.
- Frontend origin: `https://tdung1807.github.io`.
- Access logs CloudFront: bucket riêng `minecraft-monitor-logs-889629667836`
  tại Singapore, SSE-S3, không public, tự xóa sau 7 ngày.

## Chi phí và thời hạn

AWS công bố ưu đãi `t4g.small` 750 giờ/tháng đến hết **31/12/2026 UTC**,
tự áp dụng cho khách hàng mới và hiện tại, bao gồm Singapore.
Một máy chạy cả tháng nằm trong 750 giờ. CPU đặt ở chế độ `standard` để
không phát sinh phí surplus CPU credits.

Trong ưu đãi, chi phí nền dự kiến theo 730 giờ/tháng:

- EC2 compute: 0 USD, trong điều kiện ưu đãi.
- IPv4 public: 0,005 USD/giờ, khoảng 3,65 USD/tháng.
- gp3 8 GB: 0,096 USD/GB/tháng, khoảng 0,768 USD/tháng.
- Tổng nền: khoảng **4,42 USD/tháng**, chưa gồm traffic, request/log storage,
  thuế hoặc thay đổi giá. IPv4 dùng cho outbound Minecraft/SSM/cập nhật;
  inbound API đi qua CloudFront VPC origin.

CloudFront dùng pay-as-you-go, không có phí thuê cố định; traffic/request có thể
phát sinh phí ngoài mức miễn phí áp dụng. Theo dõi chi phí thực tế tại
AWS Settings > Billing và AWS Billing and Cost Management.

**Sau ưu đãi, `t4g.small` tự tính giá On-Demand.** Cần đổi xuống `t4g.micro`
trước khi ưu đãi hết nếu muốn giảm chi phí. Giá `t4g.micro` kiểm tra lúc triển khai:
0,0106 USD/giờ tại Singapore, khoảng 7,74 USD/tháng phần compute.
Thay đổi instance type cần stop/start; dữ liệu RAM/sequence sẽ reset.

Free plan của project được kiểm tra đang ACTIVE, còn 100 USD credit,
hết hạn **06/01/2027**. Credit không đồng nghĩa được chạy vượt thời hạn Free plan.

Nguồn: https://aws.amazon.com/ec2/faqs/#t4g-instances

## Frontend

Workflow: `.github/workflows/pages.yml`.
Repository variable `API_BASE_URL` phải là HTTPS origin của backend, không có path.
Pages sử dụng GitHub Actions làm build source.

Build local:

```sh
API_BASE_URL=https://<distribution>.cloudfront.net python3 scripts/build-pages.py
```

Output nằm ở `artifacts/pages`, được git-ignore. `config.js` chứa URL API public,
không chứa AWS credential. Đường dẫn asset tương đối hỗ trợ Pages dưới đường dẫn repo.

## Vận hành backend

Trong Systems Manager Run Command hoặc Session Manager:

```sh
sudo systemctl status minecraft-monitor nginx
sudo journalctl -u minecraft-monitor -n 50 --no-pager
curl http://127.0.0.1:8080/health/ready
curl http://127.0.0.1:9090/metrics
```

Source cấu hình nằm ở `deploy/aws`. Binary ARM64 được cross-compile trước khi upload,
không cài Go/compiler lên EC2. Bundle triển khai được truyền qua presigned S3 URL
ngắn hạn và kiểm tra SHA-256 trước khi cài. Bucket upload tạm được dọn sau triển khai.

Backend không tự cập nhật khi push source. Để cập nhật, build binary `linux/arm64`,
chuyển vào `/opt/minecraft-monitor/minecraft-monitor` qua kênh quản lý tin cậy,
restart service rồi kiểm tra REST/SSE. Giữ bản binary cũ để rollback.

Máy đơn: stop/restart hoặc lỗi máy sẽ gây gián đoạn ngắn. SSE reconnect nhận snapshot mới.
Không tăng replica khi chưa có poller leader/fan-out chung.
