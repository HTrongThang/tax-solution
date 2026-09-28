# DANH SÁCH HẠNG MỤC CẦN TỐI ƯU - TAXSOLUTION

Tài liệu này tổng hợp các hạng mục cần tối ưu cho dự án **TaxSolution**, phân loại theo mức độ ưu tiên từ bảo mật, độ ổn định cho đến hiệu năng và tính năng mở rộng.

---

## MỨC ĐỘ 1: BẢO MẬT & ĐỘ ỔN ĐỊNH (Ưu tiên cao nhất)

- [x] **1. Chuyển đổi lưu trữ tài khoản thuế sang SQLite kèm mã hóa DPAPI**
  - **Vị trí file:** `Frontend/Services/DatabaseService.cs`, `SecurityHelper.cs`, `UserForm.cs`
  - **Kết quả:** Đã triển khai database SQLite (`tax_solution.db`), tự động tạo bảng `TaxAccounts`, mã hóa mật khẩu bằng Windows DPAPI, tự động migrate dữ liệu từ file `tax_accounts.json` cũ sang SQLite và sao lưu thành `.bak`.

- [x] **2. Dọn dẹp tiến trình `TaxBot.exe` mồ côi (Zombie Process)**
  - **Vị trí file:** `Frontend/Program.cs` (hàm `StartBackend`)
  - **Kết quả:** Đã thêm cơ chế tự động quét và kill các tiến trình `TaxBot` cũ còn sót lại trước khi khởi động, giải phóng cổng 8080 và bổ sung đường dẫn fallback tới `Backend/TaxBot.exe`.

- [ ] **3. Chống lỗi vỡ cú pháp JavaScript khi gọi `ExecuteScriptAsync`**
  - **Vị trí file:** `Frontend/Views/MainForm.cs` & `Frontend/Views/UserForm.cs`
  - **Hiện trạng:** Nối chuỗi trực tiếp kiểu `$"window.showAlert('{msg}')"`. Nếu `msg` có dấu nháy đơn, nháy kép hoặc ký tự xuống dòng sẽ gây `SyntaxError` khiến WebView2 không thực thi được.
  - **Giải pháp:** Thay thế toàn bộ việc nối chuỗi bằng `JsonConvert.ToString(msg)`.

- [ ] **4. Ẩn / Bảo mật chuỗi Hash mật khẩu tài khoản Founder**
  - **Vị trí file:** `Frontend/Views/MainForm.cs` (dòng 102)
  - **Hiện trạng:** Chuỗi SHA-256 hash của Founder bị hardcode cứng trong code C#.
  - **Giải pháp:** Chuyển sang lưu trữ cấu hình có mã hóa hoặc dùng cơ chế Salt/PBKDF2.

---

## MỨC ĐỘ 2: HIỆU NĂNG & GIAO TIẾP FRONTEND - C#

- [ ] **5. Chuyển cơ chế truyền danh sách hóa đơn sang `PostWebMessageAsJson`**
  - **Vị trí file:**
    - C#: `Frontend/Views/UserForm.cs` (dòng 641)
    - JS: `Frontend/WebResources/js/crawlerTable.js`
  - **Hiện trạng:** Đang nhồi chuỗi JSON megabyte vào hàm `ExecuteScriptAsync`.
  - **Giải pháp:** Đổi sang gửi message qua `PostWebMessageAsJson` và bên JS bắt sự kiện bằng `window.chrome.webview.addEventListener('message', ...)`.

- [ ] **6. Bổ sung Phân trang (Pagination) cho bảng hiển thị hóa đơn**
  - **Vị trí file:** `Frontend/WebResources/js/crawlerTable.js`
  - **Hiện trạng:** Vẽ toàn bộ hàng nghìn dòng vào DOM cùng một lúc gây giật/đơ thanh cuộn.
  - **Giải pháp:** Chia trang (50 - 100 dòng/trang) để thanh cuộn luôn mượt mà.

- [ ] **7. Đóng gói tĩnh CSS Tailwind (Production Build)**
  - **Vị trí file:** `Frontend/WebResources/js/tailwind.js`
  - **Hiện trạng:** Thư viện runtime nặng 407 KB, biên dịch JIT trực tiếp trong web.
  - **Giải pháp:** Build ra 1 file `style.css` tĩnh đã minify (khoảng 20-30 KB) để giao diện nạp ngay lập tức, giảm tải CPU/RAM.

---

## MỨC ĐỘ 3: ĐỘ BỀN CỦA BOT CRAWLER (BACKEND GO)

- [ ] **8. Thay thế các XPath cứng theo số thứ tự button `[10]`, `[14]`**
  - **Vị trí file:** `Backend/internal/crawler/crawler.go` (dòng 532, 748, 778)
  - **Hiện trạng:** Chọn nút chuyển trang bằng `(//button[@type='button'])[10]`. Nếu website thuế thay đổi giao diện, bot sẽ bị gãy hoàn toàn.
  - **Giải pháp:** Đổi sang selector theo đặc trưng:
    - Nút chuyển trang: `//button[.//i[contains(@class,'anticon-right')]]`
    - Nút tải: Tìm theo icon download hoặc tooltip.

- [ ] **9. Tùy chọn chế độ ẩn trình duyệt (Headless Chrome)**
  - **Vị trí file:** `Backend/internal/crawler/crawler.go` (hàm `NewCrawlerService`)
  - **Hiện trạng:** Luôn hiển thị cửa sổ Chrome to trên màn hình.
  - **Giải pháp:** Cho phép nhận cấu hình bật/tắt `Headless(true/false)` theo ý người dùng.

---

## MỨC ĐỘ 4: TIỆN ÍCH CHO NGƯỜI DÙNG CUỐI (KẾ TOÁN)

- [ ] **10. Thêm tính năng "Xuất Excel" danh sách hóa đơn**
  - **Vị trí file:** `Frontend/WebResources/User/components/crawler_table.html` & `crawlerTable.js`
  - **Giải pháp:** Tích hợp thư viện xuất Excel (`xlsx.full.min.js`) để kế toán tải file bảng kê đối chiếu nhanh.

- [ ] **11. Nút "Mở thư mục lưu" khi hoàn tất tải file**
  - **Vị trí file:** `Frontend/Views/UserForm.cs` & `Frontend/WebResources/js/crawlerTable.js`
  - **Giải pháp:** Khi cào xong, hiển thị nút bấm để mở thẳng thư mục lưu trên Windows Explorer thông qua `Process.Start("explorer.exe", outputDir)`.
