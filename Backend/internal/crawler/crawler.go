package crawler

import (
	"TAXSOLUTION/internal/captcha"
	"TAXSOLUTION/internal/logger"
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
	"sync"
	"time"
	"unicode"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

type CrawlerService struct {
	Browser            *rod.Browser
	Page               *rod.Page
	OutputDir          string
	IsRunning          bool   // Đang chạy hay không
	CurrentStatus      string // Thông báo trạng thái (ví dụ: "Đang tải trang 2...")
	TotalInvoices      int    // Tổng số hóa đơn đã tải được
	StopRequested      bool   // Cờ yêu cầu dừng tiến trình
	CurrentTaxCode     string // Mã số thuế hiện tại
	CurrentInvoiceType string // Chiều hóa đơn: ban_ra hoặc mua_vao
	CurrentDateRange   string // Khoảng ngày: vd 01/10/2026-31/10/2026

	// Captcha khi tìm kiếm hóa đơn MTT: kênh nhận mã người dùng nhập (nil = không có captcha đang chờ)
	searchCaptchaMu sync.Mutex
	searchCaptchaCh chan string

	// Captcha khi chuyển trang (MTT)
	pagerCaptchaOn     bool
	pagerCaptchaCtx    context.Context
	pagerCaptchaStream chan models.StreamData
	pagerCaptchaErr    error

	// Hình thức tra cứu gần nhất (để luồng tải chi tiết biết có cần xử lý captcha MTT) và chế độ chỉ-AI (không có popup nhập tay)
	searchCategory  string
	captchaNoManual bool

	// Thời điểm lần tìm kiếm gần nhất có dữ liệu (để nghỉ 5s trước khi chuyển tab)
	lastDataAt time.Time
}

// logError: Ghi lỗi 1 dòng vào %AppData%\TaxSolution\logs\error.log
func (c *CrawlerService) logError(step string, err error, detail string) {
	taxCode := c.CurrentTaxCode
	invType := c.CurrentInvoiceType
	dateRange := c.CurrentDateRange
	errStr := "Unknown error"
	if err != nil {
		errStr = err.Error()
	}
	logger.LogError(taxCode, invType, dateRange, step, errStr, detail)
}

// Stop: Gửi tín hiệu dừng tiến trình đang chạy (Search hoặc Crawl)
func (c *CrawlerService) Stop() {
	c.StopRequested = true
	c.CurrentStatus = "Đã nhận lệnh hủy..."
	fmt.Println("🛑 Đã nhận tín hiệu dừng từ người dùng.")
}

// EnsureBrowser: Tự động khởi động Chrome nếu chưa có hoặc sau khi bị Shutdown
func (s *CrawlerService) EnsureBrowser() error {
	if s.Browser != nil {
		return nil
	}
	l := launcher.New().
		Leakless(false).
		Headless(false) // Chạy ngầm Chrome không hiện cửa sổ

	// Tự tìm Chrome/Chromium/Edge được cài trên máy thay vì hardcode 1 đường dẫn duy nhất
	var browserBin string
	if chromePath, found := launcher.LookPath(); found {
		browserBin = chromePath
	} else {
		candidates := []string{
			"C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
			"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Google\\Chrome\\Application\\chrome.exe"),
			"C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe",
			"C:\\Program Files\\Microsoft\\Edge\\Application\\msedge.exe",
		}
		for _, cand := range candidates {
			if cand != "" {
				if _, err := os.Stat(cand); err == nil {
					browserBin = cand
					break
				}
			}
		}
	}

	if browserBin != "" {
		l = l.Bin(browserBin)
		fmt.Printf("💻 Dùng trình duyệt từ: %s\n", browserBin)
	} else {
		fmt.Println("⚠️ Không tìm thấy Chrome, thử đường dẫn mặc định")
		l = l.Bin("C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe")
	}
	l.Append("--start-maximized")

	// Chrome chặn trang tải nhiều file liên tiếp ("Needs permission to download") → bật sẵn quyền "Tự động tải nhiều file"
	// cho hồ sơ Chrome của bot để việc tải hàng loạt hóa đơn không bị chặn giữa chừng.
	l = l.Preferences(`{"profile":{"default_content_setting_values":{"automatic_downloads":1}},"download":{"prompt_for_download":false}}`)

	url, err := l.Launch()
	if err != nil {
		return fmt.Errorf("không thể khởi chạy Chrome: %v", err)
	}
	browser := rod.New().ControlURL(url)
	err = browser.Connect()
	if err != nil {
		return fmt.Errorf("không thể kết nối Chrome: %v", err)
	}
	s.Browser = browser
	return nil
}

// Khởi tạo CrawlerService
func NewCrawlerService() *CrawlerService {
	s := &CrawlerService{}
	_ = s.EnsureBrowser()
	return s
}

// Hàm thực hiện đăng nhập và lấy dữ liệu
func (s *CrawlerService) ShowLogin() error {
	if s.Browser == nil {
		if err := s.EnsureBrowser(); err != nil {
			return err
		}
	}
	fmt.Println("Đang mở trang chủ...")

	// Khởi tạo trang và gán vào s.Page để dùng cho các hàm Login, Crawl sau này
	err := rod.Try(func() {
		s.Page = s.Browser.MustPage("https://hoadondientu.gdt.gov.vn/")
		s.Page.MustWaitLoad()
	})
	if err != nil {
		return fmt.Errorf("không thể mở trang thuế: %v", err)
	}

	// Đợi ngắn để trang tải xong và popup thông báo (nếu có) kịp hiển thị
	time.Sleep(1000 * time.Millisecond)

	// Đóng popup thông báo nếu có
	s.DismissAnnouncements()

	// Thử mở dialog đăng nhập ngay khi khởi tạo
	_ = s.OpenLoginDialog()

	return nil
}

// ShowLoginDialog: Mở dialog đăng nhập trên TAB HIỆN TẠI (không mở tab mới).
// Dùng sau SoftLogout khi page đã ở trang chủ, hoặc khi UI yêu cầu mở lại form login.
func (s *CrawlerService) ShowLoginDialog() error {
	if s.Page == nil || s.Browser == nil {
		return s.ShowLogin()
	}

	return s.OpenLoginDialog()
}

// OpenLoginDialog: Đảm bảo mở dialog đăng nhập trên tab hiện tại một cách tin cậy
func (s *CrawlerService) OpenLoginDialog() error {
	if s.Page == nil {
		return fmt.Errorf("trình duyệt chưa mở")
	}

	fmt.Println("Đang kiểm tra và mở Dialog đăng nhập...")

	// 1. Nếu Dialog Đăng nhập ĐÃ ĐANG MỞ (có ô #username hoặc input đang visible) -> Xong ngay!
	if hasModal, _, _ := s.Page.Has(".ant-modal-content"); hasModal {
		isAlreadyOpen, _ := s.Page.Eval(`() => {
			const m = document.querySelector('.ant-modal-content');
			if (!m) return false;
			return !!(m.querySelector('#username') || m.querySelector('input'));
		}`)
		if isAlreadyOpen != nil && isAlreadyOpen.Value.Bool() {
			fmt.Println("🆗 Dialog Đăng nhập đã hiển thị sẵn!")
			return nil
		}
	}

	// 2. Đóng popup thông báo nếu có và chờ backdrop đóng hẳn
	s.DismissAnnouncements()

	// 3. Thử mở Dialog Đăng nhập bằng cách click vào nút "Đăng nhập" ở thanh header
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		// Kiểm tra lại nếu modal đã mở
		if hasModal, _, _ := s.Page.Has(".ant-modal-content"); hasModal {
			isAlreadyOpen, _ := s.Page.Eval(`() => {
				const m = document.querySelector('.ant-modal-content');
				if (!m) return false;
				return !!(m.querySelector('#username') || m.querySelector('input'));
			}`)
			if isAlreadyOpen != nil && isAlreadyOpen.Value.Bool() {
				fmt.Println("🆗 Dialog Đăng nhập đã hiển thị thành công!")
				return nil
			}
		}

		// A. Click bằng Rod trực tiếp vào phần tử chứa chữ "Đăng nhập" ở trang chủ (ngoài modal)
		_ = rod.Try(func() {
			loginBtn := s.Page.Timeout(3 * time.Second).MustElementX("//*[not(ancestor::*[contains(@class, 'ant-modal')]) and contains(text(), 'Đăng nhập')]")
			loginBtn.MustWaitVisible().MustClick()
		})

		// B. Hỗ trợ thêm bằng JS click vào phần tử lá (leaf node) chứa chữ "Đăng nhập"
		_ = rod.Try(func() {
			s.Page.Eval(`() => {
				const candidates = Array.from(document.querySelectorAll("span, a, button, li, p, div"));
				for (const el of candidates) {
					if (el.closest('.ant-modal') || el.closest('.ant-modal-wrap')) continue;
					if ((el.innerText || el.textContent || '').trim() === 'Đăng nhập' && el.children.length === 0) {
						el.click();
						if (el.parentElement) el.parentElement.click();
						return;
					}
				}
				for (const el of candidates) {
					if (el.closest('.ant-modal') || el.closest('.ant-modal-wrap')) continue;
					if ((el.innerText || el.textContent || '').trim() === 'Đăng nhập') {
						el.click();
						return;
					}
				}
			}`)
		})

		// C. Đợi Dialog đăng nhập hiện lên
		waitErr := rod.Try(func() {
			s.Page.Timeout(3 * time.Second).MustElement(".ant-modal-content").MustWaitVisible()
		})
		if waitErr == nil {
			fmt.Println("[DEBUG] Dialog Đăng nhập đã hiện thành công!")
			return nil
		}

		lastErr = waitErr
		fmt.Printf("⚠️ Lần %d mở dialog đăng nhập chưa xong (%v), thử lại...\n", attempt, waitErr)
		time.Sleep(300 * time.Millisecond)
	}

	return fmt.Errorf("không thể mở dialog đăng nhập: %v", lastErr)
}

// DismissAnnouncements: Đóng popup thông báo/cảnh báo từ Cục Thuế bằng cách click nút đóng tự nhiên của trang.
// Tuyệt đối không xóa phần tử DOM để tránh làm crash React/Ant Design.
func (s *CrawlerService) DismissAnnouncements() {
	if s.Page == nil {
		return
	}

	_ = rod.Try(func() {
		closed, _ := s.Page.Eval(`() => {
			const modals = document.querySelectorAll('.ant-modal-content');
			for (const m of modals) {
				// TUYỆT ĐỐI BỎ QUA nếu là modal đăng nhập (chứa #username hoặc #password hoặc input)
				if (m.querySelector('#username') || m.querySelector('#password') || m.querySelector('input')) {
					continue;
				}

				const txt = (m.innerText || '').toLowerCase();
				// Nhận diện modal thông báo của Cục Thuế
				if (txt.includes('thông báo') || txt.includes('giả mạo') || txt.includes('cảnh báo') || txt.includes('quảng cáo')) {
					const closeBtn = m.querySelector('.ant-modal-close, .ant-modal-close-x, button[aria-label="Close"]') ||
					                 m.parentElement?.querySelector('.ant-modal-close, .ant-modal-close-x');
					if (closeBtn) {
						closeBtn.click();
						return true;
					}
				}
			}
			return false;
		}`)

		if closed != nil && closed.Value.Bool() {
			fmt.Println("✅ Đã đóng popup thông báo của Cục Thuế.")
			time.Sleep(500 * time.Millisecond)
		}
	})
}

func (s *CrawlerService) GetCaptcha() (string, error) {
	fmt.Println("📸 Đang lấy mã Captcha TRONG khung Đăng nhập...")

	if s.Page == nil {
		return "", fmt.Errorf("trình duyệt chưa mở")
	}

	// Retry tối đa 2 lần, mỗi lần timeout ngắn để không làm C# bị timeout (15s)
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		var encoded string
		err := rod.Try(func() {
			// 1. Chờ Dialog đăng nhập hiện lên (phải có #username)
			s.Page.Timeout(5 * time.Second).MustElement(".ant-modal-content #username").MustWaitVisible()

			// 2. Tìm ảnh captcha linh hoạt
			var el *rod.Element
			imgEl, errImg := s.Page.Timeout(5 * time.Second).Element(".ant-modal-content img[class*='Captcha'], .ant-modal-content img.Captcha__Image-sc-1up1k1e-1")
			if errImg == nil {
				el = imgEl
			} else {
				el = s.Page.Timeout(3 * time.Second).MustElement(".ant-modal-content img")
			}

			// 3. Đợi ảnh render xong
			el.MustWaitVisible()
			time.Sleep(300 * time.Millisecond)

			// 4. Chụp ảnh vào thư mục tạm (tránh lỗi quyền ở Program Files)
			tmpFile := filepath.Join(os.TempDir(), "taxsolution_captcha.png")
			imgData := el.MustScreenshot(tmpFile)
			encoded = base64.StdEncoding.EncodeToString(imgData)
			os.Remove(tmpFile)
		})
		if err == nil && encoded != "" {
			fmt.Println("✅ Đã lấy xong mã Captcha!")
			return encoded, nil
		}
		lastErr = err
		fmt.Printf("⚠️ Lần %d lấy captcha chưa được: %v, thử lại...\n", attempt, err)
		time.Sleep(400 * time.Millisecond)
	}

	return "", fmt.Errorf("không thể lấy captcha: %v", lastErr)
}
func (c *CrawlerService) Login(user, pass, manualCaptcha string) error {
	fmt.Println("🚀 Đang thực hiện đăng nhập (Chế độ kiểm soát nghiêm ngặt)...")
	c.CurrentTaxCode = strings.TrimSpace(user)

	if c.Page == nil {
		err := fmt.Errorf("TRÌNH DUYỆT CHƯA MỞ: Vui lòng gọi /init trước")
		c.logError("loginTax", err, "Page is nil")
		return err
	}

	isAutoSolve := strings.TrimSpace(manualCaptcha) == "" && captcha.Enabled()
	maxAttempts := 1
	if isAutoSolve {
		maxAttempts = 3 // Thử tự động tối đa 3 lần với AI nếu portal báo sai captcha
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if isAutoSolve && attempt > 1 {
			fmt.Printf("🔄 [Hệ thống Login] Thử lại lần %d/%d (lấy mã captcha mới và giải lại)...\n", attempt, maxAttempts)
			time.Sleep(1 * time.Second)
		}

		// --- 1. ĐIỀN THÔNG TIN ---
		// Sử dụng MustClick trước khi Input để React nhận diện Focus State
		c.Page.MustElement(".ant-modal-content #username").MustClick().MustSelectText(".*").MustInput(user)
		c.Page.MustElement(".ant-modal-content #password").MustClick().MustSelectText(".*").MustInput(pass)

		// Tự động giải captcha bằng AI nếu chưa có mã hoặc mã rỗng
		finalCaptcha := strings.TrimSpace(manualCaptcha)
		if isAutoSolve {
			fmt.Printf("🤖 [Lần %d] Chưa có mã Captcha, tự động giải bằng Gemini AI...\n", attempt)
			capBase64, errCap := c.GetCaptcha()
			if errCap != nil || capBase64 == "" {
				lastErr = fmt.Errorf("không thể lấy ảnh captcha: %v", errCap)
				continue
			}

			cleanCap := capBase64
			if idx := strings.Index(cleanCap, ","); idx != -1 {
				cleanCap = cleanCap[idx+1:]
			}

			imgBytes, errDec := base64.StdEncoding.DecodeString(cleanCap)
			if errDec != nil {
				lastErr = fmt.Errorf("không thể decode ảnh captcha: %v", errDec)
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			aiCode, aiErr := captcha.SolveCaptchaImage(ctx, imgBytes)
			cancel()

			if aiErr == nil && len(aiCode) == 6 {
				fmt.Printf("🤖 Gemini giải captcha đăng nhập thành công: %s\n", aiCode)
				finalCaptcha = aiCode
			} else {
				fmt.Printf("⚠️ Gemini không giải được captcha đăng nhập (lỗi: %v, code: '%s')\n", aiErr, aiCode)
				lastErr = fmt.Errorf("SAI_CAPTCHA: AI không nhận diện được mã")
				continue
			}
		}

		elCap := c.Page.MustElement(".ant-modal-content #cvalue")
		elCap.MustClick().MustSelectText(".*").MustInput(finalCaptcha)

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
		var iterErr error

		for i := 0; i < 90; i++ {
			// ƯU TIÊN 1: KIỂM TRA THÀNH CÔNG (Dựa trên HTML Header thực tế)
			// Check nút Logout (mũi tên phải), icon User, icon Bell
			hasSuccess, _, _ := c.Page.Has(".anticon-arrow-right, .anticon-user, .anticon-bell")

			// Check URL (Chỉ tin thành công nếu URL chứa domain thuế và KHÔNG chứa chữ login/auth)
			currURL := c.Page.MustInfo().URL
			isAuthPage := strings.Contains(strings.ToLower(currURL), "login") || strings.Contains(strings.ToLower(currURL), "auth")

			if hasSuccess && !isAuthPage {
				fmt.Println("✅ Đăng nhập thành công thực tế!")
				return nil
			}

			// Nếu modal đã đóng và không ở auth page -> Thành công
			hasModal, _, _ := c.Page.Has(".ant-modal-content")
			if !hasModal && !isAuthPage && i > 3 {
				fmt.Println("✅ Modal đăng nhập đã đóng - Đăng nhập thành công!")
				return nil
			}

			// ƯU TIÊN 2: KIỂM TRA LỖI (Đợi sau vòng lặp thứ 3 ~ 1.5 giây để tránh lỗi cũ)
			if i > 3 {
				errText, _ := c.Page.Eval(`() => {
					const notis = Array.from(document.querySelectorAll('.ant-notification-notice, .ant-message-error, .ant-message-notice, .ant-form-item-explain-error, .ant-alert-error'));
					for (const n of notis) {
						const t = (n.innerText || n.textContent || '').trim();
						if (t) return t;
					}
					return '';
				}`)

				if errText != nil && errText.Value.String() != "" {
					msg := errText.Value.String()
					fmt.Printf("❌ Thất bại xác thực: %s\n", msg)
					iterErr = parseLoginError(msg)
					break
				}
			}

			if i%10 == 0 && i > 0 {
				fmt.Printf("⏳ Đang chờ hệ thống thuế phản hồi đăng nhập (%ds)...\n", (i*500)/1000)
			}

			time.Sleep(500 * time.Millisecond)
		}

		if iterErr != nil {
			c.logError("loginTax", iterErr, iterErr.Error())
			if strings.Contains(iterErr.Error(), "SAI_THONG_TIN") {
				return iterErr
			}
			if isAutoSolve && strings.Contains(iterErr.Error(), "SAI_CAPTCHA") {
				lastErr = iterErr
				continue
			}
			return iterErr
		}

		// --- 4. CHỐT CHẶN CUỐI CÙNG ---
		// Kiểm tra xem trang có báo lỗi gì còn sót lại không
		finalErr, _ := c.Page.Eval(`() => {
			const notis = Array.from(document.querySelectorAll('.ant-notification-notice, .ant-message-error, .ant-message-notice, .ant-form-item-explain-error, .ant-alert-error'));
			for (const n of notis) {
				const t = (n.innerText || n.textContent || '').trim();
				if (t) return t;
			}
			return '';
		}`)
		if finalErr != nil && finalErr.Value.String() != "" {
			msg := finalErr.Value.String()
			loginErr := parseLoginError(msg)
			c.logError("loginTax", loginErr, msg)
			if strings.Contains(loginErr.Error(), "SAI_THONG_TIN") {
				return loginErr
			}
			if isAutoSolve && strings.Contains(loginErr.Error(), "SAI_CAPTCHA") {
				lastErr = loginErr
				continue
			}
			return loginErr
		}

		lastErr = fmt.Errorf("LOI_DANG_NHAP: Quá thời gian chờ đăng nhập hoặc trang thuế chưa phản hồi")
	}

	if lastErr != nil {
		return lastErr
	}
	timeoutErr := fmt.Errorf("LOI_DANG_NHAP: Quá thời gian chờ đăng nhập hoặc thông tin đăng nhập không hợp lệ")
	c.logError("loginTax", timeoutErr, "Quá thời gian chờ đăng nhập")
	return timeoutErr
}

// parseLoginError phân loại chính xác nguyên nhân lỗi đăng nhập trang thuế
func parseLoginError(msg string) error {
	lower := strings.ToLower(msg)
	// 1. Kiểm tra lỗi thông tin tài khoản / mật khẩu trước tiên
	if strings.Contains(lower, "tên đăng nhập") ||
		strings.Contains(lower, "mật khẩu") ||
		strings.Contains(lower, "tài khoản") ||
		strings.Contains(lower, "password") ||
		strings.Contains(lower, "username") {
		return fmt.Errorf("SAI_THONG_TIN: %s", msg)
	}
	// 2. Kiểm tra lỗi Captcha / Mã xác thực
	if strings.Contains(lower, "captcha") ||
		strings.Contains(lower, "mã xác nhận") ||
		strings.Contains(lower, "mã xác thực") ||
		strings.Contains(lower, "mã bảo vệ") ||
		strings.Contains(lower, "mã kiểm tra") {
		return fmt.Errorf("SAI_CAPTCHA: %s", msg)
	}
	// 3. Các lỗi đăng nhập khác từ trang thuế
	return fmt.Errorf("LOI_DANG_NHAP: %s", msg)
}

// switchTab: Chuyển tab Ant Design trên trang thuế dựa trên selector div[role="tab"].ant-tabs-tab
func (c *CrawlerService) switchTab(exactTargetText, fallbackKeyword string, isSubTab bool) error {
	var targetEl *rod.Element
	var isAlreadyActive bool

	err := rod.Try(func() {
		markerId := "crawler-tab-" + strconv.FormatInt(time.Now().UnixNano(), 10)
		res := c.Page.MustEval(fmt.Sprintf(`() => {
			const exact = "%s".toLowerCase();
			const keyword = "%s".toLowerCase();
			const isSub = %t;
			
			let candidates = [];
			if (isSub) {
				// Chỉ tìm tab con bên trong tabpane cha đang active (.ant-tabs-tabpane-active)
				// Loại trừ các tabpane đang inactive
				const activeOuterPanes = Array.from(document.querySelectorAll('.ant-tabs-tabpane-active'))
					.filter(p => !p.closest('.ant-tabs-tabpane-inactive'));
				
				for (const pane of activeOuterPanes) {
					const tabsInPane = Array.from(pane.querySelectorAll('div[role="tab"].ant-tabs-tab, .ant-tabs-tab'))
						.filter(t => !t.closest('.ant-tabs-tabpane-inactive'));
					candidates.push(...tabsInPane);
				}
				// Fallback nếu không thấy trong active pane
				if (candidates.length === 0) {
					candidates = Array.from(document.querySelectorAll('div[role="tab"].ant-tabs-tab, .ant-tabs-tab'))
						.filter(t => !t.closest('.ant-tabs-tabpane-inactive'));
				}
			} else {
				// Tab chiều lớn (Bán ra / Mua vào): tìm ở cấp ngoài cùng
				candidates = Array.from(document.querySelectorAll('div[role="tab"].ant-tabs-tab, .ant-tabs-tab'))
					.filter(t => {
						const txt = (t.innerText || '').toLowerCase();
						return txt.includes('bán ra') || txt.includes('mua vào');
					});
				if (candidates.length === 0) {
					candidates = Array.from(document.querySelectorAll('div[role="tab"].ant-tabs-tab, .ant-tabs-tab'));
				}
			}

			const targetTab = candidates.find(t => {
				const span = t.querySelector('span');
				const txt = (span ? span.innerText : t.innerText || '').trim().toLowerCase();
				return txt === exact || txt.includes(exact) || (keyword && txt.includes(keyword));
			});

			if (targetTab) {
				const isActive = targetTab.classList.contains('ant-tabs-tab-active') || targetTab.getAttribute('aria-selected') === 'true';
				if (isActive) {
					return "ALREADY_ACTIVE";
				}
				const toClick = targetTab.querySelector('span') || targetTab;
				toClick.setAttribute('data-tab-target', '%s');
				// Kích hoạt sự kiện chuột qua JS
				toClick.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true, view: window }));
				toClick.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true, view: window }));
				toClick.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window }));
				toClick.click();
				return "FOUND_TARGET";
			}
			return "NOT_FOUND";
		}`, exactTargetText, fallbackKeyword, isSubTab, markerId))

		status := res.Str()
		if status == "ALREADY_ACTIVE" {
			isAlreadyActive = true
			return
		}
		if status == "FOUND_TARGET" {
			markerSelector := fmt.Sprintf("[data-tab-target='%s']", markerId)
			if el, errEl := c.Page.Element(markerSelector); errEl == nil && el != nil {
				_ = rod.Try(func() {
					el.MustEval(`() => this.removeAttribute('data-tab-target')`)
				})
				targetEl = el
			}
		}
	})

	if isAlreadyActive {
		fmt.Printf("🎯 Chuyển tab [%s]: ALREADY_ACTIVE\n", exactTargetText)
		return nil
	}

	if targetEl != nil {
		fmt.Printf("🎯 Chuyển tab [%s]: CLICKED (và kích hoạt native click)\n", exactTargetText)
		_ = rod.Try(func() {
			targetEl.MustClick()
		})
		time.Sleep(500 * time.Millisecond)
		return nil
	}

	if err != nil {
		c.logError("switchTab", err, fmt.Sprintf("Lỗi kiểm tra tab [%s]", exactTargetText))
		fmt.Printf("⚠️ Lỗi kiểm tra tab (%s): %v\n", exactTargetText, err)
		return err
	}

	tabNotFoundErr := fmt.Errorf("không tìm thấy tab [%s]", exactTargetText)
	c.logError("switchTab", tabNotFoundErr, fmt.Sprintf("Fallback keyword: %s", fallbackKeyword))
	fmt.Printf("⚠️ Không tìm thấy tab [%s]\n", exactTargetText)
	return nil
}

func (c *CrawlerService) SearchAndGetInvoices(ctx context.Context, fromDate, toDate, invoiceType, category, checkResult string, stream chan models.StreamData) ([]models.Invoice, error) {
	// Giới hạn ngày kết thúc không vượt quá ngày hiện tại (trang thuế không cho chọn ngày tương lai)
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	toT, errTo := time.Parse("02/01/2006", toDate)
	if errTo == nil && toT.After(today) {
		toT = today
		toDate = toT.Format("02/01/2006")
	}

	if category == "may_tinh_tien" {
		fromT, err1 := time.Parse("02/01/2006", fromDate)
		if err1 == nil && errTo == nil && toT.Sub(fromT) > 9*24*time.Hour {
			return c.searchAndGetInvoicesChunked(ctx, fromT, toT, invoiceType, category, checkResult, stream)
		}
	}
	return c.searchAndGetInvoicesSingle(ctx, fromDate, toDate, invoiceType, category, checkResult, stream)
}

func (c *CrawlerService) searchAndGetInvoicesChunked(ctx context.Context, fromT, toT time.Time, invoiceType, category, checkResult string, stream chan models.StreamData) ([]models.Invoice, error) {
	var chunks [][2]time.Time
	for start := fromT; !start.After(toT); start = start.AddDate(0, 0, 10) {
		end := start.AddDate(0, 0, 9)
		if end.After(toT) {
			end = toT
		}
		chunks = append(chunks, [2]time.Time{start, end})
	}

	var allInvoices []models.Invoice
	seen := make(map[string]bool)
	totalCount := 0

	for i, chunk := range chunks {
		if ctx.Err() != nil || c.StopRequested {
			return allInvoices, fmt.Errorf("thao tác đã bị hủy")
		}
		cFrom := chunk[0].Format("02/01/2006")
		cTo := chunk[1].Format("02/01/2006")
		if stream != nil {
			stream <- models.StreamData{
				Status:     fmt.Sprintf("📅 Đang tìm đoạn %d/%d (%s - %s)...", i+1, len(chunks), cFrom, cTo),
				TotalSoFar: len(allInvoices),
				IsDone:     false,
			}
		}

		subStream := make(chan models.StreamData, 10)
		type resType struct {
			invs []models.Invoice
			err  error
		}
		resCh := make(chan resType, 1)

		go func() {
			defer close(subStream)
			invs, err := c.searchAndGetInvoicesSingle(ctx, cFrom, cTo, invoiceType, category, checkResult, subStream)
			resCh <- resType{invs: invs, err: err}
		}()

		for sub := range subStream {
			if sub.IsDone {
				continue
			}
			if stream != nil {
				sub.Status = fmt.Sprintf("[%d/%d] (%s - %s) %s", i+1, len(chunks), cFrom, cTo, sub.Status)
				sub.TotalSoFar = len(allInvoices) + sub.TotalSoFar
				if sub.TotalOnTax > 0 {
					sub.TotalOnTax = len(allInvoices) + sub.TotalOnTax
				}
				stream <- sub
			}
		}

		r := <-resCh
		if r.err != nil && !strings.Contains(r.err.Error(), "Không có kết quả") && len(r.invs) == 0 {
			fmt.Printf("⚠️ Lỗi ở đoạn %s - %s: %v\n", cFrom, cTo, r.err)
		}

		for _, inv := range r.invs {
			key := fmt.Sprintf("%s_%s_%s", inv.MSTNguoiBan, inv.KyHieu, inv.SoHoaDon)
			if !seen[key] {
				seen[key] = true
				totalCount++
				inv.STT = fmt.Sprintf("%d", totalCount)
				allInvoices = append(allInvoices, inv)
			}
		}

		if stream != nil && i < len(chunks)-1 {
			stream <- models.StreamData{
				Status:     fmt.Sprintf("✅ Đoạn %d/%d (%s - %s): xong (%d HĐ). Chuẩn bị quét đoạn %d/%d...", i+1, len(chunks), cFrom, cTo, len(r.invs), i+2, len(chunks)),
				TotalSoFar: len(allInvoices),
				IsDone:     false,
			}
		}

		if i < len(chunks)-1 {
			time.Sleep(1500 * time.Millisecond)
		}
	}

	if stream != nil {
		stream <- models.StreamData{
			Status:     fmt.Sprintf("Hoàn thành: tìm thấy tổng cộng %d hóa đơn qua %d đoạn!", len(allInvoices), len(chunks)),
			Invoices:   allInvoices,
			TotalSoFar: len(allInvoices),
			TotalOnTax: len(allInvoices),
			IsDone:     true,
		}
	}
	return allInvoices, nil
}

func (c *CrawlerService) searchAndGetInvoicesSingle(ctx context.Context, fromDate, toDate, invoiceType, category, checkResult string, stream chan models.StreamData) ([]models.Invoice, error) {
	if c.Page == nil {
		if c.Browser == nil {
			return nil, fmt.Errorf("browser chưa được khởi tạo")
		}
		c.Page = c.Browser.MustPage()
	}

	// MTT: bật xử lý captcha mỗi lần chuyển trang trong suốt lượt tra cứu này
	c.pagerCaptchaOn = category == "may_tinh_tien"
	c.pagerCaptchaCtx = ctx
	c.pagerCaptchaStream = stream
	c.pagerCaptchaErr = nil
	c.searchCategory = category
	defer func() { c.pagerCaptchaOn = false; c.pagerCaptchaStream = nil }()

	var allInvoices []models.Invoice
	seen := make(map[string]bool)
	c.IsRunning = true
	c.TotalInvoices = 0
	c.StopRequested = false
	c.CurrentInvoiceType = invoiceType
	c.CurrentDateRange = fmt.Sprintf("%s-%s", fromDate, toDate)

	if stream != nil {
		stream <- models.StreamData{Status: "🚀 Đang khởi động ứng dụng...", IsDone: false}
	}

	defer func() {
		c.IsRunning = false
		if c.CurrentStatus != "Lỗi" && c.CurrentStatus != "Không có kết quả" && c.CurrentStatus != "Đã hủy" {
			c.CurrentStatus = "Hoàn thành"
		}
	}()

	// --- 1. ĐIỀU HƯỚNG ---
	c.CurrentStatus = "Đang mở menu Tra cứu..."

	// Kiểm tra Page còn valid không
	if c.Page == nil {
		errNil := fmt.Errorf("page is nil - trình duyệt chưa sẵn sàng")
		c.logError("navigation", errNil, "Trình duyệt chưa sẵn sàng")
		if stream != nil {
			stream <- models.StreamData{Status: "❌ Trình duyệt chưa sẵn sàng", IsDone: true}
		}
		return nil, errNil
	}

	// Kiểm tra xem đã ở trang Tra cứu chưa — nếu rồi thì bỏ qua navigate
	currentURL := ""
	_ = rod.Try(func() { currentURL = c.Page.MustInfo().URL })
	alreadyOnSearch := strings.Contains(currentURL, "tra-cuu")

	// Đóng popup thông báo nếu có xuất hiện
	c.DismissAnnouncements()

	if !alreadyOnSearch {
		navErr := rod.Try(func() {
			c.Page.MustElementR(".ml-menu-item span", "Tra cứu").MustClick()
			c.Page.MustElement("a[href*='/tra-cuu/tra-cuu-hoa-don']").MustWaitVisible().MustClick()
			c.Page.MustWaitIdle()
		})
		if navErr != nil {
			c.logError("navigation", navErr, "Không thể mở trang tra cứu hóa đơn")
			if stream != nil {
				stream <- models.StreamData{Status: "❌ Không thể mở trang tra cứu", IsDone: true}
			}
			return nil, fmt.Errorf("lỗi điều hướng: %v", navErr)
		}
	} else {
		fmt.Println("📄 Đã ở trang Tra cứu, bỏ qua điều hướng.")
	}

	// Lần tìm kiếm trước có dữ liệu → nghỉ đủ 5s (tính từ lúc có dữ liệu) rồi mới click chuyển tab
	if !c.lastDataAt.IsZero() {
		if remain := 5*time.Second - time.Since(c.lastDataAt); remain > 0 {
			if stream != nil {
				stream <- models.StreamData{Status: "⏳ Nghỉ vài giây trước khi chuyển tab...", IsDone: false}
			}
			fmt.Printf("⏳ Nghỉ %.1fs trước khi chuyển tab...\n", remain.Seconds())
			time.Sleep(remain)
		}
		c.lastDataAt = time.Time{}
	}

	// --- 1.1. CHỌN TAB CHIỀU HÓA ĐƠN (BÁN RA / MUA VÀO) ---
	if invoiceType == "mua_vao" {
		if stream != nil {
			stream <- models.StreamData{Status: "🎯 Đang chọn tab: Hóa đơn mua vào...", IsDone: false}
		}
		_ = c.switchTab("Tra cứu hóa đơn điện tử mua vào", "mua vào", false)
	} else {
		if stream != nil {
			stream <- models.StreamData{Status: "🎯 Đang chọn tab: Hóa đơn bán ra...", IsDone: false}
		}
		_ = c.switchTab("Tra cứu hóa đơn điện tử bán ra", "bán ra", false)
	}

	// --- 1.2. CHỌN TAB HÌNH THỨC (HÓA ĐƠN ĐIỆN TỬ / MÁY TÍNH TIỀN) ---
	if category == "may_tinh_tien" {
		if stream != nil {
			stream <- models.StreamData{Status: "🖩 Đang chọn hình thức: Từ máy tính tiền...", IsDone: false}
		}
		_ = c.switchTab("Hóa đơn có mã khởi tạo từ máy tính tiền", "máy tính tiền", true)
	} else {
		_ = c.switchTab("Hóa đơn điện tử", "hóa đơn điện tử", true)
	}

	// --- 1.3. CHỌN KẾT QUẢ KIỂM TRA ---
	// Luôn chủ động chọn đúng mục trên trang thuế (kể cả "Tất cả"), vì trang thuế có thể đang
	// ở giá trị khác (vd: "Cục Thuế đã nhận") do lần tra cứu trước hoặc mặc định của hệ thống.
	// selectCheckResult tự bỏ qua nếu ô đã đúng giá trị mục tiêu.
	needSelectCheckResult := strings.TrimSpace(checkResult) != ""

	if needSelectCheckResult {
		labelName := checkResult
		switch checkResult {
		case "co_ma":
			labelName = "Đã cấp mã hóa đơn"
		case "khong_ma":
			labelName = "Cục Thuế đã nhận không mã"
		case "may_tinh_tien":
			labelName = "Cục Thuế đã nhận hóa đơn có mã khởi tạo từ máy tính tiền"
		}
		if stream != nil {
			stream <- models.StreamData{Status: fmt.Sprintf("🎯 Chọn kết quả kiểm tra: %s...", labelName), IsDone: false}
		}
		if errSel := c.selectCheckResult(checkResult); errSel != nil {
			fmt.Printf("⚠️ Cảnh báo chọn 'Kết quả kiểm tra': %v\n", errSel)
			if stream != nil {
				stream <- models.StreamData{Status: fmt.Sprintf("⚠️ Chưa chọn được '%s', tiếp tục tìm kiếm...", labelName), IsDone: false}
			}
		}
	}

	// --- 2. TÌM KIẾM (chọn ngày thông minh: so sánh với anchor hiện tại) ---
	if stream != nil {
		stream <- models.StreamData{Status: fmt.Sprintf("📅 Chọn khoảng thời gian %s - %s...", fromDate, toDate), IsDone: false}
	}
	// Đọc ngày hiện tại trên trang thuế (để kiểm tra xem có cần reset anchor không)
	var curFrom, curTo string
	_ = rod.Try(func() {
		getVal := func(sel string) string {
			els, err := c.Page.Elements(sel)
			if err == nil && len(els) > 0 {
				for _, el := range els {
					if vis, errV := el.Visible(); errV == nil && vis {
						if v, errA := el.Attribute("value"); errA == nil && v != nil {
							return *v
						}
					}
				}
				if v, errA := els[0].Attribute("value"); errA == nil && v != nil {
					return *v
				}
			}
			return ""
		}
		curFrom = getVal("#tngay input")
		curTo = getVal("#dngay input")
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
			c.Page.MustEval(`() => {
				document.querySelectorAll('.ant-calendar-picker-clear').forEach(btn => {
					btn.style.opacity = '1';
					btn.style.display = 'block';
					btn.click();
				});
			}`)
		})
		time.Sleep(300 * time.Millisecond)
		curFrom = ""
		curTo = ""
	}

	// Nhập FROM trước, rồi TO sau (nếu ô đã đúng ngày thì bỏ qua để tiết kiệm thời gian)
	if curFrom != fromDate {
		if err := c.selectDateOnCalendar("#tngay input", fromDate); err != nil {
			if stream != nil {
				stream <- models.StreamData{Status: "❌ Lỗi chọn ngày bắt đầu: " + err.Error(), IsDone: true}
			}
			return nil, err
		}
	} else {
		fmt.Printf("⏩ Ô Từ ngày đã là %s, bỏ qua nhập lại\n", fromDate)
	}

	if curTo != toDate {
		if err := c.selectDateOnCalendar("#dngay input", toDate); err != nil {
			if stream != nil {
				stream <- models.StreamData{Status: "❌ Lỗi chọn ngày kết thúc: " + err.Error(), IsDone: true}
			}
			return nil, err
		}
	} else {
		fmt.Printf("⏩ Ô Đến ngày đã là %s, bỏ qua nhập lại\n", toDate)
	}
	if stream != nil {
		stream <- models.StreamData{Status: "🔍 Đang tìm kiếm...", IsDone: false}
	}
	time.Sleep(400 * time.Millisecond)

	err := rod.Try(func() {
		// 1. Thử click bằng JS trong tab đang active (bền bỉ, tránh bị popup/overlay che hoặc nằm ngoài viewport)
		var clicked bool
		res, errEval := c.Page.Eval(`() => {
			const buttons = Array.from(document.querySelectorAll("button[type='submit'], button.ant-btn-primary, button"));
			// Chỉ lấy button KHÔNG thuộc tab inactive
			const activeButtons = buttons.filter(b => !b.closest('.ant-tabs-tabpane-inactive'));
			
			// Ưu tiên nút có chữ "Tìm kiếm"
			let btn = activeButtons.find(b => (b.innerText || '').trim().includes('Tìm kiếm'));
			if (!btn) {
				btn = activeButtons.find(b => b.type === 'submit' || b.classList.contains('ant-btn-primary'));
			}
			if (!btn) {
				btn = buttons.find(b => (b.innerText || '').trim().includes('Tìm kiếm'));
			}

			if (btn) {
				btn.scrollIntoView({ block: 'center', inline: 'center' });
				btn.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true, view: window }));
				btn.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true, view: window }));
				btn.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window }));
				btn.click();
				return true;
			}
			return false;
		}`)

		if errEval == nil && res != nil && res.Value.Bool() {
			clicked = true
		}

		if !clicked {
			// 2. Fallback: Dùng Rod native search
			btn, errBtn := c.Page.ElementR("button", "Tìm kiếm")
			if errBtn != nil {
				btn, errBtn = c.Page.Element("button.ant-btn-primary[type='submit'], button[type='submit']")
			}
			if errBtn != nil {
				panic(errBtn)
			}
			btn.MustScrollIntoView()
			btn.MustClick()
		}
		fmt.Println("✅ Đã bấm nút Tìm kiếm thành công!")
	})
	if err != nil {
		if stream != nil {
			stream <- models.StreamData{Status: "❌ Không thể click nút tìm kiếm", IsDone: true}
		}
		return nil, fmt.Errorf("không thể click nút tìm kiếm: %v", err)
	}

	// Hóa đơn MTT: cổng thuế bắt nhập captcha mỗi lần bấm Tìm kiếm → chờ người dùng nhập trên app
	if category == "may_tinh_tien" {
		if errCap := c.handleSearchCaptcha(ctx, stream); errCap != nil {
			if ctx.Err() != nil || c.StopRequested {
				c.CurrentStatus = "Đã hủy"
			} else {
				c.CurrentStatus = "Lỗi"
			}
			if stream != nil {
				stream <- models.StreamData{Status: "❌ " + errCap.Error(), IsDone: true}
			}
			return allInvoices, errCap
		}
	}

	// Đợi website thuế xử lý request: chờ spinner xuất hiện rồi biến mất (tối đa 3 phút)
	time.Sleep(1500 * time.Millisecond)
	if stream != nil && c.isTaxLoading() {
		stream <- models.StreamData{Status: "⏳ Đang chờ hệ thống thuế tải dữ liệu...", IsDone: false}
	}
	if c.waitTaxIdle(ctx, stream, 180*time.Second) {
		c.CurrentStatus = "Đã hủy"
		return allInvoices, fmt.Errorf("thao tác đã bị hủy")
	}
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
		if ctx.Err() != nil || c.StopRequested {
			c.CurrentStatus = "Đã hủy"
			return allInvoices, fmt.Errorf("thao tác đã bị hủy")
		}

		// A. Đợi và lấy thông tin tổng số kết quả
		foundData := false
		// Vòng chờ tối thiểu 40 lần (~12s); nếu trang thuế vẫn đang tải thì chờ tiếp (tối đa ~3 phút)
		for i := 0; i < 40 || (i < 600 && c.isTaxLoading()); i++ {
			if ctx.Err() != nil || c.StopRequested {
				c.CurrentStatus = "Đã hủy"
				return allInvoices, fmt.Errorf("thao tác đã bị hủy")
			}
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

			// Tìm text "Có X kết quả" trên trang (bỏ qua tab inactive)
			var totalText string
			_ = rod.Try(func() {
				res := c.Page.MustEval(`() => {
					const all = Array.from(document.querySelectorAll('span, div, p, b'));
					const visible = all.filter(el => {
						if (el.closest('.ant-tabs-tabpane-inactive')) return false;
						if (el.offsetParent === null && window.getComputedStyle(el).display === 'none') return false;
						const t = (el.innerText || '').trim();
						return /Có\s+\d+\s+kết quả/i.test(t);
					});
					if (visible.length > 0) {
						visible.sort((a, b) => a.innerText.length - b.innerText.length);
						return visible[0].innerText.trim();
					}
					return '';
				}`)
				totalText = res.Str()
			})

			if totalText != "" {
				// Bóc số lượng bằng Regex
				re := regexp.MustCompile(`\d+`)
				match := re.FindString(totalText)
				if match != "" {
					totalOnTax, _ = strconv.Atoi(match)
				}

				if strings.Contains(totalText, "Có 0 kết quả") {
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
			statusText := fmt.Sprintf("🔄 Đang xử lý trang %d...", pageIdx)
			if rc, rt := c.pagerPos(); rt > 0 {
				statusText = fmt.Sprintf("🔄 Đang xử lý trang %d/%d...", rc, rt)
			}
			stream <- models.StreamData{
				Status:      statusText,
				CurrentPage: pageIdx,
				TotalOnTax:  totalOnTax,
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
			return allInvoices, fmt.Errorf("%s", errMsg)
		}

		// B. Cập nhật số trang từ tab đang active
		if pageIdx == 1 {
			_ = rod.Try(func() {
				res := c.Page.MustEval(`() => {
					const panes = Array.from(document.querySelectorAll('.ant-tabs-tabpane-active'));
					const activePane = panes[panes.length - 1] || document;

					// 1. Tìm các nút số trang .ant-pagination-item (1, 2, 3...)
					const items = Array.from(activePane.querySelectorAll('.ant-pagination-item'));
					let maxNum = 0;
					for (const it of items) {
						const n = parseInt((it.innerText || it.textContent || '').trim());
						if (!isNaN(n) && n > maxNum) maxNum = n;
					}
					if (maxNum > 0) return maxNum;

					// 2. Tìm trong PageIndex dạng 1/2
					const el = activePane.querySelector("div[class*='PageIndex'], .ant-pagination-simple-pager");
					if (el) {
						const parts = (el.innerText || '').split('/');
						if (parts.length === 2) {
							const t = parseInt(parts[1].trim());
							if (!isNaN(t) && t > 0) return t;
						}
					}
					return 1;
				}`)
				domPages := res.Int()
				if domPages > maxPages {
					maxPages = domPages
					fmt.Printf("📄 Đọc được số trang từ DOM: %d trang\n", maxPages)
				}
			})
		}

		// C. Đợi bảng cập nhật (từ trang 2)
		if pageIdx > 1 {
			for i := 0; i < 30 || (i < 600 && c.isTaxLoading()); i++ {
				if ctx.Err() != nil || c.StopRequested {
					c.CurrentStatus = "Đã hủy"
					return allInvoices, fmt.Errorf("thao tác đã bị hủy")
				}
				currentNo := c.getFirstInvoiceNoFromUI()
				if currentNo != "" && currentNo != lastFirstInvoiceNo {
					break
				}
				time.Sleep(300 * time.Millisecond)
			}
		}

		// D. BÓC DỮ LIỆU ĐỘNG THEO TIÊU ĐỀ CỘT
		var headers []string
		var rows [][]string

		// Hệ thống thuế chậm: nếu bảng chưa hiển thị dòng nào thì chờ rảnh rồi bóc lại (tối đa 4 lần)
		for scrapeTry := 0; scrapeTry < 4; scrapeTry++ {
			headers = nil
			rows = nil
			if scrapeTry > 0 {
				if ctx.Err() != nil || c.StopRequested {
					break
				}
				c.waitTaxIdle(ctx, stream, 45*time.Second)
				time.Sleep(2 * time.Second)
			}
			_ = rod.Try(func() {
				evalRes := c.Page.MustEval(`() => {
				const allRows = Array.from(document.querySelectorAll(".ant-table-tbody tr.ant-table-row")).filter(r => !r.closest('.ant-tabs-tabpane-inactive'));
				if (allRows.length === 0) return { headers: [], rows: [] };

				let maxCols = 0;
				allRows.forEach(r => {
					const count = r.querySelectorAll("td").length;
					if (count > maxCols) maxCols = count;
				});

				if (maxCols < 5) return { headers: [], rows: [] };

				const validRows = allRows.filter(r => r.querySelectorAll("td").length === maxCols);
				const rows = validRows.map(row => {
					return Array.from(row.querySelectorAll("td")).map(td => (td.innerText || td.textContent || "").trim());
				});

				// Lấy thead tương ứng trực tiếp với bảng chứa validRows
				let ths = [];
				const tableContainer = validRows[0].closest(".ant-table-container, .ant-table") || document;
				const thead = tableContainer.querySelector(".ant-table-header thead, thead");
				if (thead) {
					const trs = thead.querySelectorAll("tr");
					const targetTr = trs.length > 0 ? trs[trs.length - 1] : thead;
					ths = Array.from(targetTr.querySelectorAll("th"));
				}
				if (ths.length === 0) {
					ths = Array.from(document.querySelectorAll(".ant-table-thead th"));
				}
				const headers = ths.map(th => (th.innerText || th.textContent || "").trim().toLowerCase());

				return { headers: headers, rows: rows };
			}`)

				hArr := evalRes.Get("headers").Arr()
				for _, h := range hArr {
					headers = append(headers, strings.ToLower(strings.TrimSpace(h.String())))
				}

				rArr := evalRes.Get("rows").Arr()
				for _, r := range rArr {
					var rowCells []string
					for _, cell := range r.Arr() {
						rowCells = append(rowCells, strings.TrimSpace(cell.String()))
					}
					rows = append(rows, rowCells)
				}
			})
			if len(rows) > 0 {
				break
			}
		}

		fmt.Printf("📦 Trang %d: bóc được %d dòng hóa đơn (nhận diện %d cột tiêu đề).\n", pageIdx, len(rows), len(headers))

		// Tự động tính số trang từ totalOnTax và số dòng mỗi trang nếu có
		if pageIdx == 1 && totalOnTax > 0 && len(rows) > 0 {
			calcPages := (totalOnTax + len(rows) - 1) / len(rows)
			if calcPages > maxPages {
				maxPages = calcPages
				fmt.Printf("📄 Tính toán số trang từ tổng thuế (%d) và pageSize (%d): %d trang\n", totalOnTax, len(rows), maxPages)
			}
		}

		for _, cols := range rows {
			if len(cols) < 5 {
				continue
			}

			getVal := func(idx int) string {
				if idx >= 0 && idx < len(cols) {
					return strings.TrimSpace(cols[idx])
				}
				return ""
			}

			// Hàm tìm giá trị cột theo từ khóa trong tiêu đề
			getByHeader := func(keywords ...string) (string, bool) {
				for idx, h := range headers {
					for _, kw := range keywords {
						if strings.Contains(h, kw) && idx < len(cols) {
							val := strings.TrimSpace(cols[idx])
							if val != "" {
								return val, true
							}
						}
					}
				}
				return "", false
			}

			stt, _ := getByHeader("stt")
			if stt == "" {
				stt = getVal(0)
			}

			// Mã số thuế người bán (loại trừ cột người mua)
			var mst string
			for idx, h := range headers {
				if (strings.Contains(h, "mã số thuế") || strings.Contains(h, "mst")) && !strings.Contains(h, "người mua") && idx < len(cols) {
					val := strings.TrimSpace(cols[idx])
					if val != "" {
						mst = val
						break
					}
				}
			}
			if mst == "" {
				mst = getVal(1)
			}

			// Ký hiệu mẫu số (thường là số: 1, 2...)
			mauSo, _ := getByHeader("mẫu số")
			if mauSo == "" {
				mauSo = getVal(2)
			}

			// Ký hiệu hóa đơn (thường là chữ: C26TNL, 1C21TML...)
			// QUAN TRỌNG: Phải LOẠI TRỪ "mẫu số" vì tiêu đề "Ký hiệu mẫu số" cũng chứa chữ "ký hiệu"!
			var kyHieu string
			for idx, h := range headers {
				if (strings.Contains(h, "ký hiệu hóa đơn") || strings.Contains(h, "ký hiệu hđ") || (strings.Contains(h, "ký hiệu") && !strings.Contains(h, "mẫu số"))) && idx < len(cols) {
					val := strings.TrimSpace(cols[idx])
					if val != "" {
						kyHieu = val
						break
					}
				}
			}
			if kyHieu == "" {
				kyHieu = getVal(3)
			}
			// Nếu kyHieu vẫn bị gán nhầm giá trị của mauSo và cột 3 có giá trị khác, ưu tiên lấy cột 3
			if kyHieu == mauSo && len(mauSo) <= 2 {
				val3 := getVal(3)
				if val3 != "" && val3 != mauSo {
					kyHieu = val3
				}
			}

			// Số hóa đơn (loại trừ "mẫu số")
			var soHD string
			for idx, h := range headers {
				if (strings.Contains(h, "số hóa đơn") || strings.Contains(h, "số hđ")) && !strings.Contains(h, "mẫu số") && idx < len(cols) {
					val := strings.TrimSpace(cols[idx])
					if val != "" {
						soHD = val
						break
					}
				}
			}
			if soHD == "" {
				soHD = getVal(4)
			}

			ngayLap, _ := getByHeader("ngày lập")
			if ngayLap == "" {
				ngayLap = getVal(5)
			}

			thongTin, _ := getByHeader("thông tin", "người mua")
			if thongTin == "" {
				thongTin = getVal(6)
			}

			chuaThue, _ := getByHeader("chưa thuế")
			if chuaThue == "" {
				chuaThue = getVal(7)
			}

			var tienThue string
			for idx, h := range headers {
				if strings.Contains(h, "tiền thuế") && !strings.Contains(h, "chưa thuế") && idx < len(cols) {
					val := strings.TrimSpace(cols[idx])
					if val != "" {
						tienThue = val
						break
					}
				}
			}
			if tienThue == "" {
				tienThue = getVal(8)
			}

			chietKhau, _ := getByHeader("chiết khấu")
			if chietKhau == "" {
				chietKhau = getVal(9)
			}

			// TỔNG TIỀN THANH TOÁN (Số tiền thực tế của hóa đơn)
			// Trên bảng tra cứu hóa đơn của Tổng cục Thuế, Cột 10 luôn là Tổng tiền thanh toán
			var tongThanhToan string
			if val, ok := getByHeader("thanh toán"); ok && val != "" && !isInvoiceStatusText(val) {
				tongThanhToan = val
			} else {
				tongThanhToan = getVal(10)
			}

			// Nếu tongThanhToan bị lấy nhầm chữ trạng thái (ví dụ do lỗi lệch cột), ưu tiên lấy giá trị cột 10
			if isInvoiceStatusText(tongThanhToan) || tongThanhToan == "" {
				col10 := getVal(10)
				if !isInvoiceStatusText(col10) && col10 != "" {
					tongThanhToan = col10
				} else {
					// Dự phòng: Tự động cộng Chưa thuế + Tiền thuế - Chiết khấu
					sum := parseMoney(chuaThue) + parseMoney(tienThue) - parseMoney(chietKhau)
					if sum > 0 {
						tongThanhToan = formatMoney(sum)
					}
				}
			}

			// TRẠNG THÁI HÓA ĐƠN
			// Trên bảng tra cứu hóa đơn, Cột 11 luôn là Trạng thái hóa đơn ("Hóa đơn mới", "Hóa đơn thay thế", v.v.)
			trangThai, _ := getByHeader("trạng thái")
			if trangThai == "" || isNumericOnly(trangThai) {
				trangThai = getVal(11)
			}
			if (isNumericOnly(trangThai) || trangThai == "") && isInvoiceStatusText(getVal(11)) {
				trangThai = getVal(11)
			}

			// Tiền phí: Bảng tra cứu chuẩn không có cột phí riêng, mặc định là "0"
			tongPhi := "0"
			if val, ok := getByHeader("tiền phí", "phí"); ok && val != "" && !strings.Contains(val, "thanh toán") && !isInvoiceStatusText(val) {
				tongPhi = val
			}

			// Kết quả kiểm tra & xử lý (Cột 12 & 13)
			kiemTra, _ := getByHeader("kết quả kiểm tra", "kiểm tra")
			if kiemTra == "" {
				kiemTra = getVal(12)
			}

			xuLy, _ := getByHeader("kết quả xử lý", "xử lý", "sai sót")
			if xuLy == "" {
				if len(cols) > 13 {
					xuLy = getVal(13)
				} else {
					xuLy = kiemTra
				}
			}

			var lienQuan string
			if len(cols) > 14 {
				lienQuan = getVal(14)
			}

			// Đơn vị tiền tệ
			currency, _ := getByHeader("tiền tệ", "đơn vị")
			if currency == "" || len(currency) > 5 {
				currency = "VND"
				foreignCurrencies := map[string]bool{
					"USD": true, "EUR": true, "JPY": true, "GBP": true,
					"AUD": true, "SGD": true, "CAD": true, "CNY": true,
					"KRW": true, "CHF": true, "HKD": true, "THB": true,
				}
				for _, c := range cols {
					val := strings.ToUpper(strings.TrimSpace(c))
					if foreignCurrencies[val] {
						currency = val
						break
					}
				}
			}

			uniqueKey := fmt.Sprintf("%s_%s_%s", mst, kyHieu, soHD)
			if uniqueKey != "__" && seen[uniqueKey] {
				continue
			}
			if uniqueKey != "__" {
				seen[uniqueKey] = true
			}

			inv := models.Invoice{
				STT:              stt,
				MSTNguoiBan:      mst,
				KyHieuMauSo:      mauSo,
				KyHieu:           kyHieu,
				SoHoaDon:         soHD,
				NgayLap:          ngayLap,
				ThongTinHoaDon:   thongTin,
				TongChuaThue:     chuaThue,
				TongTienThue:     tienThue,
				ChietKhau:        chietKhau,
				TongPhi:          tongPhi,
				TongThanhToan:    tongThanhToan,
				DonViTienTe:      currency,
				TrangThai:        trangThai,
				KetQuaXuLy:       xuLy,
				KetQuaKiemTra:    kiemTra,
				HoaDonLienQuan:   lienQuan,
				ThongTinLienQuan: "",
			}
			allInvoices = append(allInvoices, inv)
			c.TotalInvoices++
		}

		if len(rows) > 0 {
			firstRow := rows[0]
			if len(firstRow) > 4 {
				lastFirstInvoiceNo = strings.TrimSpace(firstRow[4])
			}
		}

		// --- 4. CHUYỂN TRANG ---
		// Nếu đã thu thập đủ tổng số hóa đơn báo trên trang thuế -> Dừng
		// Đã đủ số hóa đơn thuế báo → chỉ dừng khi ô "N / M" khớp (đã ở trang cuối) hoặc không đọc được ô này
		if totalOnTax > 0 && len(allInvoices) >= totalOnTax {
			fmt.Printf("🏁 Đã lấy đủ toàn bộ %d/%d hóa đơn. Dừng chuyển trang!\n", len(allInvoices), totalOnTax)
			break
		}

		// Dừng theo vị trí thật "N / M" trên trang thuế (không dùng biến đếm pageIdx vì có thể lệch khi thuế chậm)
		curPos, totPos := c.pagerPos()
		// Chưa lấy đủ số hóa đơn mà thấy "trang cuối" → đọc lại vị trí (thuế đang loading có thể hiển thị sai) trước khi kết luận
		for v := 0; v < 3 && totalOnTax > 0 && len(allInvoices) < totalOnTax && (totPos == 0 || curPos >= totPos) && ctx.Err() == nil && !c.StopRequested; v++ {
			c.waitTaxIdle(ctx, stream, 30*time.Second)
			time.Sleep(3 * time.Second)
			curPos, totPos = c.pagerPos()
		}
		if totPos > 0 {
			if curPos >= totPos {
				fmt.Printf("🏁 Đã đến trang cuối %d/%d.\n", curPos, totPos)
				break
			}
		} else if pageIdx >= maxPages {
			fmt.Printf("🏁 Đã đến trang tối đa %d/%d.\n", pageIdx, maxPages)
			break
		}

		// Ưu tiên: bấm nút > bằng CLICK CHUỘT THẬT và xác nhận ô "N / M" đã đổi trang thật.
		// Nếu thuế không chuyển trang -> dừng an toàn, không đếm trang ảo.
		if curPos, totPos := c.pagerPos(); totPos > 0 {
			nextOK := c.goNextPage(curPos, totPos)
			// Chưa lấy đủ mà không sang được trang: chờ thuế rảnh rồi thử lại (tối đa 3 vòng) trước khi bỏ cuộc
			for retry := 1; !nextOK && retry <= 10 && ctx.Err() == nil && !c.StopRequested && c.pagerCaptchaErr == nil; retry++ {
				fmt.Printf("🔁 Chưa sang được trang %d, thử lại lần %d/10 sau 10s...\n", curPos+1, retry)
				sendStream(ctx, stream, models.StreamData{Status: fmt.Sprintf("⏳ Thuế phản hồi chậm, thử chuyển sang trang %d lại (%d/10)...", curPos+1, retry)})
				time.Sleep(10 * time.Second)
				c.waitTaxIdle(ctx, stream, 60*time.Second)
				if now, _ := c.pagerPos(); now > curPos {
					nextOK = true
					break
				}
				nextOK = c.goNextPage(curPos, totPos)
			}
			if !nextOK {
				if ctx.Err() != nil || c.StopRequested {
					c.CurrentStatus = "Đã hủy"
					return allInvoices, fmt.Errorf("thao tác đã bị hủy")
				}
				if c.pagerCaptchaErr != nil {
					errCap := c.pagerCaptchaErr
					c.CurrentStatus = "Lỗi"
					if stream != nil {
						stream <- models.StreamData{Status: "❌ " + errCap.Error(), IsDone: true}
					}
					return allInvoices, errCap
				}
				fmt.Printf("⚠️ Thuế không chuyển được sang trang %d (đang kẹt ở %d/%d). Dừng để tránh trang ảo!\n", curPos+1, curPos, totPos)
				c.logError("chuyển trang", fmt.Errorf("không chuyển được sang trang %d", curPos+1), fmt.Sprintf("đã lấy %d/%d hóa đơn", len(allInvoices), totalOnTax))
				if stream != nil {
					stream <- models.StreamData{Status: fmt.Sprintf("⚠️ Chỉ lấy được %d/%d hóa đơn (thuế không chuyển được sang trang %d)", len(allInvoices), totalOnTax, curPos+1), TotalOnTax: totalOnTax, IsDone: false}
				}
				break
			}
			c.waitTaxIdle(ctx, stream, 60*time.Second)
			continue
		}

		// Fallback (không có ô "N / M"): đa chiến lược Icon Next, ant-pagination-next, Input Simple Pager, Số trang
		var nextClicked bool
		var nextMethod string
		for attempt := 1; attempt <= 40 && !nextClicked && nextMethod != "last-page"; attempt++ {
			if attempt > 1 {
				if ctx.Err() != nil || c.StopRequested {
					break
				}
				// Nút > bị khóa khi thuế đang tải -> chờ rảnh rồi bấm lại
				c.waitTaxIdle(ctx, stream, 30*time.Second)
			}
			forceNext := attempt >= 15
			_ = rod.Try(func() {
				res := c.Page.MustEval(`(targetPage, force) => {
					// 1. Tìm container thuộc tab đang active
					const activePanes = Array.from(document.querySelectorAll('.ant-tabs-tabpane-active'))
						.filter(p => !p.closest('.ant-tabs-tabpane-inactive'));
					const container = activePanes[activePanes.length - 1] || document;

					const fire = (el) => {
						if (!el) return false;
						el.scrollIntoView({ block: 'nearest' });
						el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true, view: window }));
						el.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true, view: window }));
						el.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window }));
						if (typeof el.click === 'function') el.click();
						return true;
					};

					// Cách 0 (chính xác nhất): ô "cur / total" (styles__PageIndex) và nút > nằm ngay cột kế bên
					const idxEl = Array.from(container.querySelectorAll('div')).find(d => d.children.length === 0 && /^\s*\d+\s*\/\s*\d+\s*$/.test(d.textContent || ''));
					if (idxEl) {
						const m = idxEl.textContent.match(/(\d+)\s*\/\s*(\d+)/);
						if (m && parseInt(m[1], 10) >= parseInt(m[2], 10)) return { ok: false, method: 'last-page' };
						const col = idxEl.closest('.ant-col');
						const nb = col && col.nextElementSibling ? col.nextElementSibling.querySelector('button') : null;
						if (nb && nb.querySelector('.anticon-right') && (!nb.disabled || force)) {
							if (nb.disabled || nb.hasAttribute('disabled')) { nb.removeAttribute('disabled'); nb.disabled = false; nb.style.pointerEvents = 'auto'; }
							fire(nb);
							return { ok: true, method: 'pager-next-btn' };
						}
					}

					// Cách 1: Nút chứa icon anticon-right (chuẩn Ant Design nút Next trên trang thuế)
					const rightIcons = Array.from(container.querySelectorAll('i.anticon-right, .anticon-right, [aria-label*="right"]'));
					for (const icon of rightIcons) {
						const btn = icon.closest('button, a, li');
						if (btn && !btn.disabled && !btn.hasAttribute('disabled') && !btn.classList.contains('ant-pagination-disabled') && btn.getAttribute('aria-disabled') !== 'true') {
							fire(btn);
							return { ok: true, method: 'icon-right' };
						}
					}

					// Cách 2: li.ant-pagination-next
					const nextLis = Array.from(container.querySelectorAll('.ant-pagination-next, li[title*="Next"], li[title*="trang sau"]'));
					for (const nextLi of nextLis) {
						if (nextLi.classList.contains('ant-pagination-disabled') || nextLi.getAttribute('aria-disabled') === 'true') continue;
						const toClick = nextLi.querySelector('button, a, .ant-pagination-item-link') || nextLi;
						if (!toClick.disabled && !toClick.hasAttribute('disabled')) {
							fire(toClick);
							return { ok: true, method: 'ant-pagination-next' };
						}
					}

					// Cách 3: Nhập số trang vào ô input trong simple pager (1/10) hoặc quick jumper rồi Enter
					const pageInput = container.querySelector('.ant-pagination-simple-pager input, .ant-pagination-options-quick-jumper input, .ant-pagination input');
					if (pageInput) {
						pageInput.focus();
						pageInput.value = targetPage;
						pageInput.dispatchEvent(new Event('input', { bubbles: true }));
						pageInput.dispatchEvent(new Event('change', { bubbles: true }));
						pageInput.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', keyCode: 13, which: 13, bubbles: true }));
						pageInput.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', keyCode: 13, which: 13, bubbles: true }));
						return { ok: true, method: 'simple-pager-input' };
					}

					// Cách 4: Click vào số trang đích (nếu có thanh số 1, 2, 3...)
					const numItem = container.querySelector('.ant-pagination-item-' + targetPage + ', li[title="' + targetPage + '"]');
					if (numItem && !numItem.classList.contains('ant-pagination-item-active')) {
						fire(numItem.querySelector('a') || numItem);
						return { ok: true, method: 'numbered-item' };
					}

					// Cách 5: Tìm kiếm trên toàn document (fallback ngoài container)
					const docRight = document.querySelector('button i.anticon-right, li.ant-pagination-next:not(.ant-pagination-disabled) a, li.ant-pagination-next:not(.ant-pagination-disabled) button');
					if (docRight) {
						const b = docRight.closest('button, a, li');
						if (b && !b.disabled && !b.classList.contains('ant-pagination-disabled')) {
							fire(b);
							return { ok: true, method: 'doc-fallback' };
						}
					}

					return { ok: false, method: 'none' };
				}`, pageIdx+1, forceNext)
				nextClicked = res.Get("ok").Bool()
				nextMethod = res.Get("method").Str()
			})
			if !nextClicked && nextMethod != "last-page" && attempt < 40 {
				time.Sleep(3000 * time.Millisecond)
			}
		}
		if !nextClicked {
			fmt.Printf("🏁 Không còn trang tiếp theo hoặc nút Next bị vô hiệu hóa. Dừng tại trang %d!\n", pageIdx)
			if totalOnTax > 0 && len(allInvoices) < totalOnTax {
				c.logError("chuyển trang", fmt.Errorf("dừng sớm ở trang %d", pageIdx), fmt.Sprintf("đã lấy %d/%d hóa đơn, maxPages=%d", len(allInvoices), totalOnTax, maxPages))
				if stream != nil {
					stream <- models.StreamData{Status: fmt.Sprintf("⚠️ Chỉ lấy được %d/%d hóa đơn (không chuyển được sang trang %d)", len(allInvoices), totalOnTax, pageIdx+1), IsDone: false}
				}
			}
			break
		}
		fmt.Printf("⏭️ Đã bấm sang trang %d (cách: %s)...\n", pageIdx+1, nextMethod)
		time.Sleep(1500 * time.Millisecond)
		// Xác nhận ô "N / M" đã đổi sang trang đích (tối đa ~30s), rồi chờ hết loading
		for w := 0; w < 30; w++ {
			cur := 0
			_ = rod.Try(func() {
				r := c.Page.Timeout(3 * time.Second).MustEval(`() => {
					const panes = Array.from(document.querySelectorAll('.ant-tabs-tabpane-active'));
					const root = panes[panes.length - 1] || document;
					const el = Array.from(root.querySelectorAll('div')).find(d => d.children.length === 0 && /^\s*\d+\s*\/\s*\d+\s*$/.test(d.textContent || ''));
					if (!el) return -1;
					return parseInt(el.textContent.match(/(\d+)/)[1], 10);
				}`)
				cur = r.Int()
			})
			if cur == -1 || cur == pageIdx+1 {
				break
			}
			time.Sleep(1000 * time.Millisecond)
		}
		c.waitTaxIdle(ctx, stream, 60*time.Second)
	}

	// 5. KẾT THÚC
	finalStatus := "✅ Hoàn thành tra cứu!"
	if totalOnTax > 0 && len(allInvoices) < totalOnTax {
		finalStatus = fmt.Sprintf("⚠️ Chưa lấy hết: mới có %d/%d hóa đơn. Vui lòng tìm kiếm lại để lấy đủ", len(allInvoices), totalOnTax)
		fmt.Println(finalStatus)
	}
	if len(allInvoices) > 0 {
		c.lastDataAt = time.Now()
	}
	if stream != nil {
		stream <- models.StreamData{
			Invoices:   allInvoices,
			Status:     finalStatus,
			TotalSoFar: c.TotalInvoices,
			TotalOnTax: totalOnTax,
			IsDone:     true,
		}
	}

	return allInvoices, nil
}

// isTaxLoading: trả về true nếu trang thuế đang hiển thị trạng thái đang tải
// (spinner Ant Design / bảng đang loading / nút Tìm kiếm đang loading) ở tab đang hoạt động.
func (c *CrawlerService) isTaxLoading() bool {
	loading := false
	_ = rod.Try(func() {
		res := c.Page.Timeout(3 * time.Second).MustEval(`() => {
			const shown = (el) => {
				if (!el || el.closest('.ant-tabs-tabpane-inactive')) return false;
				return el.offsetParent !== null || el.getClientRects().length > 0;
			};
			const sel = '.ant-spin-spinning, .ant-spin-blur, .ant-table-loading, .ant-btn-loading';
			return Array.from(document.querySelectorAll(sel)).some(shown);
		}`)
		loading = res.Bool()
	})
	return loading
}

// waitTaxIdle: chờ đến khi trang thuế hết trạng thái đang tải (2 lần kiểm tra liên tiếp đều rảnh).
// Trả về true nếu bị hủy giữa chừng; hết maxWait thì tự thoát để các bước sau xử lý như cũ.
func (c *CrawlerService) waitTaxIdle(ctx context.Context, stream chan models.StreamData, maxWait time.Duration) bool {
	start := time.Now()
	deadline := start.Add(maxWait)
	lastNotify := start
	idleCount := 0
	for time.Now().Before(deadline) {
		if ctx.Err() != nil || c.StopRequested {
			return true
		}
		if c.isTaxLoading() {
			idleCount = 0
			// Báo tiến trình định kỳ để giao diện biết vẫn đang chờ thuế (tránh watchdog 90s)
			if stream != nil && time.Since(lastNotify) >= 10*time.Second {
				stream <- models.StreamData{Status: fmt.Sprintf("⏳ Hệ thống thuế đang tải dữ liệu... (%d giây)", int(time.Since(start).Seconds())), IsDone: false}
				lastNotify = time.Now()
			}
		} else {
			idleCount++
			if idleCount >= 2 {
				return false
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

// Hàm bổ trợ lấy Số hóa đơn đầu tiên trên bảng hiện tại
func (c *CrawlerService) getFirstInvoiceNoFromUI() string {
	res, err := c.Page.Eval(`() => {
        const rows = Array.from(document.querySelectorAll(".ant-table-tbody tr.ant-table-row")).filter(r => !r.closest('.ant-tabs-tabpane-inactive'));
        if (rows.length === 0) return "";
        const cell = rows[0].querySelector("td:nth-child(5)");
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

	fmt.Printf("📅  %s cho ô %s...\n", dateStr, selector)

	// Dọn dẹp mọi style ẩn bị lưu sai trước đây (nếu có)
	_ = rod.Try(func() {
		c.Page.MustEval(`() => {
			document.querySelectorAll('.ant-select-dropdown').forEach(d => {
				if (d.style.display === 'none') d.style.removeProperty('display');
			});
		}`)
	})

	// 1. Click ô date picker để mở popup calendar (tìm ô VISIBLE trong active tab)
	var clickErr error
	for retry := 0; retry < 3; retry++ {
		clickErr = rod.Try(func() {
			els, err := c.Page.Timeout(3 * time.Second).Elements(selector)
			if err == nil && len(els) > 0 {
				for _, el := range els {
					if vis, errV := el.Visible(); errV == nil && vis {
						el.MustClick()
						return
					}
				}
			}
			el := c.Page.Timeout(3 * time.Second).MustElement(selector)
			el.MustClick()
		})
		if clickErr == nil {
			break
		}
		fmt.Printf("⚠️ Retry click ô lịch lần %d: %v\n", retry+1, clickErr)
		time.Sleep(400 * time.Millisecond)
	}
	if clickErr != nil {
		calErr := fmt.Errorf("không thể mở ô lịch %s: %v", selector, clickErr)
		c.logError("selectDateOnCalendar", calErr, fmt.Sprintf("Selector: %s | Ngày: %s", selector, dateStr))
		return calErr
	}
	time.Sleep(400 * time.Millisecond)

	// 2. Tìm ô input bên trong popup calendar và gõ ngày (dùng Timeout để KHÔNG BAO GIỜ BỊ ĐỨNG)
	var inputErr error
	for retry := 0; retry < 2; retry++ {
		inputErr = rod.Try(func() {
			calInput := c.Page.Timeout(3 * time.Second).MustElement(".ant-calendar-input")
			calInput.MustWaitVisible()
			calInput.MustSelectAllText().MustInput(dateStr)
			fmt.Printf("✅ Đã nhập ngày %s vào ô calendar\n", dateStr)
		})
		if inputErr == nil {
			break
		}
		// Nếu chưa thấy ô .ant-calendar-input, thử click lại ô lịch
		fmt.Printf("⚠️ Chưa thấy popup lịch, click lại ô %s...\n", selector)
		_ = rod.Try(func() {
			c.Page.Timeout(2 * time.Second).MustElement(selector).MustClick()
		})
		time.Sleep(400 * time.Millisecond)
	}
	if inputErr != nil {
		return fmt.Errorf("không thể nhập ngày vào calendar: %v", inputErr)
	}
	time.Sleep(400 * time.Millisecond)

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

	time.Sleep(400 * time.Millisecond)
	return nil
}

// getDownloadTempDir trả về thư mục tạm dùng cho việc tải file ZIP từ Chrome.
// Nếu người dùng đã chọn OutputDir, thư mục tạm sẽ là thư mục con (.temp_download) trong OutputDir,
// ngược lại sẽ dùng thư mục tạm của hệ thống (os.TempDir), hoàn toàn không tạo thư mục lạ trong Downloads.
func (c *CrawlerService) getDownloadTempDir() string {
	if c.OutputDir != "" {
		return filepath.Join(c.OutputDir, ".temp_download")
	}
	return filepath.Join(os.TempDir(), "TaxSolution_Temp")
}

// SetupBrowserDownload cấu hình browser download vào thư mục tạm hợp lệ khi cần.
func (c *CrawlerService) SetupBrowserDownload() {
	downloadDir := c.getDownloadTempDir()
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
	// Chỉ tạo thư mục timestamp khi OutputDir chưa được set từ API
	// (tránh ghi đè đường dẫn mà người dùng đã chọn)
	if c.OutputDir == "" {
		homeDir, _ := os.UserHomeDir()
		timeStr := time.Now().Format("20060102_150405")
		c.OutputDir = filepath.Join(homeDir, "Downloads", "Invoices_Output_"+timeStr)
		fmt.Printf("⚠️  OutputDir chưa được set, dùng mặc định: %s\n", c.OutputDir)
	} else {
		fmt.Printf("✅ Dùng OutputDir đã chọn: %s\n", c.OutputDir)
	}

	downloadDir := c.getDownloadTempDir()
	os.MkdirAll(downloadDir, os.ModePerm)
	os.MkdirAll(c.OutputDir, os.ModePerm)

	proto.BrowserSetDownloadBehavior{
		Behavior:      proto.BrowserSetDownloadBehaviorBehaviorAllow,
		DownloadPath:  downloadDir,
		EventsEnabled: true,
	}.Call(c.Page)

	fmt.Printf("🚀 Khởi tạo thành công!\n📁 Folder lưu chính: %s\n📁 Folder tạm: %s\n", c.OutputDir, downloadDir)
}

func (c *CrawlerService) CrawlAllPages() {
	// 1. Khởi tạo trạng thái bắt đầu
	c.IsRunning = true
	c.TotalInvoices = 0
	c.StopRequested = false
	c.CurrentStatus = "Bắt đầu khởi động ứng dụng..."
	c.GrantPermissions()

	// MTT: cổng thuế hỏi captcha mỗi lần đổi trang (kể cả quay về trang 1) → tự giải bằng AI (luồng này không có popup nhập tay)
	c.pagerCaptchaErr = nil
	c.pagerCaptchaOn = c.searchCategory == "may_tinh_tien" && captcha.Enabled()
	c.pagerCaptchaCtx = context.Background()
	c.pagerCaptchaStream = nil
	c.captchaNoManual = c.pagerCaptchaOn
	defer func() { c.pagerCaptchaOn = false; c.captchaNoManual = false }()

	// Đảm bảo khi hàm kết thúc (xong hoặc lỗi) thì IsRunning phải về false và xóa sạch thư mục tạm
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("❌ Panic trong CrawlAllPages: %v\n", r)
			c.CurrentStatus = "Lỗi"
		}
		c.IsRunning = false
		if c.CurrentStatus != "Lỗi" && c.CurrentStatus != "Đã hủy" {
			c.CurrentStatus = "Hoàn thành"
		}
		// Dọn dẹp sạch sẽ thư mục tạm sau khi hoàn tất tải toàn bộ
		tempDir := c.getDownloadTempDir()
		_ = os.RemoveAll(tempDir)
	}()

	fmt.Println("🚀 Bắt đầu quy trình cào đa trang...")
	// BẮT BUỘC quay về trang 1 trước khi tải chi tiết (dù người dùng đang đứng ở trang nào)
	c.CurrentStatus = "Đang quay về trang đầu tiên..."
	fmt.Println("⏮️ Đang reset về trang 1 bằng nút mũi tên trái...")

	// MTT: mỗi lần bấm lùi đều dính captcha → bấm Tìm kiếm lại 1 lần (chỉ 1 captcha, tự về trang 1) cho nhanh
	if c.pagerCaptchaOn {
		if cur, total := c.pagerPos(); total > 0 && cur > 1 {
			fmt.Printf("🔍 Đang ở trang %d/%d → bấm Tìm kiếm lại để về trang 1 (nhanh hơn bấm lùi từng trang).\n", cur, total)
			var lastErr error
			for try := 1; try <= 8 && !c.StopRequested; try++ {
				if !c.clickSearchButton() {
					break
				}
				lastErr = c.handleSearchCaptchaWait(c.pagerCaptchaCtx, nil, searchCaptchaAppearWait)
				if lastErr == nil {
					time.Sleep(1500 * time.Millisecond)
					c.waitNotLoading(60 * time.Second)
					break
				}
				// Sai captcha → bấm Tìm kiếm lại cho tới khi qua được
				fmt.Printf("🔁 Captcha chưa qua (%v), bấm Tìm kiếm lại (%d/8)...\n", lastErr, try)
				time.Sleep(2 * time.Second)
			}
			if lastErr != nil && !c.StopRequested {
				if now, _ := c.pagerPos(); now > 1 {
					fmt.Println("⚠️ Tìm kiếm lại chưa về được trang 1, chuyển sang bấm lùi từng trang.")
				}
			}
		}
	}

	backCapFails := 0
	for guard := 0; guard < 500; guard++ {
		if c.StopRequested {
			c.CurrentStatus = "Đã hủy"
			fmt.Println("🛑 Tiến trình cào đã bị người dùng hủy.")
			return
		}
		cur, total := c.pagerPos()
		fmt.Printf("📍 Vị trí phân trang hiện tại: %d/%d\n", cur, total)
		if total == 0 && guard < 3 {
			// Chưa đọc được thanh phân trang -> chờ trang ổn định rồi đọc lại trước khi kết luận chỉ có 1 trang
			c.waitNotLoading(10 * time.Second)
			time.Sleep(1 * time.Second)
			continue
		}
		if total == 0 || cur <= 1 {
			fmt.Printf("✅ Đã ở trang 1 (%d/%d) hoặc chỉ có 1 trang.\n", cur, total)
			break
		}
		c.CurrentStatus = "Đang chuẩn bị dữ liệu..."
		if !c.clickPager("left") {
			// Nút < đang tạm khóa do trang thuế còn loading -> chờ rồi thử lại
			time.Sleep(1 * time.Second)
			if c2, _ := c.pagerPos(); c2 == cur {
				c.waitNotLoading(30 * time.Second)
				if !c.clickPager("left") {
					fmt.Printf("⚠️ Không bấm được nút lùi ở trang %d.\n", cur)
					break
				}
			}
		}
		if c.pagerCaptchaOn {
			if errCap := c.handleSearchCaptchaWait(c.pagerCaptchaCtx, nil, pagerCaptchaAppearWait); errCap != nil {
				backCapFails++
				if backCapFails >= 10 || c.StopRequested {
					c.CurrentStatus = "Lỗi"
					c.logError("captcha quay về trang 1", errCap, fmt.Sprintf("đang ở trang %d/%d", cur, total))
					return
				}
				// Sai captcha → bấm lùi lại để cổng thuế hỏi lại
				fmt.Printf("🔁 Captcha chưa qua (%v), bấm lùi lại (%d/10)...\n", errCap, backCapFails)
				time.Sleep(2 * time.Second)
				continue
			}
		}
		c.waitPagerPage(cur-1, 30*time.Second)
		c.waitNotLoading(60 * time.Second)
	}
	if cur, total := c.pagerPos(); total > 0 && cur != 1 {
		c.CurrentStatus = "Lỗi"
		c.logError("quay về trang 1", fmt.Errorf("không về được trang 1"), fmt.Sprintf("đang ở trang %d/%d", cur, total))
		fmt.Printf("❌ Không quay về được trang 1 (đang ở %d/%d). Hủy tải để tránh thiếu dữ liệu!\n", cur, total)
		return
	}
	c.waitNotLoading(60 * time.Second)
	time.Sleep(1 * time.Second)

	for page := 1; ; page++ {
		if c.StopRequested {
			c.CurrentStatus = "Đã hủy"
			fmt.Println("🛑 Tiến trình cào đã bị người dùng hủy.")
			break
		}
		// Cập nhật trạng thái trang hiện tại cho FE
		c.CurrentStatus = fmt.Sprintf("Đang xử lý trang %d...", page)
		fmt.Printf("\n📄 --- %s ---\n", c.CurrentStatus)

		// Đợi bảng hiện ra trong tab đang active
		_ = rod.Try(func() {
			c.Page.Timeout(10 * time.Second).MustElement(".ant-tabs-tabpane-active .ant-table-tbody").MustWaitVisible()
		})
		time.Sleep(1 * time.Second)

		// Chỉ lấy các dòng thực sự có dữ liệu (tr.ant-table-row) trong tab đang active
		var rows rod.Elements
		for retry := 0; retry < 5; retry++ {
			markerId := "crawler-row-" + strconv.FormatInt(time.Now().UnixNano(), 10)
			res := c.Page.MustEval(fmt.Sprintf(`() => {
				const activePanes = Array.from(document.querySelectorAll('.ant-tabs-tabpane-active'))
					.filter(p => !p.closest('.ant-tabs-tabpane-inactive'));
				const container = activePanes[activePanes.length - 1] || document;
				const activeRows = Array.from(container.querySelectorAll(".ant-table-tbody tr.ant-table-row"));
				activeRows.forEach((r, idx) => {
					r.setAttribute('data-crawl-row', '%s' + '-' + idx);
				});
				return activeRows.length;
			}`, markerId))

			if res.Int() > 0 {
				if found, errF := c.Page.Elements(fmt.Sprintf("[data-crawl-row^='%s-']", markerId)); errF == nil && len(found) > 0 {
					rows = found
					break
				}
			}
			time.Sleep(500 * time.Millisecond)
		}

		fmt.Printf("📂 Tìm thấy %d dòng hóa đơn tại trang %d.\n", len(rows), page)
		if len(rows) == 0 {
			fmt.Println("🏁 Không có dòng hóa đơn nào trên trang này. Dừng lại!")
			break
		}

		downloadRow := func(i int, row *rod.Element) error {
			// Hệ thống thuế hay chậm/lỗi: thử lại tối đa 3 lần, mỗi lần chờ trang thuế rảnh rồi mới tải
			var err error
			for attempt := 1; attempt <= 3; attempt++ {
				if c.StopRequested {
					return fmt.Errorf("tiến trình đã bị người dùng dừng")
				}
				c.waitNotLoading(45 * time.Second)
				err = c.ProcessDownloadInvoice(row, i)
				if err == nil {
					return nil
				}
				fmt.Printf("   ⚠️ Dòng %d lỗi (lần %d/3): %v\n", i+1, attempt, err)
				if attempt < 3 {
					c.CurrentStatus = fmt.Sprintf("Hệ thống thuế phản hồi chậm, đang thử lại (%d/3)...", attempt+1)
					time.Sleep(3 * time.Second)
				}
			}
			return err
		}

		var failedIdx []int
		for i, row := range rows {
			if c.StopRequested {
				c.CurrentStatus = "Đã hủy"
				fmt.Println("🛑 Tiến trình cào đã bị người dùng hủy.")
				break
			}
			// Cập nhật trạng thái từng dòng để FE hiển thị tiến độ (ProgressBar)
			c.CurrentStatus = fmt.Sprintf("Trang %d: Đang tải dòng %d/%d", page, i+1, len(rows))
			fmt.Printf("   🔍 %s...\n", c.CurrentStatus)

			if err := downloadRow(i, row); err != nil {
				fmt.Printf("   ❌ Lỗi tại dòng %d sau 3 lần thử: %v → sẽ thử lại sau khi hết trang\n", i+1, err)
				c.logError("tải chi tiết hóa đơn", err, fmt.Sprintf("trang %d, dòng %d/%d", page, i+1, len(rows)))
				failedIdx = append(failedIdx, i)
				continue
			}

			// Tải xong 1 dòng thì tăng biến đếm tổng
			c.TotalInvoices++
		}

		// Các dòng lỗi KHÔNG bỏ qua: chờ thuế hồi phục rồi tải lại (tối đa 4 vòng) trước khi sang trang
		for round := 1; round <= 4 && len(failedIdx) > 0 && !c.StopRequested; round++ {
			fmt.Printf("🔁 Trang %d còn %d dòng chưa tải được, thử lại vòng %d/4 sau %ds...\n", page, len(failedIdx), round, round*10)
			c.CurrentStatus = fmt.Sprintf("Trang %d: thử lại %d dòng lỗi (vòng %d/4)...", page, len(failedIdx), round)
			time.Sleep(time.Duration(round*10) * time.Second)
			c.waitNotLoading(60 * time.Second)

			var stillFailed []int
			for _, i := range failedIdx {
				if c.StopRequested {
					break
				}
				if err := downloadRow(i, rows[i]); err != nil {
					stillFailed = append(stillFailed, i)
					continue
				}
				fmt.Printf("   ✅ Dòng %d đã tải được ở vòng thử lại %d.\n", i+1, round)
				c.TotalInvoices++
			}
			failedIdx = stillFailed
		}
		if len(failedIdx) > 0 && !c.StopRequested {
			nums := make([]string, 0, len(failedIdx))
			for _, i := range failedIdx {
				nums = append(nums, strconv.Itoa(i+1))
			}
			fmt.Printf("❌ Trang %d vẫn còn %d dòng không tải được: %s\n", page, len(failedIdx), strings.Join(nums, ", "))
			c.logError("tải chi tiết hóa đơn", fmt.Errorf("thiếu %d dòng", len(failedIdx)), fmt.Sprintf("trang %d, các dòng: %s", page, strings.Join(nums, ", ")))
		}

		// Chuyển sang trang kế tiếp bằng nút > cạnh ô "N / M"
		cur, total := c.pagerPos()
		if total > 0 && cur >= total {
			fmt.Printf("🏁 Đã cào xong trang cuối cùng (Trang %d/%d). Dừng lại!\n", cur, total)
			break
		}
		if !c.goNextPage(cur, total) {
			if c.StopRequested {
				break
			}
			if c.pagerCaptchaErr != nil {
				c.CurrentStatus = "Lỗi"
				c.logError("captcha chuyển trang tải chi tiết", c.pagerCaptchaErr, fmt.Sprintf("đang ở trang %d/%d", cur, total))
			}
			fmt.Printf("⚠️ Không chuyển được sang trang %d (nút > của thuế không phản hồi). Dừng để tránh tải trùng!\n", cur+1)
			c.logError("chuyển trang tải chi tiết", fmt.Errorf("không chuyển được sang trang %d", cur+1), fmt.Sprintf("đang ở trang %d/%d, nút > bị khóa", cur, total))
			break
		}
		fmt.Println("⏭️ Đã chuyển sang trang tiếp theo...")
		c.waitNotLoading(60 * time.Second)
		time.Sleep(1 * time.Second)
	}
}

// pagerPos đọc ô "cur / total" của thanh phân trang trên tab đang active. Không có thanh phân trang -> (0,0).
func (c *CrawlerService) pagerPos() (cur, total int) {
	err := rod.Try(func() {
		res := c.Page.Timeout(5 * time.Second).MustEval(`() => {
			const shown = (el) => !el.closest('.ant-tabs-tabpane-inactive') && (el.offsetParent !== null || el.getClientRects().length > 0);
			const el = Array.from(document.querySelectorAll('div')).find(d => d.children.length === 0 && /^\s*\d+\s*\/\s*\d+\s*$/.test(d.textContent || '') && shown(d));
			if (!el) return '0/0';
			const m = el.textContent.match(/(\d+)\s*\/\s*(\d+)/);
			return m[1] + '/' + m[2];
		}`)
		fmt.Sscanf(res.Str(), "%d/%d", &cur, &total)
	})
	if err != nil {
		fmt.Printf("⚠️ pagerPos lỗi: %v\n", err)
	}
	return
}

// clickPager bấm nút < (dir="left") hoặc > (dir="right") nằm cạnh ô "N / M". Trả về true nếu đã bấm.
func (c *CrawlerService) clickPager(dir string) bool {
	return c.clickPagerOpt(dir, false)
}

// clickPagerOpt như clickPager; force=true sẽ gỡ thuộc tính disabled (nút bị thuế khóa kẹt) trước khi bấm.
func (c *CrawlerService) clickPagerOpt(dir string, force bool) bool {
	// Bước 1: tìm nút, gắn marker; trả 0=không thấy, 1=đang disabled, 2=sẵn sàng
	state := 0
	_ = rod.Try(func() {
		res := c.Page.Timeout(5*time.Second).MustEval(`(dir) => {
			document.querySelectorAll('[data-pager-target]').forEach(e => e.removeAttribute('data-pager-target'));
			const shown = (el) => !el.closest('.ant-tabs-tabpane-inactive') && (el.offsetParent !== null || el.getClientRects().length > 0);
			let btn = null;
			const el = Array.from(document.querySelectorAll('div')).find(d => d.children.length === 0 && /^\s*\d+\s*\/\s*\d+\s*$/.test(d.textContent || '') && shown(d));
			if (el) {
				const col = el.closest('.ant-col');
				const sib = col ? (dir === 'left' ? col.previousElementSibling : col.nextElementSibling) : null;
				btn = sib ? sib.querySelector('button') : null;
				if (btn && !btn.querySelector('.anticon-' + dir)) btn = null;
			}
			if (!btn) {
				const ic = Array.from(document.querySelectorAll('button i.anticon-' + dir)).find(shown);
				btn = ic ? ic.closest('button') : null;
			}
			if (!btn) return 0;
			btn.setAttribute('data-pager-target', dir);
			return (btn.disabled || btn.hasAttribute('disabled')) ? 1 : 2;
		}`, dir)
		state = res.Int()
	})
	if state == 0 {
		return false
	}
	if state == 2 {
		// Click chuột thật qua CDP (isTrusted=true) để React nhận sự kiện
		nativeOK := false
		_ = rod.Try(func() {
			el := c.Page.Timeout(5 * time.Second).MustElement("[data-pager-target='" + dir + "']")
			_ = el.ScrollIntoView()
			if err := el.Timeout(5*time.Second).Click("left", 1); err == nil {
				nativeOK = true
			}
		})
		if nativeOK {
			return true
		}
	} else if !force {
		return false
	}
	return c.clickPagerJS(dir, force)
}

// clickPagerJS: phương án dự phòng, click giả lập bằng JS (có thể bị React bỏ qua).
func (c *CrawlerService) clickPagerJS(dir string, force bool) bool {
	clicked := false
	err := rod.Try(func() {
		res := c.Page.Timeout(5*time.Second).MustEval(`(dir, force) => {
			const shown = (el) => !el.closest('.ant-tabs-tabpane-inactive') && (el.offsetParent !== null || el.getClientRects().length > 0);
			let btn = null;
			const el = Array.from(document.querySelectorAll('div')).find(d => d.children.length === 0 && /^\s*\d+\s*\/\s*\d+\s*$/.test(d.textContent || '') && shown(d));
			if (el) {
				const col = el.closest('.ant-col');
				const sib = col ? (dir === 'left' ? col.previousElementSibling : col.nextElementSibling) : null;
				btn = sib ? sib.querySelector('button') : null;
				if (btn && !btn.querySelector('.anticon-' + dir)) btn = null;
			}
			if (!btn) {
				const ic = Array.from(document.querySelectorAll('button i.anticon-' + dir)).find(shown);
				btn = ic ? ic.closest('button') : null;
			}
			if (!btn) return false;
			if (btn.disabled || btn.hasAttribute('disabled')) {
				if (!force) return false;
				btn.removeAttribute('disabled');
				btn.disabled = false;
				btn.style.pointerEvents = 'auto';
			}
			btn.scrollIntoView({ block: 'nearest' });
			btn.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true, view: window }));
			btn.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true, view: window }));
			btn.click();
			return true;
		}`, dir, force)
		clicked = res.Bool()
	})
	if err != nil {
		fmt.Printf("⚠️ clickPager(%s) lỗi: %v\n", dir, err)
	}
	return clicked
}

// unstickPager đóng các popup/overlay có thể đang giữ trang thuế ở trạng thái khóa nút chuyển trang.
func (c *CrawlerService) unstickPager() {
	_ = rod.Try(func() {
		c.Page.Timeout(5 * time.Second).MustEval(`() => {
			document.querySelectorAll('.ant-modal-close, .ant-message-notice, .ant-notification-notice-close').forEach(b => { try { b.click(); } catch (e) {} });
			document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', keyCode: 27, bubbles: true }));
			document.body.click();
		}`)
	})
}

// goNextPage chuyển sang trang cur+1. Nút > của thuế hay bị khóa tạm thời khi hệ thống chậm nên sẽ:
// chờ trang rảnh → bấm thử → sau 20s dọn overlay → sau 40s ép gỡ khóa nút. Tối đa 150s.
func (c *CrawlerService) goNextPage(cur, total int) bool {
	start := time.Now()
	deadline := start.Add(150 * time.Second)
	for attempt := 0; time.Now().Before(deadline); attempt++ {
		if c.StopRequested {
			return false
		}
		// Đã sang trang mới (ví dụ lần bấm trước phản hồi muộn) -> không bấm thêm để tránh nhảy cóc trang
		if now, _ := c.pagerPos(); now >= cur+1 {
			return true
		}
		c.waitNotLoading(30 * time.Second)
		elapsed := time.Since(start)
		if elapsed > 20*time.Second && attempt%3 == 0 {
			c.unstickPager()
		}
		force := elapsed > 40*time.Second
		if c.clickPagerOpt("right", force) {
			if force {
				fmt.Println("⚠️ Nút > của thuế bị khóa, đã ép gỡ khóa và bấm.")
			}
			// MTT: cổng thuế hỏi lại captcha mỗi lần chuyển trang → giải (AI / người dùng) rồi mới chờ đổi trang
			if c.pagerCaptchaOn {
				if errCap := c.handleSearchCaptchaWait(c.pagerCaptchaCtx, c.pagerCaptchaStream, pagerCaptchaAppearWait); errCap != nil {
					if c.captchaNoManual && !c.StopRequested {
						// Tải chi tiết: sai captcha / popup đóng → bấm lại nút > để cổng thuế hỏi lại, bấm cho tới khi qua được
						fmt.Printf("🔁 Captcha chưa qua (%v), bấm chuyển trang lại...\n", errCap)
						time.Sleep(2 * time.Second)
						continue
					}
					c.pagerCaptchaErr = errCap
					return false
				}
			}
			if c.waitPagerPage(cur+1, 25*time.Second) {
				return true
			}
			continue
		}
		c.CurrentStatus = "Hệ thống thuế phản hồi chậm, đang chờ chuyển trang..."
		time.Sleep(2 * time.Second)
	}
	return false
}

// waitPagerPage chờ ô "N / M" hiển thị đúng trang target (tối đa maxWait).
func (c *CrawlerService) waitPagerPage(target int, maxWait time.Duration) bool {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if c.StopRequested {
			return false
		}
		if cur, _ := c.pagerPos(); cur == target {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// waitNotLoading chờ trang thuế hết trạng thái loading (2 lần kiểm tra liên tiếp).
func (c *CrawlerService) waitNotLoading(maxWait time.Duration) {
	deadline := time.Now().Add(maxWait)
	idle := 0
	for time.Now().Before(deadline) && !c.StopRequested {
		if c.isTaxLoading() {
			idle = 0
		} else {
			idle++
			if idle >= 2 {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (c *CrawlerService) ProcessDownloadInvoice(row *rod.Element, index int) error {
	if c.StopRequested {
		return fmt.Errorf("tiến trình đã bị người dùng dừng")
	}
	fmt.Printf("\n🔍 --- Đang xử lý dòng %d ---\n", index+1)

	if row == nil {
		return fmt.Errorf("row is nil")
	}

	// --- BƯỚC 1: CHỌN DÒNG HÓA ĐƠN (Dùng JS để vượt qua CSS pointer-events: none) ---
	_ = rod.Try(func() {
		row.MustEval(`() => {
			this.scrollIntoView({ block: 'center', inline: 'center' });
			// Kích hoạt click trên các ô td và chính dòng tr
			const cells = Array.from(this.querySelectorAll('td'));
			const target = cells.length > 1 ? cells[1] : this;
			
			target.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true, view: window }));
			target.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true, view: window }));
			target.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window }));
			target.click();

			this.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true, view: window }));
			this.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true, view: window }));
			this.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window }));
			this.click();
		}`)
	})
	time.Sleep(800 * time.Millisecond)

	// --- BƯỚC 2: TÌM VÀ CLICK NÚT TẢI (XML / ZIP) TRÊN TOOLBAR ---
	var btnDownload *rod.Element
	_ = rod.Try(func() {
		markerId := "crawler-dl-" + strconv.FormatInt(time.Now().UnixNano(), 10)
		res := c.Page.MustEval(fmt.Sprintf(`() => {
			const activePanes = Array.from(document.querySelectorAll('.ant-tabs-tabpane-active'))
				.filter(p => !p.closest('.ant-tabs-tabpane-inactive'));
			const container = activePanes[activePanes.length - 1] || document;

			const allBtns = Array.from(container.querySelectorAll('button'));
			// Tìm theo title / aria-label / text chứa Tải hoặc icon
			let target = allBtns.find(b => {
				const t = ((b.innerText || '') + ' ' + (b.getAttribute('title') || '') + ' ' + (b.getAttribute('aria-label') || '')).toLowerCase();
				return t.includes('tải') || t.includes('download') || t.includes('xml');
			});

			if (!target) {
				const toolBtns = allBtns.filter(b => b.classList.contains('fRqwwC') || b.querySelector('i[class*="download"], i[class*="file"], i[class*="export"]'));
				if (toolBtns.length >= 4) {
					target = toolBtns[3];
				} else if (toolBtns.length > 0) {
					target = toolBtns[toolBtns.length - 1];
				}
			}

			if (!target) {
				const docFBtns = Array.from(document.querySelectorAll('button.fRqwwC'));
				if (docFBtns.length >= 4) {
					target = docFBtns[3];
				}
			}

			if (target) {
				target.setAttribute('data-dl-target', '%s');
				return true;
			}
			return false;
		}`, markerId))

		if res.Bool() {
			if el, errEl := c.Page.Element(fmt.Sprintf("[data-dl-target='%s']", markerId)); errEl == nil && el != nil {
				btnDownload = el
			}
		}
	})

	if btnDownload == nil {
		btns := c.Page.MustElements("button.fRqwwC")
		if len(btns) >= 4 {
			btnDownload = btns[3]
		} else {
			btnDownload, _ = c.Page.ElementX("(//button[@type='button'])[14]")
		}
	}

	if btnDownload == nil {
		return fmt.Errorf("không tìm thấy nút tải file")
	}

	// Xóa zip cũ còn sót để không nhặt nhầm file của lần thử trước
	if staleFiles, errStale := os.ReadDir(c.getDownloadTempDir()); errStale == nil {
		for _, sf := range staleFiles {
			if !sf.IsDir() && strings.HasSuffix(sf.Name(), ".zip") {
				_ = os.Remove(filepath.Join(c.getDownloadTempDir(), sf.Name()))
			}
		}
	}

	// Click nút tải bằng JS event để tránh lỗi pointer-events hoặc overlay
	_ = rod.Try(func() {
		btnDownload.MustEval(`() => {
			this.scrollIntoView({ block: 'center', inline: 'center' });
			this.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true, view: window }));
			this.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true, view: window }));
			this.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window }));
			this.click();
		}`)
	})
	fmt.Println("   📥 Đang tải file...")

	// --- BƯỚC 3: ĐẶT TÊN ---
	tds, errTds := row.Elements("td")
	if errTds != nil || len(tds) == 0 {
		return fmt.Errorf("không thể đọc dữ liệu dòng %d", index+1)
	}

	getText := func(idx int) string {
		if idx >= 0 && idx < len(tds) {
			return strings.TrimSpace(tds[idx].MustText())
		}
		return ""
	}

	// Cột 2: Ký hiệu mẫu số, Cột 3: Ký hiệu HĐ, Cột 4: Số HĐ, Cột 5: Ngày lập
	soKHMS := getText(2)
	kyHieuHD := getText(3)
	soHD := getText(4)
	ngayLap := strings.ReplaceAll(getText(5), "/", "")

	if soKHMS == "" {
		soKHMS = getText(1)
	}
	if kyHieuHD == "" {
		kyHieuHD = getText(2)
	}
	if soHD == "" {
		soHD = getText(3)
	}
	if ngayLap == "" {
		ngayLap = strings.ReplaceAll(getText(4), "/", "")
	}

	// Đánh số thứ tự 01, 02...
	fileName := fmt.Sprintf("%02d_%s_%s_%s_%s", index+1, soKHMS, kyHieuHD, soHD, ngayLap)

	// --- BƯỚC 4: GIẢI NÉN VÀ DỌN DẸP ---
	downloadDir := c.getDownloadTempDir()

	var latestZip string

	// Check mỗi 500ms, chờ tối đa 30s vì hệ thống thuế có lúc phản hồi rất chậm
	err := c.retry(60, 500*time.Millisecond, func() bool {
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
		return fmt.Errorf("❌ Không tìm thấy file Zip hợp lệ sau 30 giây chờ đợi")
	}

	// Dùng c.OutputDir đã khởi tạo từ đầu
	err = c.ExtractAndMove(latestZip, c.OutputDir, fileName)

	if err != nil {
		return fmt.Errorf("không giải nén được file hóa đơn: %v", err)
	}

	fmt.Printf("   ✅ Đã lưu vào: %s\n", c.OutputDir)
	os.Remove(latestZip)

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
		if c.StopRequested {
			return fmt.Errorf("tiến trình đã bị người dùng dừng")
		}
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

func isInvoiceStatusText(s string) bool {
	sLower := strings.ToLower(strings.TrimSpace(s))
	if sLower == "" {
		return false
	}
	if strings.Contains(sLower, "hóa đơn") ||
		strings.Contains(sLower, "mới") ||
		strings.Contains(sLower, "thay thế") ||
		strings.Contains(sLower, "điều chỉnh") ||
		strings.Contains(sLower, "hủy") ||
		strings.Contains(sLower, "xóa bỏ") {
		return true
	}
	hasDigit := false
	for _, r := range s {
		if unicode.IsDigit(r) {
			hasDigit = true
			break
		}
	}
	return !hasDigit
}

func isNumericOnly(s string) bool {
	sClean := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), ".", ""), ",", ""), " ", "")
	if sClean == "" {
		return false
	}
	for _, r := range sClean {
		if !unicode.IsDigit(r) && r != '-' && r != '+' {
			return false
		}
	}
	return true
}

func parseMoney(s string) float64 {
	clean := strings.ReplaceAll(strings.TrimSpace(s), ".", "")
	clean = strings.ReplaceAll(clean, ",", ".")
	f, _ := strconv.ParseFloat(clean, 64)
	return f
}

func formatMoney(val float64) string {
	intVal := int64(val)
	s := strconv.FormatInt(intVal, 10)
	n := len(s)
	if n <= 3 {
		return s
	}
	var res []byte
	rem := n % 3
	if rem > 0 {
		res = append(res, s[:rem]...)
	}
	for i := rem; i < n; i += 3 {
		if len(res) > 0 {
			res = append(res, '.')
		}
		res = append(res, s[i:i+3]...)
	}
	return string(res)
}

// selectCheckResult: Chọn mục trong dropdown "Kết quả kiểm tra (*)" trên trang thuế
func (c *CrawlerService) selectCheckResult(checkResult string) error {
	if c.Page == nil {
		return fmt.Errorf("page is nil")
	}

	target := strings.TrimSpace(checkResult)
	if target == "" {
		return nil
	}

	// Map legacy keys if passed
	switch target {
	case "co_ma":
		target = "Đã cấp mã hóa đơn"
	case "khong_ma":
		target = "Cục Thuế đã nhận không mã"
	case "may_tinh_tien":
		target = "Cục Thuế đã nhận hóa đơn có mã khởi tạo từ máy tính tiền"
	case "tat_ca", "tat ca", "all":
		target = "Tất cả"
	}

	fmt.Printf("🔍 Bắt đầu chọn 'Kết quả kiểm tra' mục tiêu: [%s]\n", target)

	var selectErr error
	rodErr := rod.Try(func() {
		page := c.Page.Timeout(15 * time.Second)

		res, err := page.Eval(`async (target) => {
			// Dọn dẹp inline style display nếu có
			document.querySelectorAll('.ant-select-dropdown').forEach(d => {
				d.style.removeProperty('display');
				d.style.removeProperty('visibility');
			});

			const normTarget = target.trim().toLowerCase();

			// Tìm ô #ttxly thuộc tab đang ACTIVE (bỏ qua tab ẩn)
			const allBoxes = Array.from(document.querySelectorAll('#ttxly, [id*="ttxly"]'));
			let activeBox = allBoxes.find(b => {
				if (b.closest('.ant-tabs-tabpane-inactive')) return false;
				const r = b.getBoundingClientRect();
				return r.width > 0 && r.height > 0;
			});
			if (!activeBox) {
				activeBox = allBoxes.find(b => !b.closest('.ant-tabs-tabpane-inactive')) || allBoxes[0];
			}
			if (!activeBox) return { ok: false, msg: 'Không tìm thấy ô combobox #ttxly' };

			const valEl = activeBox.querySelector('.ant-select-selection-selected-value') || activeBox;
			const curVal = (valEl.getAttribute('title') || valEl.innerText || '').trim();

			// Nếu đã đúng giá trị mục tiêu thì hoàn thành ngay không cần click
			if (curVal.toLowerCase() === normTarget) {
				return { ok: true, already: true, val: curVal };
			}

			const combo = activeBox.querySelector('.ant-select-selection') || activeBox.querySelector('[role="combobox"]') || activeBox;
			combo.scrollIntoView({ block: 'nearest' });
			combo.focus();
			['mousedown', 'mouseup', 'click'].forEach(evt => {
				combo.dispatchEvent(new MouseEvent(evt, { bubbles: true, cancelable: true, view: window }));
			});
			if (typeof combo.click === 'function') combo.click();

			const arrow = activeBox.querySelector('.ant-select-arrow');
			if (arrow) {
				arrow.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window }));
			}

			// Tìm dropdown popup
			const ctrlId = combo.getAttribute('aria-controls');
			let matched = null;

			for (let i = 0; i < 20; i++) {
				let items = [];
				if (ctrlId) {
					const root = document.getElementById(ctrlId);
					if (root) items = Array.from(root.querySelectorAll('li'));
				}
				if (items.length === 0) {
					const openDropdowns = Array.from(document.querySelectorAll('.ant-select-dropdown:not(.ant-select-dropdown-hidden)'));
					for (const dd of openDropdowns) {
						const list = Array.from(dd.querySelectorAll('li'));
						if (list.length > 0) {
							items = list;
							break;
						}
					}
				}
				if (items.length === 0) {
					items = Array.from(document.querySelectorAll('li[role="option"]')).filter(li => {
						const r = li.getBoundingClientRect();
						return r.width > 0 && r.height > 0;
					});
				}

				if (items.length > 0) {
					matched = items.find(li => (li.innerText || li.textContent || '').trim().toLowerCase() === normTarget);
					if (!matched) {
						matched = items.find(li => (li.innerText || li.textContent || '').trim().toLowerCase().includes(normTarget));
					}
					if (matched) break;
				}

				if (i === 3 || i === 8) {
					if (arrow) arrow.click();
					combo.focus();
					combo.click();
					combo.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', keyCode: 40, bubbles: true }));
				}

				await new Promise(r => setTimeout(r, 60));
			}

			if (!matched) {
				return { 
					ok: false, 
					msg: 'Không tìm thấy option [' + target + ']', 
					curVal: curVal 
				};
			}

			// Kích hoạt sự kiện chuột lên option
			matched.scrollIntoView({ block: 'nearest' });
			['mouseenter', 'mouseover', 'mousedown', 'mouseup', 'click'].forEach(evt => {
				matched.dispatchEvent(new MouseEvent(evt, {
					bubbles: true,
					cancelable: true,
					view: window,
					button: 0,
					buttons: 1
				}));
			});

			await new Promise(r => setTimeout(r, 100));
			if (document.activeElement) document.activeElement.blur();

			const finalVal = (valEl.getAttribute('title') || valEl.innerText || '').trim();
			return { ok: true, val: finalVal };
		}`, target)

		if err != nil {
			selectErr = fmt.Errorf("lỗi eval JS: %w", err)
			fmt.Printf("❌ Lỗi eval khi chọn 'Kết quả kiểm tra' [%s]: %v\n", target, err)
			return
		}

		if res != nil {
			ok := res.Value.Get("ok").Bool()
			val := res.Value.Get("val").Str()
			if ok {
				fmt.Printf("🎯 'Kết quả kiểm tra' sau khi chọn: [%s] (Mục tiêu: [%s])\n", val, target)
			} else {
				msg := res.Value.Get("msg").Str()
				selectErr = fmt.Errorf("%s", msg)
				fmt.Printf("❌ Thất bại khi chọn 'Kết quả kiểm tra' [%s]: %s\n", target, msg)
			}
		}
		time.Sleep(200 * time.Millisecond)
	})

	if rodErr != nil && selectErr == nil {
		selectErr = fmt.Errorf("lỗi rod runtime: %w", rodErr)
		fmt.Printf("❌ Lỗi rod khi chọn 'Kết quả kiểm tra' [%s]: %v\n", target, rodErr)
	}

	if selectErr != nil {
		c.logError("selectCheckResult", selectErr, fmt.Sprintf("Mục tiêu: %s", target))
	}

	return selectErr
}
