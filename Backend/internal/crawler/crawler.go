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

	// 1. Điền thông tin
	// Đối với Element đã lấy ra (el), Thu dùng .SelectText(".*") để chọn toàn bộ nội dung
	c.Page.MustElement(".ant-modal-content #username").MustSelectText(".*").MustInput("") // Xóa trắng
	c.Page.MustElement(".ant-modal-content #username").MustInput(user)

	c.Page.MustElement(".ant-modal-content #password").MustSelectText(".*").MustInput("") // Xóa trắng
	c.Page.MustElement(".ant-modal-content #password").MustInput(pass)

	elCap := c.Page.MustElement(".ant-modal-content #cvalue")
	elCap.MustSelectText(".*").MustInput("") // Chọn tất cả bằng Regex ".*" rồi xóa
	elCap.MustInput(captcha)

	// Ép React/Ant Design nhận diện sự thay đổi dữ liệu
	elCap.MustEval(`() => {
        this.dispatchEvent(new Event('input', { bubbles: true }));
        this.dispatchEvent(new Event('change', { bubbles: true }));
        this.blur();
    }`)

	// 2. CLICK ĐĂNG NHẬP
	// Tìm đúng nút Primary trong Modal để click
	c.Page.MustElement(".ant-modal-content button.ant-btn-primary").MustClick()

	// 3. VÒNG LẶP KIỂM TRA LỖI (Quét liên tục trong 5 giây)
	fmt.Println("⏳ Đang quét lỗi và trạng thái hệ thống...")

	for i := 0; i < 20; i++ {
		// 1. Dùng Has để kiểm tra lỗi một cách nhẹ nhàng (Không dùng Must để tránh Panic)
		hasErr, el, _ := c.Page.Has(".ant-notification-notice-message, .ant-message-error, .ant-form-item-explain-error")

		if hasErr {
			// Dùng Element thay vì MustElement để lấy text an toàn
			msg := ""
			if el != nil {
				msg, _ = el.Text()
			}
			fmt.Printf("❌ Thất bại: %s\n", msg)

			// Nếu là lỗi Captcha
			if strings.Contains(strings.ToLower(msg), "captcha") || strings.Contains(strings.ToLower(msg), "mã xác nhận") {
				// CỰC KỲ QUAN TRỌNG: Không dùng MustClick ở đây vì nó sẽ gây treo Postman
				// Mình chỉ cần báo lỗi về, việc click đổi ảnh để sau
				return fmt.Errorf("SAI_CAPTCHA: %s", msg)
			}

			// Trả lỗi đăng nhập thông thường về ngay
			return fmt.Errorf("LOI_DANG_NHAP: %s", msg)
		}

		// 2. Kiểm tra thành công
		if hasLogout, _, _ := c.Page.Has(".anticon-logout"); hasLogout {
			fmt.Println("✅ Đăng nhập thành công!")
			return nil
		}

		time.Sleep(250 * time.Millisecond)
	}

	// 4. CHỐT CHẶN CUỐI CÙNG: Kiểm tra URL nếu Modal đã đóng mà chưa thấy lỗi
	currURL := c.Page.MustInfo().URL
	if strings.Contains(currURL, "gdt.gov.vn") && !strings.Contains(currURL, "login") {
		return nil
	}

	return fmt.Errorf("TIMEOUT: Hệ thống không báo lỗi nhưng không vào được trang chủ")
}
func (c *CrawlerService) SearchAndGetInvoices(fromDate, toDate string) ([]models.Invoice, error) {
	// 1. Điều hướng menu (Mở trang Tra cứu hóa đơn)
	var allInvoices []models.Invoice
	c.GrantPermissions()
	fmt.Println("🚀 Đang mở trang Tra cứu...")

	c.Page.MustWaitLoad()

	c.Page.MustElementR(".ml-menu-item span", "Tra cứu").MustClick()

	fmt.Println("🖱️ Đang vào phân mục Tra cứu hóa đơn...")
	c.Page.MustElement("a[href*='/tra-cuu/tra-cuu-hoa-don']").MustWaitVisible().MustClick()

	c.Page.MustWaitIdle()
	time.Sleep(1 * time.Second)

	// --- GIỮ NGUYÊN HÀM PHỤ selectDateOnCalendar CỦA THẮNG ---
	selectDateOnCalendar := func(selector string, dateStr string) {
		parts := strings.Split(dateStr, "/")
		dayst := strings.TrimLeft(parts[0], "0")
		monthst := parts[1]
		yearst := parts[2]

		fmt.Printf("📅 Đang chọn %s cho ô %s...\n", dateStr, selector)
		elInput := c.Page.MustElement(selector)
		elInput.MustClick()
		time.Sleep(800 * time.Millisecond)

		// Vòng lặp Năm (Giữ nguyên)
		for {
			calYear := c.Page.MustElement(".ant-calendar-year-select").MustText()
			if calYear == yearst {
				break
			}
			if yearst > calYear {
				c.Page.MustElement(".ant-calendar-next-year-btn").MustClick()
			} else {
				c.Page.MustElement(".ant-calendar-prev-year-btn").MustClick()
			}
			time.Sleep(100 * time.Millisecond)
		}

		// Vòng lặp Tháng (Giữ nguyên)
		for {
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
			time.Sleep(100 * time.Millisecond)
		}

		// Click chọn Ngày (Giữ nguyên)
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

	// --- THỰC THI TRA CỨU ---
	// 1. Thực hiện chọn 2 ô ngày
	selectDateOnCalendar("#tngay input", fromDate)
	selectDateOnCalendar("#dngay input", toDate)

	// 2. Nhấn Tìm kiếm
	fmt.Println("🖱️ Đang nhấn nút Tìm kiếm...")
	c.Page.MustElement("button.ant-btn-primary[type='submit']").MustClick()

	// 3. Đợi dữ liệu bảng tải lên
	fmt.Println("⏳ Đang đợi dữ liệu bảng tải lên...")
	c.Page.Timeout(20 * time.Second).MustElement(".ant-table-tbody").MustWaitVisible()
	time.Sleep(2 * time.Second)

	// 4. BÓC TÁCH DỮ LIỆU (Thay vì trả về rows trực tiếp)
	var invoiceList []models.Invoice
	// rows := c.Page.MustElements(".ant-table-tbody tr")

	for {
		fmt.Println("⏳ Đang đợi dữ liệu bảng tải lên...")
		c.Page.Timeout(20 * time.Second).MustElement(".ant-table-tbody").MustWaitVisible()
		time.Sleep(1500 * time.Millisecond) // Đợi chút cho dữ liệu đổ vào tr

		// 1. Bóc tách dữ liệu trang hiện tại
		rows := c.Page.MustElements(".ant-table-tbody tr")
		for _, row := range rows {
			cols := row.MustElements("td")
			if len(cols) >= 10 {
				// Thắng kiểm tra lại thứ tự cột trên máy Thắng nhé, thường là:
				// cols[1]: Ký hiệu, cols[2]: Số hóa đơn, cols[3]: Ngày lập,
				// cols[4]: MST người bán, cols[5]: Tên người bán...
				inv := models.Invoice{
					STT:         strings.TrimSpace(cols[0].MustText()),
					KyHieu:      strings.TrimSpace(cols[1].MustText()),
					SoHoaDon:    strings.TrimSpace(cols[2].MustText()),
					NgayLap:     strings.TrimSpace(cols[3].MustText()),
					MSTNguoiBan: strings.TrimSpace(cols[4].MustText()),
					NguoiBan:    strings.TrimSpace(cols[5].MustText()),
					TongTien:    strings.TrimSpace(cols[7].MustText()),
					TrangThai:   strings.TrimSpace(cols[9].MustText()),
				}
				if inv.STT != "" && inv.STT != "Không có dữ liệu" {
					allInvoices = append(allInvoices, inv)
				}
			}
		}

		// 2. KIỂM TRA VÀ NHẤN TRANG TIẾP THEO
		// Tìm nút mũi tên sang phải (Next Page)
		nextBtn, err := c.Page.Element(".ant-pagination-next")
		if err != nil {
			break // Không thấy nút phân trang thì dừng
		}

		// Kiểm tra xem nút Next có bị xám (disabled) không
		classAttr, _ := nextBtn.Attribute("class")
		if classAttr != nil && strings.Contains(*classAttr, "ant-pagination-disabled") {
			fmt.Println("✅ Đã lấy hết tất cả các trang!")
			break // Nếu bị disabled nghĩa là đã ở trang cuối
		}

		fmt.Println("⏭️ Đang sang trang tiếp theo...")
		nextBtn.MustClick()
		time.Sleep(1 * time.Second)
	}

	fmt.Printf("📂 Thành công! Tìm thấy %d hóa đơn.\n", len(invoiceList))
	return invoiceList, nil
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
