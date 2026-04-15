package crawler

import (
	"TAXSOLUTION/internal/models"
	"archive/zip"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

type CrawlerService struct {
	Browser       *rod.Browser
	Page          *rod.Page
	OutputDir     string
	IsRunning     bool   // Đang chạy hay không
	CurrentStatus string // Thông báo trạng thái (ví dụ: "Đang tải trang 2...")
	TotalInvoices int    // Tổng số hóa đơn đã tải được
}

// Sửa chỗ này: Xóa crawler. trước CrawlerService
func NewCrawlerService() *CrawlerService {
	l := launcher.New().
		Bin("C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe").
		Leakless(false).
		Headless(false)
	l.Append("--start-maximized")

	url := l.MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()

	// Sửa chỗ này luôn: Xóa crawler.
	return &CrawlerService{
		Browser: browser,
	}
}

// Hàm thực hiện đăng nhập và lấy dữ liệu
func (s *CrawlerService) ShowLogin() error {
	fmt.Println("Đang mở trang chủ...")

	// Khởi tạo trang và gán vào s.Page để dùng cho các hàm Login, Crawl sau này
	s.Page = s.Browser.MustPage("https://hoadondientu.gdt.gov.vn/")
	s.Page.MustWaitLoad()

	// --- XỬ LÝ QUẢNG CÁO TỐI ƯU ---
	_ = rod.Try(func() {
		// Tìm nút close và click nếu thấy trong vòng 3s
		s.Page.Timeout(3 * time.Second).MustElement(".ant-modal-close").MustClick()
		fmt.Println("Đã đóng quảng cáo.")
	})

	fmt.Println("Đang mở Dialog đăng nhập...")
	// Tìm nút Đăng nhập bằng XPath
	loginBtn := s.Page.MustElementX("//*[contains(text(), 'Đăng nhập')]")
	loginBtn.MustWaitVisible().MustClick()

	// Đợi Modal (Dialog) hiện lên hoàn toàn
	s.Page.MustElement(".ant-modal-content").MustWaitVisible()

	fmt.Println("[DEBUG] Dialog Đăng nhập đã hiện!")
	return nil
}

func (s *CrawlerService) GetCaptcha() (string, error) {
	fmt.Println("📸 Đang lấy mã Captcha TRONG khung Đăng nhập...")

	// 1. Chờ Dialog hiện lên hẳn (Dùng s.Page cho đồng bộ với struct của Thu nhé)
	s.Page.MustElement(".ant-modal-content").MustWaitVisible()

	// 2. SELECTOR THẦN THÁNH
	el := s.Page.MustElement(".ant-modal-content img.Captcha__Image-sc-1up1k1e-1")

	// 3. Đợi ảnh ổn định
	el.MustWaitVisible()
	time.Sleep(1 * time.Second)

	// 4. Chụp ảnh
	imgData := el.MustScreenshot("")

	// 5. Vừa lưu file để Thu test, vừa chuẩn bị Base64
	fileName := "current_captcha.png"
	os.WriteFile(fileName, imgData, 0644)

	// Mở file lên cho Thu xem (Chỉ dùng khi đang code test)
	_ = exec.Command("cmd", "/c", "start", fileName).Run()

	encoded := base64.StdEncoding.EncodeToString(imgData)

	fmt.Println("✅ Đã lấy xong mã Captcha và lưu file thành công!")

	return encoded, nil
}
func (c *CrawlerService) Login(user, pass, captcha string) error {
	fmt.Println("🚀 Đang thực hiện đăng nhập (Chế độ kiểm soát nghiêm ngặt)...")

	if c.Page == nil {
		return fmt.Errorf("TRÌNH DUYỆT CHƯA MỞ: Vui lòng gọi /init trước")
	}

	// --- 1. ĐIỀN THÔNG TIN ---
	// Sử dụng MustClick trước khi Input để React nhận diện Focus State
	c.Page.MustElement(".ant-modal-content #username").MustClick().MustSelectText(".*").MustInput(user)
	c.Page.MustElement(".ant-modal-content #password").MustClick().MustSelectText(".*").MustInput(pass)

	elCap := c.Page.MustElement(".ant-modal-content #cvalue")
	elCap.MustClick().MustSelectText(".*").MustInput(captcha)

	// Ép React/Ant Design nhận diện sự thay đổi dữ liệu (Trigger Events)
	c.Page.MustEval(`() => {
        const ev = { bubbles: true };
        document.querySelectorAll('.ant-modal-content input').forEach(i => {
            i.dispatchEvent(new Event('input', ev));
            i.dispatchEvent(new Event('change', ev));
        });
    }`)

	// --- 2. CLICK ĐĂNG NHẬP ---
	c.Page.MustElement(".ant-modal-content button.ant-btn-primary").MustClick()

	// --- 3. VÒNG LẶP KIỂM TRA (CHỐT KẾT QUẢ) ---
	fmt.Println("⏳ Đang quét lỗi và trạng thái hệ thống...")

	for i := 0; i < 25; i++ {
		// ƯU TIÊN 1: KIỂM TRA THÀNH CÔNG (Dựa trên HTML Header thực tế Thắng gửi)
		// Check nút Logout (mũi tên phải), icon User, icon Bell
		hasSuccess, _, _ := c.Page.Has(".anticon-arrow-right, .anticon-user, .anticon-bell")

		// Check URL (Chỉ tin thành công nếu URL chứa domain thuế và KHÔNG chứa chữ login/auth)
		currURL := c.Page.MustInfo().URL
		isAuthPage := strings.Contains(strings.ToLower(currURL), "login") || strings.Contains(strings.ToLower(currURL), "auth")

		if hasSuccess && !isAuthPage {
			fmt.Println("✅ Đăng nhập thành công thực tế!")
			return nil
		}

		// ƯU TIÊN 2: KIỂM TRA LỖI (Đợi sau vòng lặp thứ 5 ~ 1.5 giây để tránh lỗi cũ)
		if i > 5 {
			hasErr, el, _ := c.Page.Has(".ant-notification-notice-message, .ant-message-error, .ant-form-item-explain-error")
			if hasErr {
				msg, _ := el.Text()
				if msg != "" {
					fmt.Printf("❌ Thất bại xác thực: %s\n", msg)

					// Trả về lỗi định dạng để C# xử lý logic hiện Alert
					if strings.Contains(strings.ToLower(msg), "captcha") || strings.Contains(strings.ToLower(msg), "mã xác nhận") {
						return fmt.Errorf("SAI_CAPTCHA: %s", msg)
					}
					return fmt.Errorf("LOI_DANG_NHAP: %s", msg)
				}
			}
		}

		time.Sleep(300 * time.Millisecond)
	}

	// --- 4. CHỐT CHẶN CUỐI CÙNG ---
	// Nếu hết 7.5 giây mà vẫn thấy trang Login hoặc không thấy nút Logout
	return fmt.Errorf("TIMEOUT: Hệ thống không phản hồi hoặc sai thông tin đăng nhập")
}
func (c *CrawlerService) SearchAndGetInvoices(fromDate, toDate string) ([]models.Invoice, error) {
	var allInvoices []models.Invoice
	c.GrantPermissions()

	// 1. Điều hướng Menu
	fmt.Println("🚀 Đang mở trang Tra cứu...")
	c.Page.MustWaitLoad()
	c.Page.MustElementR(".ml-menu-item span", "Tra cứu").MustClick()

	fmt.Println("🖱️ Đang vào phân mục Tra cứu hóa đơn...")
	c.Page.MustElement("a[href*='/tra-cuu/tra-cuu-hoa-don']").MustWaitVisible().MustClick()
	c.Page.MustWaitIdle()
	time.Sleep(1 * time.Second)

	// --- Hàm phụ chọn lịch (Giữ nguyên vì Thắng chạy đang ổn) ---
	selectDateOnCalendar := func(selector string, dateStr string) {
		parts := strings.Split(dateStr, "/")
		if len(parts) < 3 {
			return
		}
		dayst := strings.TrimLeft(parts[0], "0")
		monthst := parts[1]
		yearst := parts[2]

		elInput := c.Page.MustElement(selector)
		elInput.MustClick()
		time.Sleep(500 * time.Millisecond)

		for i := 0; i < 20; i++ {
			calYear := c.Page.MustElement(".ant-calendar-year-select").MustText()
			if calYear == yearst {
				break
			}
			if yearst > calYear {
				c.Page.MustElement(".ant-calendar-next-year-btn").MustClick()
			} else {
				c.Page.MustElement(".ant-calendar-prev-year-btn").MustClick()
			}
		}
		for i := 0; i < 12; i++ {
			calMonthFull := c.Page.MustElement(".ant-calendar-month-select").MustText()
			calMonth := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(calMonthFull, "Thg", ""), "Tháng", ""))
			if calMonth == monthst {
				break
			}
			if monthst > calMonth {
				c.Page.MustElement(".ant-calendar-next-month-btn").MustClick()
			} else {
				c.Page.MustElement(".ant-calendar-prev-month-btn").MustClick()
			}
		}
		cells := c.Page.MustElements(".ant-calendar-table .ant-calendar-cell")
		for _, cell := range cells {
			if cell.MustText() == dayst {
				classPtr := cell.MustAttribute("class")
				if classPtr != nil && !strings.Contains(*classPtr, "last-month-cell") && !strings.Contains(*classPtr, "next-month-btn-day") {
					cell.MustClick()
					break
				}
			}
		}
	}

	// 2. Điền ngày và Tìm kiếm
	selectDateOnCalendar("#tngay input", fromDate)
	selectDateOnCalendar("#dngay input", toDate)

	fmt.Println("🖱️ Đang nhấn nút Tìm kiếm...")
	c.Page.MustElement("button.ant-btn-primary[type='submit']").MustClick()

	// 3. Vòng lặp bóc tách và Phân trang
	for pageIdx := 1; ; pageIdx++ {
		fmt.Printf("⏳ Đang xử lý trang %d...\n", pageIdx)

		// CHIẾN THUẬT: Đợi dữ liệu thực sự (Ưu tiên hàng dữ liệu)
		foundData := false
		for i := 0; i < 40; i++ {
			// Nếu thấy dòng dữ liệu (tr có class ant-table-row)
			if has, _, _ := c.Page.Has(".ant-table-tbody tr.ant-table-row"); has {
				foundData = true
				break
			}

			// Chỉ tin là "Không có dữ liệu" sau khi đã đợi ít nhất 3 giây (i > 8)
			// Để tránh bắt nhầm cái Placeholder của lần tìm kiếm cũ
			if i > 8 {
				if has, _, _ := c.Page.Has(".ant-table-placeholder"); has {
					txt, _ := c.Page.MustElement(".ant-table-placeholder").Text()
					if strings.Contains(txt, "Không có dữ liệu") {
						fmt.Println("ℹ️ Trang Thuế xác nhận: Không có dữ liệu.")
						return allInvoices, nil
					}
				}
			}
			time.Sleep(350 * time.Millisecond)
		}

		if !foundData {
			fmt.Println("⚠️ Timeout hoặc không thấy bảng dữ liệu.")
			break
		}

		// Bóc tách trang hiện tại
		rows := c.Page.MustElements(".ant-table-tbody tr.ant-table-row")
		for _, row := range rows {
			cols := row.MustElements("td")
			if len(cols) >= 10 {
				stt := strings.TrimSpace(cols[0].MustText())
				if stt == "" || stt == "Không có dữ liệu" {
					continue
				}

				inv := models.Invoice{
					STT:         stt,
					KyHieu:      strings.TrimSpace(cols[1].MustText()),
					SoHoaDon:    strings.TrimSpace(cols[2].MustText()),
					NgayLap:     strings.TrimSpace(cols[3].MustText()),
					MSTNguoiBan: strings.TrimSpace(cols[4].MustText()),
					NguoiBan:    strings.TrimSpace(cols[5].MustText()),
					TongTien:    strings.TrimSpace(cols[7].MustText()),
					TrangThai:   strings.TrimSpace(cols[9].MustText()),
				}
				allInvoices = append(allInvoices, inv)
			}
		}

		// 4. Xử lý chuyển trang
		nextBtn, err := c.Page.Element(".ant-pagination-next")
		if err != nil {
			break
		}

		classAttr, _ := nextBtn.Attribute("class")
		if classAttr != nil && strings.Contains(*classAttr, "ant-pagination-disabled") {
			fmt.Println("✅ Đã lấy hết tất cả các trang.")
			break
		}

		fmt.Println("⏭️ Đang sang trang tiếp theo...")
		nextBtn.MustClick()
		c.Page.MustWaitIdle()
		time.Sleep(2 * time.Second) // Đợi bảng mới render hoàn toàn
	}

	fmt.Printf("📂 Xong! Tổng cộng lấy được %d hóa đơn.\n", len(allInvoices))
	return allInvoices, nil
}
func (c *CrawlerService) GrantPermissions() {
	homeDir, _ := os.UserHomeDir()
	// Lấy thời gian lúc bắt đầu chạy (Dùng làm tên folder chung)
	timeStr := time.Now().Format("20060102_150405")

	downloadDir := filepath.Join(homeDir, "Downloads", "Invoices_Temp")
	// Lưu đường dẫn vào struct để dùng xuyên suốt
	c.OutputDir = filepath.Join(homeDir, "Downloads", "Invoices_Output_"+timeStr)

	os.MkdirAll(downloadDir, os.ModePerm)
	os.MkdirAll(c.OutputDir, os.ModePerm)

	proto.BrowserSetDownloadBehavior{
		Behavior:      proto.BrowserSetDownloadBehaviorBehaviorAllow,
		DownloadPath:  downloadDir,
		EventsEnabled: true,
	}.Call(c.Page)

	fmt.Printf("🚀 Khởi tạo thành công!\n📁 Folder lưu chính: %s\n", c.OutputDir)
}
func (c *CrawlerService) CrawlAllPages() {
	// 1. Khởi tạo trạng thái bắt đầu
	c.IsRunning = true
	c.TotalInvoices = 0
	c.CurrentStatus = "Bắt đầu khởi động bot..."

	// Đảm bảo khi hàm kết thúc (xong hoặc lỗi) thì IsRunning phải về false
	defer func() {
		c.IsRunning = false
		if c.CurrentStatus != "Lỗi" {
			c.CurrentStatus = "Hoàn thành"
		}
	}()

	fmt.Println("🚀 Bắt đầu quy trình cào đa trang...")

	for page := 1; ; page++ {
		// Cập nhật trạng thái trang hiện tại cho FE
		c.CurrentStatus = fmt.Sprintf("Đang xử lý trang %d...", page)
		fmt.Printf("\n📄 --- %s ---\n", c.CurrentStatus)

		// Đợi bảng hiện ra
		c.Page.Timeout(15 * time.Second).MustElement(".ant-table-tbody").MustWaitVisible()
		time.Sleep(2 * time.Second)

		rows := c.Page.MustElements(".ant-table-tbody tr")
		fmt.Printf("📂 Tìm thấy %d dòng hóa đơn tại trang %d.\n", len(rows), page)

		for i, row := range rows {
			// Cập nhật trạng thái từng dòng để FE hiển thị tiến độ (ProgressBar)
			c.CurrentStatus = fmt.Sprintf("Trang %d: Đang tải dòng %d/%d", page, i+1, len(rows))
			fmt.Printf("   🔍 %s...\n", c.CurrentStatus)

			err := c.ProcessDownloadInvoice(row, i)
			if err != nil {
				fmt.Printf("   ❌ Lỗi tại dòng %d: %v\n", i+1, err)
				continue // Lỗi dòng này thì bỏ qua làm dòng tiếp theo
			}

			// Tải xong 1 dòng thì tăng biến đếm tổng
			c.TotalInvoices++
		}

		// Kiểm tra nút sang trang
		btnNext, err := c.Page.ElementX("(//button[@type='button'])[10]")

		// Kiểm tra điều kiện dừng
		if err != nil || btnNext.MustAttribute("disabled") != nil {
			fmt.Printf("🏁 Đã cào xong trang cuối cùng (Trang %d). Dừng lại!\n", page)
			break
		}

		fmt.Println("⏭️ Chuyển sang trang tiếp theo...")
		btnNext.MustClick()

		// Đợi 3 giây để dữ liệu trang mới kịp thay đổi
		time.Sleep(3 * time.Second)
	}
}

func (c *CrawlerService) ProcessDownloadInvoice(row *rod.Element, index int) error {
	fmt.Printf("\n🔍 --- Đang xử lý dòng %d ---\n", index+1)

	// --- BƯỚC 1 & 2: CHỌN VÀ TẢI (Giữ nguyên logic của Thắng) ---
	row.MustScrollIntoView()
	col2, _ := row.Element("td:nth-child(2)")
	col2.MustWaitVisible().MustClick()
	time.Sleep(1 * time.Second)

	btns := c.Page.MustElements("button.fRqwwC")
	var btnDownload *rod.Element
	if len(btns) >= 4 {
		btnDownload = btns[3]
	} else {
		btnDownload = c.Page.MustElementX("(//button[@type='button'])[14]")
	}
	btnDownload.MustWaitVisible().MustClick()
	fmt.Println("   📥 Đang tải file...")

	// --- BƯỚC 3: ĐẶT TÊN (Sửa lỗi biến stt) ---
	tds := row.MustElements("td")
	soKHMS := strings.TrimSpace(tds[1].MustText())
	kyHieuHD := strings.TrimSpace(tds[2].MustText())
	soHD := strings.TrimSpace(tds[3].MustText())
	ngayLap := strings.ReplaceAll(strings.TrimSpace(tds[4].MustText()), "/", "")

	// Đánh số thứ tự 01, 02...
	fileName := fmt.Sprintf("%02d_%s_%s_%s_%s", index+1, soKHMS, kyHieuHD, soHD, ngayLap)

	// --- BƯỚC 4: GIẢI NÉN VÀ DỌN DẸP ---

	homeDir, _ := os.UserHomeDir()
	downloadDir := filepath.Join(homeDir, "Downloads", "Invoices_Temp")

	var latestZip string

	// Thay vì Sleep 5s, ta check mỗi 500ms (tổng 10s timeout)
	err := c.retry(20, 500*time.Millisecond, func() bool {
		path, errFile := GetLatestFile(downloadDir)
		// Điều kiện 1: Không có lỗi khi lấy file
		// Điều kiện 2: File phải có đuôi .zip (tránh lấy nhầm file .tmp của Chrome)
		if errFile == nil && strings.HasSuffix(path, ".zip") {
			// Điều kiện 3: File phải có dung lượng (đã ghi xong xuống đĩa)
			info, errStat := os.Stat(path)
			if errStat == nil && info.Size() > 0 {
				latestZip = path
				return true
			}
		}
		return false
	})

	if err != nil {
		return fmt.Errorf("❌ Không tìm thấy file Zip hợp lệ sau 10 giây chờ đợi")
	}

	// Dùng c.OutputDir đã khởi tạo từ đầu
	err = c.ExtractAndMove(latestZip, c.OutputDir, fileName)

	if err == nil {
		fmt.Printf("   ✅ Đã lưu vào: %s\n", c.OutputDir)
		os.Remove(latestZip)
	}

	return nil
}

// ExtractAndMove: Giải nén file zip vào thư mục mới
func (c *CrawlerService) ExtractAndMove(srcZip, destDir, fileName string) error {
	r, err := zip.OpenReader(srcZip)
	if err != nil {
		return err
	}
	defer r.Close()

	os.MkdirAll(destDir, os.ModePerm)

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		// 1. Lấy đuôi file và chuyển về chữ thường để so khớp cho chuẩn
		ext := strings.ToLower(filepath.Ext(f.Name))

		// 2. CHỈ CHỌN XML VÀ PDF (Bỏ qua HTML và các file khác)
		if ext != ".xml" && ext != ".html" {
			fmt.Printf("      ⏭️  Bỏ qua file không cần thiết: %s\n", f.Name)
			continue
		}

		// 3. Tiến hành đổi tên và lưu (giống như cũ)
		finalPath := filepath.Join(destDir, fileName+ext)

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(finalPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		fmt.Printf("      📄 Đã đổi tên & lưu: %s%s\n", fileName, ext)
	}
	return nil
}
func GetLatestFile(dir string) (string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var latestFile string
	var latestTime time.Time
	for _, entry := range files {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".zip") {
			info, _ := entry.Info()
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestFile = entry.Name()
			}
		}
	}
	if latestFile == "" {
		return "", fmt.Errorf("thư mục trống")
	}
	return filepath.Join(dir, latestFile), nil
}
func (c *CrawlerService) retry(attempts int, sleep time.Duration, f func() bool) error {
	for i := 0; i < attempts; i++ {
		if f() {
			return nil
		}
		time.Sleep(sleep)
	}
	return fmt.Errorf("timeout sau %d lần thử", attempts)
}
func (s *CrawlerService) Logout() error {
	if s.Browser == nil {
		return nil
	}

	// Đóng toàn bộ trình duyệt và giải phóng tiến trình chrome.exe
	err := s.Browser.Close()
	if err != nil {
		return err
	}

	// Sau khi Close, biến Browser sẽ không dùng được nữa cho đến khi Init lại
	s.Browser = nil

	fmt.Println("Hệ thống: Đã đóng trình duyệt Chrome.")
	return nil
}
