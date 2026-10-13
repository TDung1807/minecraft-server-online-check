# Minecraft Monitor

Backend Go theo dõi `bora.pikamc.vn:25005` qua Minecraft Java TCP status ping. Một poller chung truy vấn mỗi 5 giây; REST đọc RAM và SSE đẩy snapshot. Giao diện theo token trong `DESIGN.md`: nền navy, thẻ pastel, minh họa khối Minecraft, sao chép IP và danh sách người chơi cập nhật trực tiếp. Frontend được nhúng trong binary và deploy riêng lên GitHub Pages, không cần Node hay database. 
