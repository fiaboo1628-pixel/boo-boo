# 👻 Boo Boo

Dashboard home-server nhẹ cho Linux, ý tưởng giống [CasaOS](https://casaos.io): cài app bằng một cú bấm (chạy bằng Docker), xem CPU/RAM/ổ đĩa, tất cả qua giao diện web có đăng nhập.

Chạy được trên Debian, Ubuntu, Raspberry Pi OS (x86_64, arm64, armv7). Chỉ là **một file binary Go** + Docker.

## Cài đặt nhanh

```sh
curl -fsSL https://raw.githubusercontent.com/fiaboo1628-pixel/boo-boo/main/scripts/install.sh | sudo sh
```

Script sẽ cài Docker (nếu chưa có), tải binary từ bản release mới nhất, tạo service systemd `booboo`. Sau đó mở `http://<ip-server>:8080` và tạo tài khoản admin ở lần đầu.

> Script cần có ít nhất một bản release (tag `v*`), workflow `release.yml` sẽ tự build binary cho 3 kiến trúc.

## Chạy từ source (để phát triển)

Cần Go 1.24+ và Docker (có plugin `docker compose`).

```sh
go run ./cmd/booboo -addr :8080 -data ./data
```

| Flag | Mặc định | Ý nghĩa |
|---|---|---|
| `-addr` | `:8080` | Địa chỉ web UI lắng nghe |
| `-data` | `/var/lib/booboo` | Nơi lưu tài khoản và dữ liệu các app |

## Cách hoạt động

- **App Store**: mỗi app là một thư mục trong [`catalog/`](catalog) gồm `app.json` (tên, mô tả, icon, cổng) và `docker-compose.yml`. Catalog được nhúng vào binary.
- Khi bấm **Cài đặt**, Boo Boo chép file compose vào `<data>/apps/<id>/` rồi chạy `docker compose -p booboo-<id> up -d`. Biến `${APP_DATA}` trỏ tới `<data>/apps/<id>/data` để dữ liệu app nằm lâu dài trên máy.
- **Dừng / Chạy / Gỡ** gọi `docker compose stop | start | down`. Gỡ app **không xoá dữ liệu**, cài lại sẽ dùng lại.
- **Thông số hệ thống** đọc trực tiếp từ `/proc` và `statfs`, không cần thư viện ngoài.
- **Đăng nhập**: một tài khoản admin, mật khẩu băm PBKDF2-SHA256, phiên đăng nhập bằng cookie HttpOnly.

App có sẵn: File Browser, Jellyfin, Nextcloud, Pi-hole, Uptime Kuma.

> Pi-hole dùng cổng 53. Trên Ubuntu cần tắt DNS stub của `systemd-resolved` trước khi cài.

### Thêm app mới

Tạo `catalog/<id>/app.json` và `catalog/<id>/docker-compose.yml` (xem các app có sẵn làm mẫu), build lại là xong.

## Cấu trúc code

```
cmd/booboo/        main: đọc flag, khởi động server
internal/apps/     quản lý app qua docker compose
internal/auth/     tài khoản admin + phiên đăng nhập
internal/sysinfo/  CPU / RAM / ổ đĩa / uptime
internal/server/   HTTP API
web/               giao diện (HTML/CSS/JS thuần, nhúng vào binary)
catalog/           các app trong App Store
scripts/           script cài đặt + service systemd
```

## Kế hoạch

**MVP (bản này)**
- [x] App Store: cài / chạy / dừng / gỡ app từ catalog docker-compose
- [x] Dashboard CPU, RAM, ổ đĩa, uptime
- [x] Đăng nhập admin
- [x] Script cài một dòng + systemd service
- [x] CI chạy test, workflow release build binary amd64/arm64/armv7

**Tiếp theo**
- [ ] Cài app chạy nền, hiện tiến trình tải image
- [ ] Xem log và dùng tài nguyên của từng container
- [ ] Cho phép chỉnh cổng / biến môi trường trước khi cài (tránh trùng cổng)
- [ ] Thêm app từ file compose tự dán vào (custom app)
- [ ] Trình quản lý file và ổ đĩa gắn ngoài (USB, HDD)
- [ ] Catalog tải từ repo riêng để cập nhật app mà không cần build lại
- [ ] HTTPS / reverse proxy, đổi mật khẩu, nhiều người dùng
