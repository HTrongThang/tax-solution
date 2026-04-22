package crawler

import (
	"TAXSOLUTION/internal/models"
	"archive/zip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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
func (c *CrawlerService) SearchAndGetInvoices(ctx context.Context, fromDate, toDate string, stream chan models.StreamData) ([]models.Invoice, error) {
	if c.Page == nil {
		if c.Browser == nil {
			return nil, fmt.Errorf("browser chưa được khởi tạo")
		}
		c.Page = c.Browser.MustPage()
	}

	var allInvoices []models.Invoice
	seen := make(map[string]bool)
	c.IsRunning = true
	c.TotalInvoices = 0

	c.GrantPermissions()

	if stream != nil {
		stream <- models.StreamData{Status: "🚀 Đang khởi động bot...", IsDone: false}
	}

	defer func() {
		c.IsRunning = false
		if c.CurrentStatus != "Lỗi" && c.CurrentStatus != "Không có kết quả" {
			c.CurrentStatus = "Hoàn thành"
		}
	}()

	// --- 1. ĐIỀU HƯỚNG ---
	c.CurrentStatus = "Đang mở menu Tra cứu..."
	if err := c.Page.Context(ctx).Timeout(30 * time.Second).WaitLoad(); err != nil {
		c.CurrentStatus = "Lỗi"
		return nil, fmt.Errorf("không thể tải trang: %v", err)
	}

	c.Page.MustElementR(".ml-menu-item span", "Tra cứu").MustClick()
	c.Page.MustElement("a[href*='/tra-cuu/tra-cuu-hoa-don']").MustWaitVisible().MustClick()
	c.Page.MustWaitIdle()

	// --- 2. TÌM KIẾM ---
	c.selectDateOnCalendar("#tngay input", fromDate)
	c.selectDateOnCalendar("#dngay input", toDate)
	c.Page.MustElement("button.ant-btn-primary[type='submit']").MustClick()

	// --- 3. VÒNG LẶP ---
	maxPages := 1
	lastFirstInvoiceNo := ""
	totalOnTax := 0
	hasSentTotal := false // Biến cờ để chỉ gửi tổng số lượng 1 lần

	for pageIdx := 1; ; pageIdx++ {
		if ctx.Err() != nil {
			return allInvoices, ctx.Err()
		}

		// A. Đợi và lấy thông tin tổng số kết quả
		foundData := false
		for i := 0; i < 40; i++ {
			el, err := c.Page.ElementR("span", `Có\s\d+\skết quả`)
			if err == nil {
				txt := el.MustText()

				// Bóc số lượng bằng Regex
				re := regexp.MustCompile(`\d+`)
				match := re.FindString(txt)
				if match != "" {
					totalOnTax, _ = strconv.Atoi(match)
				}

				if strings.Contains(txt, "Có 0 kết quả") {
					c.CurrentStatus = "Không có kết quả"
					return allInvoices, nil
				}

				// Gửi tổng số lượng tìm thấy (Chỉ gửi 1 lần duy nhất ở trang 1)
				if !hasSentTotal && stream != nil && totalOnTax > 0 {
					stream <- models.StreamData{
						Status:     fmt.Sprintf("🔎 Hệ thống Thuế báo có tổng cộng %d hóa đơn", totalOnTax),
						TotalOnTax: totalOnTax,
						IsDone:     false,
					}
					hasSentTotal = true
				}

				foundData = true
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		// Gửi trạng thái trang hiện tại
		if stream != nil {
			stream <- models.StreamData{
				Status:      fmt.Sprintf("🔄 Đang xử lý trang %d...", pageIdx),
				CurrentPage: pageIdx,
				IsDone:      false,
			}
			time.Sleep(700 * time.Millisecond)
		}

		if !foundData {
			return allInvoices, fmt.Errorf("timeout đợi dữ liệu trang %d", pageIdx)
		}

		// B. Cập nhật số trang
		if pageIdx == 1 {
			if pageInfoEl, err := c.Page.Element("div[class*='PageIndex']"); err == nil {
				parts := strings.Split(pageInfoEl.MustText(), "/")
				if len(parts) == 2 {
					total, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
					maxPages = total
				}
			}
		}

		// C. Đợi bảng cập nhật (từ trang 2)
		if pageIdx > 1 {
			for i := 0; i < 30; i++ {
				currentNo := c.getFirstInvoiceNoFromUI()
				if currentNo != "" && currentNo != lastFirstInvoiceNo {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
		}

		// D. BÓC DỮ LIỆU
		remoteData := c.Page.MustEval(`() => {
            const rows = Array.from(document.querySelectorAll(".ant-table-tbody tr.ant-table-row"));
            return rows.map(row => Array.from(row.querySelectorAll("td")).map(td => td.innerText.trim()));
        }`).Arr()

		for _, rowRaw := range remoteData {
			cols := rowRaw.Arr()
			if len(cols) < 14 {
				continue
			}

			uniqueKey := fmt.Sprintf("%s_%s_%s", cols[1].String(), cols[3].String(), cols[4].String())
			if seen[uniqueKey] {
				continue
			}
			seen[uniqueKey] = true

			inv := models.Invoice{
				STT:            cols[0].String(),
				MSTNguoiBan:    cols[1].String(),
				KyHieuMauSo:    cols[2].String(),
				KyHieu:         cols[3].String(),
				SoHoaDon:       cols[4].String(),
				NgayLap:        cols[5].String(),
				ThongTinHoaDon: cols[6].String(),
				TongChuaThue:   cols[7].String(),
				TongTienThue:   cols[8].String(),
				TongThanhToan:  cols[11].String(),
				TrangThai:      cols[13].String(),
			}
			allInvoices = append(allInvoices, inv)
			c.TotalInvoices++
		}

		if len(remoteData) > 0 {
			lastFirstInvoiceNo = remoteData[0].Arr()[4].String()
		}

		// --- 4. CHUYỂN TRANG ---
		if pageIdx >= maxPages {
			break
		}

		btnNext, err := c.Page.ElementX("(//button[@type='button'])[10]")
		if err != nil {
			break
		}

		btnNext.MustClick()
		time.Sleep(2 * time.Second)
	}

	// 5. KẾT THÚC
	if stream != nil {
		stream <- models.StreamData{
			Invoices:   allInvoices,
			Status:     "✅ Hoàn thành tra cứu!",
			TotalSoFar: c.TotalInvoices,
			TotalOnTax: totalOnTax,
			IsDone:     true,
		}
	}

	return allInvoices, nil
}

// Hàm bổ trợ lấy Số hóa đơn đầu tiên trên bảng hiện tại
func (c *CrawlerService) getFirstInvoiceNoFromUI() string {
	res, err := c.Page.Eval(`() => {
        const cell = document.querySelector(".ant-table-tbody tr.ant-table-row td:nth-child(5)");
        return cell ? cell.innerText.trim() : "";
    }`)
	if err != nil {
		return ""
	}
	return res.Value.String()
}

// selectDateOnCalendar thực hiện chọn ngày trên giao diện Ant Design Calendar
func (c *CrawlerService) selectDateOnCalendar(selector string, dateStr string) {
	parts := strings.Split(dateStr, "/")
	if len(parts) != 3 {
		fmt.Printf("⚠️ Định dạng ngày không hợp lệ: %s\n", dateStr)
		return
	}

	dayst := strings.TrimLeft(parts[0], "0")
	monthst := parts[1]
	yearst := parts[2]

	fmt.Printf("📅 Đang chọn %s cho ô %s...\n", dateStr, selector)

	// Click để mở popup lịch
	elInput := c.Page.MustElement(selector)
	elInput.MustClick()
	time.Sleep(800 * time.Millisecond)

	// --- Vòng lặp chọn Năm ---
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
		time.Sleep(150 * time.Millisecond)
	}

	// --- Vòng lặp chọn Tháng ---
	for {
		calMonthFull := c.Page.MustElement(".ant-calendar-month-select").MustText()
		// Làm sạch chuỗi tháng (Xóa "Thg" hoặc "Tháng")
		calMonth := strings.TrimSpace(strings.ReplaceAll(calMonthFull, "Thg", ""))
		calMonth = strings.TrimSpace(strings.ReplaceAll(calMonth, "Tháng", ""))

		if calMonth == monthst {
			break
		}
		if monthst > calMonth {
			c.Page.MustElement(".ant-calendar-next-month-btn").MustClick()
		} else {
			c.Page.MustElement(".ant-calendar-prev-month-btn").MustClick()
		}
		time.Sleep(150 * time.Millisecond)
	}

	// --- Click chọn đúng Ngày ---
	cells := c.Page.MustElements(".ant-calendar-table .ant-calendar-cell")
	for _, cell := range cells {
		if cell.MustText() == dayst {
			classPtr := cell.MustAttribute("class")
			// Đảm bảo không click nhầm vào ngày của tháng trước hoặc tháng sau đang hiển thị mờ
			if classPtr != nil &&
				!strings.Contains(*classPtr, "last-month-cell") &&
				!strings.Contains(*classPtr, "next-month-btn-day") {
				cell.MustClick()
				fmt.Printf("✅ Đã chọn ngày %s\n", dayst)
				break
			}
		}
	}
	time.Sleep(500 * time.Millisecond)
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
	c.GrantPermissions()

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
