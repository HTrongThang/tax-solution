package crawler

import (
	"TAXSOLUTION/internal/models"
	"archive/zip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
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
		Leakless(false).
		// Headless(true)
		Headless(false) // Hiển thị cửa sổ Chrome

	// Tự tìm Chrome/Chromium được cài trên máy thay vì hardcode path
	if chromePath, found := launcher.LookPath(); found {
		l = l.Bin(chromePath)
		fmt.Printf("💻 Dùng trình duyệt từ: %s\n", chromePath)
	} else {
		// Fallback về đường dẫn mặc định nếu LookPath không tìm thấy
		fmt.Println("⚠️ Không tìm thấy Chrome tự động, dùng đường dẫn mặc định")
		l = l.Bin("C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe")
	}
	l.Append("--start-maximized")

	url := l.MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()

	return &CrawlerService{
		Browser: browser,
	}
}

// Hàm thực hiện đăng nhập và lấy dữ liệu
func (s *CrawlerService) ShowLogin() error {
	fmt.Println("Đang mở trang chủ...")

	// Khởi tạo trang và gán vào s.Page để dùng cho các hàm Login, Crawl sau này
	err := rod.Try(func() {
		s.Page = s.Browser.MustPage("https://hoadondientu.gdt.gov.vn/")
		s.Page.MustWaitLoad()
	})
	if err != nil {
		return fmt.Errorf("không thể mở trang thuế: %v", err)
	}

	// --- XỬ LÝ QUẢNG CÁO TỐI ƯU ---
	_ = rod.Try(func() {
		// Tìm nút close và click nếu thấy trong vòng 3s
		s.Page.Timeout(3 * time.Second).MustElement(".ant-modal-close").MustClick()
		fmt.Println("Đã đóng quảng cáo.")
	})

	fmt.Println("Đang mở Dialog đăng nhập...")
	// Tìm nút Đăng nhập bằng XPath
	err = rod.Try(func() {
		loginBtn := s.Page.Timeout(10 * time.Second).MustElementX("//*[contains(text(), 'Đăng nhập')]")
		loginBtn.MustWaitVisible().MustClick()

		// Đợi Modal (Dialog) hiện lên hoàn toàn
		s.Page.Timeout(10 * time.Second).MustElement(".ant-modal-content").MustWaitVisible()
	})
	if err != nil {
		return fmt.Errorf("không thể mở dialog đăng nhập: %v", err)
	}

	fmt.Println("[DEBUG] Dialog Đăng nhập đã hiện!")
	return nil
}

// ShowLoginDialog: Mở dialog đăng nhập trên TAB HIỆN TẠI (không mở tab mới).
// Dùng sau SoftLogout khi page đã ở trang chủ.
func (s *CrawlerService) ShowLoginDialog() error {
	if s.Page == nil {
		return fmt.Errorf("trình duyệt chưa mở")
	}

	fmt.Println("Đang mở Dialog đăng nhập trên tab hiện tại...")

	// Đóng quảng cáo nếu có (timeout ngắn)
	_ = rod.Try(func() {
		s.Page.Timeout(1 * time.Second).MustElement(".ant-modal-close").MustClick()
		fmt.Println("Đã đóng quảng cáo.")
	})

	// Click nút "Đăng nhập" và đợi dialog hiện
	err := rod.Try(func() {
		loginBtn := s.Page.Timeout(5 * time.Second).MustElementX("//*[contains(text(), 'Đăng nhập')]")
		loginBtn.MustWaitVisible().MustClick()
		s.Page.Timeout(5 * time.Second).MustElement(".ant-modal-content").MustWaitVisible()
	})
	if err != nil {
		return fmt.Errorf("không thể mở dialog đăng nhập: %v", err)
	}

	fmt.Println("[DEBUG] Dialog Đăng nhập đã hiện!")
	return nil
}

func (s *CrawlerService) GetCaptcha() (string, error) {
	fmt.Println("📸 Đang lấy mã Captcha TRONG khung Đăng nhập...")

	if s.Page == nil {
		return "", fmt.Errorf("trình duyệt chưa mở")
	}

	var encoded string
	err := rod.Try(func() {
		// 1. Chờ Dialog hiện lên hẳn
		s.Page.Timeout(5 * time.Second).MustElement(".ant-modal-content").MustWaitVisible()

		// 2. Tìm ảnh captcha
		el := s.Page.Timeout(5 * time.Second).MustElement(".ant-modal-content img.Captcha__Image-sc-1up1k1e-1")

		// 3. Đợi ảnh render xong
		el.MustWaitVisible()
		time.Sleep(300 * time.Millisecond)

		// 4. Chụp ảnh và encode Base64
		imgData := el.MustScreenshot("")
		encoded = base64.StdEncoding.EncodeToString(imgData)
	})
	if err != nil {
		return "", fmt.Errorf("không thể lấy captcha: %v", err)
	}

	fmt.Println("✅ Đã lấy xong mã Captcha!")
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

	// Chỉ setup browser download behavior, không tạo OutputDir
	// (OutputDir chỉ càn khi export XML, không phải khi search)
	c.SetupBrowserDownload()

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

	// Kiểm tra Page còn valid không
	if c.Page == nil {
		if stream != nil {
			stream <- models.StreamData{Status: "❌ Trình duyệt chưa sẵn sàng", IsDone: true}
		}
		return nil, fmt.Errorf("page is nil - trình duyệt chưa sẵn sàng")
	}

	if err := c.Page.Context(ctx).Timeout(30 * time.Second).WaitLoad(); err != nil {
		c.CurrentStatus = "Lỗi"
		return nil, fmt.Errorf("không thể tải trang: %v", err)
	}

	// Kiểm tra xem đã ở trang Tra cứu chưa — nếu rồi thì bỏ qua navigate
	currentURL := ""
	_ = rod.Try(func() { currentURL = c.Page.MustInfo().URL })
	alreadyOnSearch := strings.Contains(currentURL, "tra-cuu")

	if !alreadyOnSearch {
		navErr := rod.Try(func() {
			c.Page.MustElementR(".ml-menu-item span", "Tra cứu").MustClick()
			c.Page.MustElement("a[href*='/tra-cuu/tra-cuu-hoa-don']").MustWaitVisible().MustClick()
			c.Page.MustWaitIdle()
		})
		if navErr != nil {
			if stream != nil {
				stream <- models.StreamData{Status: "❌ Không thể mở trang tra cứu", IsDone: true}
			}
			return nil, fmt.Errorf("lỗi điều hướng: %v", navErr)
		}
	} else {
		fmt.Println("📄 Đã ở trang Tra cứu, bỏ qua điều hướng.")
	}

	// --- 2. TÌM KIẾM (chọn ngày thông minh: so sánh với anchor hiện tại) ---
	// Đọc ngày hiện tại trên trang thuế
	var curFrom, curTo string
	_ = rod.Try(func() {
		curFrom = *c.Page.MustElement("#tngay input").MustAttribute("value")
		curTo = *c.Page.MustElement("#dngay input").MustAttribute("value")
	})
	fmt.Printf("📊 Ngày hiện tại trên trang: %s → %s | Muốn chọn: %s → %s\n", curFrom, curTo, fromDate, toDate)

	// Parse ngày để so sánh (dd/mm/yyyy)
	parseDate := func(s string) time.Time {
		t, _ := time.Parse("02/01/2006", s)
		return t
	}
	newFrom := parseDate(fromDate)
	oldFrom := parseDate(curFrom)
	oldTo := parseDate(curTo)

	// Nếu ngày mới nằm NGOÀI phạm vi 31 ngày hiện tại → reset cả 2 ô ngày
	needReset := !newFrom.IsZero() && !oldFrom.IsZero() && !oldTo.IsZero() &&
		(newFrom.Before(oldFrom) || newFrom.After(oldTo))

	if needReset {
		fmt.Println("🔄 Ngày mới ngoài phạm vi 31 ngày → xóa trắng ô ngày để reset anchor")
		_ = rod.Try(func() {
			// Click nút X (clear) trên cả 2 DatePicker để xóa anchor cũ
			c.Page.MustEval(`() => {
				document.querySelectorAll('.ant-calendar-picker-clear').forEach(btn => {
					btn.style.opacity = '1';
					btn.style.display = 'block';
					btn.click();
				});
			}`)
		})
		time.Sleep(500 * time.Millisecond)
	}

	// Nhập FROM trước, rồi TO sau
	if err := c.selectDateOnCalendar("#tngay input", fromDate); err != nil {
		if stream != nil {
			stream <- models.StreamData{Status: "❌ Lỗi chọn ngày bắt đầu: " + err.Error(), IsDone: true}
		}
		return nil, err
	}
	if err := c.selectDateOnCalendar("#dngay input", toDate); err != nil {
		if stream != nil {
			stream <- models.StreamData{Status: "❌ Lỗi chọn ngày kết thúc: " + err.Error(), IsDone: true}
		}
		return nil, err
	}
	err := rod.Try(func() {
		c.Page.MustElement("button.ant-btn-primary[type='submit']").MustClick()
	})
	if err != nil {
		if stream != nil {
			stream <- models.StreamData{Status: "❌ Không thể click nút tìm kiếm", IsDone: true}
		}
		return nil, fmt.Errorf("không thể click nút tìm kiếm: %v", err)
	}

	// Đợi 1.5s để cho website xử lý request → notification kịp xuất hiện
	time.Sleep(1500 * time.Millisecond)
	// Kiểm tra lỗi ngay sau click (trước khi bắt đầu vòng poll)
	if hasErr, errEl, _ := c.Page.Has(".ant-notification-notice-message"); hasErr {
		var earlyErr string
		_ = rod.Try(func() { earlyErr = strings.TrimSpace(errEl.MustText()) })
		if earlyErr != "" {
			fmt.Printf("❌ Phát hiện lỗi sớm sau search: %s\n", earlyErr)
			if stream != nil {
				stream <- models.StreamData{Status: "❌ Lỗi từ hệ thống thuế: " + earlyErr, IsDone: true}
			}
			return allInvoices, fmt.Errorf("lỗi website thuế: %s", earlyErr)
		}
	}

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
			// FIX: Phát hiện lỗi website thuế (Ant Design notification "Request failed with status code 500")
			// Dùng rod.Try + MustText() thay vì .Text() để tránh bỏ qua lỗi do điều kiện quá chặt
			if hasErr, errEl, _ := c.Page.Has(".ant-notification-notice-message"); hasErr {
				var errMsg string
				_ = rod.Try(func() {
					errMsg = strings.TrimSpace(errEl.MustText())
				})
				if errMsg != "" {
					fmt.Printf("❌ Website thuế báo lỗi: %s\n", errMsg)
					if stream != nil {
						stream <- models.StreamData{
							Status: "❌ Lỗi từ hệ thống thuế: " + errMsg,
							IsDone: true,
						}
					}
					return allInvoices, fmt.Errorf("lỗi website thuế: %s", errMsg)
				}
			}

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
					if stream != nil {
						stream <- models.StreamData{
							Status: "📭 Không tìm thấy hóa đơn nào trong khoảng thời gian này.",
							IsDone: true,
						}
					}
					return allInvoices, nil
				}

				// Gửi tổng số lượng tìm thấy (Chỉ gửi 1 lần duy nhất ở trang 1)
				if !hasSentTotal && stream != nil && totalOnTax > 0 {
					stream <- models.StreamData{
						Status:     fmt.Sprintf("🔎 Tìm thấy tổng cộng %d hóa đơn", totalOnTax),
						TotalOnTax: totalOnTax,
						IsDone:     false,
					}
					hasSentTotal = true
				}

				foundData = true
				break
			}
			time.Sleep(300 * time.Millisecond) // Giảm từ 500ms → 300ms để bắt notification kịp
		}
		// Gửi trạng thái trang hiện tại
		if stream != nil {
			stream <- models.StreamData{
				Status:      fmt.Sprintf("🔄 Đang xử lý trang %d...", pageIdx),
				CurrentPage: pageIdx,
				IsDone:      false,
			}
			// time.Sleep(700 * time.Millisecond)
		}

		if !foundData {
			// Gửi IsDone:true trước khi return để C#/JS biết search đã kết thúc
			errMsg := fmt.Sprintf("Timeout chờ kết quả trang %d — website thuế có thể đang bị lỗi", pageIdx)
			if stream != nil {
				stream <- models.StreamData{Status: "❌ " + errMsg, IsDone: true}
			}
			return allInvoices, fmt.Errorf(errMsg)
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
				time.Sleep(300 * time.Millisecond)
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
				STT:              cols[0].String(),
				MSTNguoiBan:      cols[1].String(),
				KyHieuMauSo:      cols[2].String(),
				KyHieu:           cols[3].String(),
				SoHoaDon:         cols[4].String(),
				NgayLap:          cols[5].String(),
				ThongTinHoaDon:   cols[6].String(),
				TongChuaThue:     cols[7].String(),
				TongTienThue:     cols[8].String(),
				ChietKhau:        cols[9].String(),
				TongPhi:          cols[10].String(),
				TongThanhToan:    cols[11].String(),
				DonViTienTe:      cols[12].String(),
				TrangThai:        cols[13].String(),
				KetQuaXuLy:       cols[14].String(), // Chỉnh lại cho đúng thứ tự
				KetQuaKiemTra:    cols[15].String(), // Chỉnh lại cho đúng thứ tự
				HoaDonLienQuan:   cols[16].String(), // Thêm cho đủ cột 16
				ThongTinLienQuan: cols[17].String(), // Thêm cho đủ cột 17
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

// selectDateOnCalendar: mở popup lịch → gõ ngày vào ô .ant-calendar-input → Enter
func (c *CrawlerService) selectDateOnCalendar(selector string, dateStr string) error {
	parts := strings.Split(dateStr, "/")
	if len(parts) != 3 {
		return fmt.Errorf("định dạng ngày không hợp lệ: %s", dateStr)
	}

	fmt.Printf("📅 Đang chọn %s cho ô %s...\n", dateStr, selector)

	// 1. Click ô date picker để mở popup calendar
	var clickErr error
	for retry := 0; retry < 3; retry++ {
		clickErr = rod.Try(func() {
			el := c.Page.MustElement(selector)
			el.MustWaitVisible()
			el.MustClick()
		})
		if clickErr == nil {
			break
		}
		fmt.Printf("⚠️ Retry click ô lịch lần %d: %v\n", retry+1, clickErr)
		time.Sleep(500 * time.Millisecond)
	}
	if clickErr != nil {
		return fmt.Errorf("không thể mở ô lịch %s: %v", selector, clickErr)
	}
	time.Sleep(500 * time.Millisecond)

	// 2. Tìm ô input bên trong popup calendar và gõ ngày
	err := rod.Try(func() {
		calInput := c.Page.MustElement(".ant-calendar-input")
		calInput.MustWaitVisible()
		calInput.MustSelectAllText().MustInput(dateStr)
		fmt.Printf("✅ Đã nhập ngày %s vào ô calendar\n", dateStr)
	})
	if err != nil {
		return fmt.Errorf("không thể nhập ngày vào calendar: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// 3. Gỡ khóa disabled-cell (bypass giới hạn 30 ngày từ anchor cũ)
	_ = rod.Try(func() {
		c.Page.MustEval(`() => {
			document.querySelectorAll('.ant-calendar-disabled-cell').forEach(cell => {
				cell.classList.remove('ant-calendar-disabled-cell');
				const d = cell.querySelector('.ant-calendar-date');
				if (d) d.removeAttribute('aria-disabled');
			});
		}`)
	})
	time.Sleep(200 * time.Millisecond)

	// 4. Click ngày bằng JavaScript (bypass React disabled handler)
	_ = rod.Try(func() {
		dayst := strings.TrimLeft(parts[0], "0")
		c.Page.MustEval(`(day) => {
			const cells = document.querySelectorAll('.ant-calendar-table .ant-calendar-cell');
			for (const cell of cells) {
				const cls = cell.className || '';
				if (cls.includes('last-month-cell') || cls.includes('next-month-btn-day')) continue;
				const dateDiv = cell.querySelector('.ant-calendar-date');
				if (dateDiv && dateDiv.textContent.trim() === day) {
					dateDiv.click();
					break;
				}
			}
		}`, dayst)
		fmt.Printf("✅ Đã click chọn ngày %s\n", dayst)
	})

	time.Sleep(500 * time.Millisecond)
	return nil
}

// SetupBrowserDownload: Chỉ cấu hình browser download đến Invoices_Temp.
// Dùng khi SEARCH — không tạo/set OutputDir.
func (c *CrawlerService) SetupBrowserDownload() {
	homeDir, _ := os.UserHomeDir()
	downloadDir := filepath.Join(homeDir, "Downloads", "Invoices_Temp")
	os.MkdirAll(downloadDir, os.ModePerm)
	proto.BrowserSetDownloadBehavior{
		Behavior:      proto.BrowserSetDownloadBehaviorBehaviorAllow,
		DownloadPath:  downloadDir,
		EventsEnabled: true,
	}.Call(c.Page)
	fmt.Printf("📲 Browser download đã được cấu hình vào: %s\n", downloadDir)
}

// GrantPermissions: Setup browser download VÀ tạo OutputDir.
// Dùng khi EXPORT XML (CrawlAllPages).
func (c *CrawlerService) GrantPermissions() {
	homeDir, _ := os.UserHomeDir()

	downloadDir := filepath.Join(homeDir, "Downloads", "Invoices_Temp")

	// Chỉ tạo thư mục timestamp khi OutputDir chưa được set từ API
	// (tránh ghi đè đường dẫn mà người dùng đã chọn)
	if c.OutputDir == "" {
		timeStr := time.Now().Format("20060102_150405")
		c.OutputDir = filepath.Join(homeDir, "Downloads", "Invoices_Output_"+timeStr)
		fmt.Printf("⚠️  OutputDir chưa được set, dùng mặc định: %s\n", c.OutputDir)
	} else {
		fmt.Printf("✅ Dùng OutputDir đã chọn: %s\n", c.OutputDir)
	}

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
	// FIX: SearchAndGetInvoices kết thúc ở trang CUỐI → phải quay về trang 1
	// Website dùng custom button với icon anticon-left (không có .ant-pagination-item-1)
	c.CurrentStatus = "Đang quay về trang đầu tiên..."
	fmt.Println("⏮️ Đang reset về trang 1 bằng nút mũi tên trái...")

	for i := 0; i < 200; i++ {
		prevBtn, err := c.Page.ElementX("//button[.//i[contains(@class,'anticon-left')]]")
		if err != nil {
			fmt.Println("✅ Không tìm thấy nút Prev — đã về trang 1 hoặc chỉ có 1 trang.")
			break
		}
		if disabled, _ := prevBtn.Attribute("disabled"); disabled != nil {
			fmt.Println("✅ Nút Prev disabled — đã về trang 1.")
			break
		}
		prevBtn.MustClick()
		time.Sleep(500 * time.Millisecond)
	}
	time.Sleep(1 * time.Second)

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

// SoftLogout: Đăng xuất khỏi trang thuế NHƯNG GIỮ Chrome mở.
func (s *CrawlerService) SoftLogout() error {
	if s.Page == nil {
		return fmt.Errorf("trình duyệt chưa mở")
	}

	fmt.Println("🔄 Đang đăng xuất khỏi trang thuế (giữ Chrome)...")

	// 1. Click nút đăng xuất
	_ = rod.Try(func() {
		btns := s.Page.Timeout(1 * time.Second).MustElements(`button[class*="styles__IconButton"]`)
		if len(btns) > 0 {
			btns[1].MustClick()
			fmt.Printf("✅ Đã click nút đăng xuất (tìm thấy %d nút)\n", len(btns))
		}
	})

	return nil
}

// Shutdown: Đóng hoàn toàn trình duyệt Chrome. Dùng khi thoát app.
func (s *CrawlerService) Shutdown() error {
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
	s.Page = nil

	fmt.Println("Hệ thống: Đã đóng trình duyệt Chrome.")
	return nil
}
