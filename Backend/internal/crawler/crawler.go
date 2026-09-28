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
	"unicode"

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
	StopRequested bool   // Cờ yêu cầu dừng tiến trình
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
		Headless(false) // Hiển thị cửa sổ Chrome

	// Tự tìm Chrome/Chromium được cài trên máy thay vì hardcode path
	if chromePath, found := launcher.LookPath(); found {
		l = l.Bin(chromePath)
		fmt.Printf("💻 Dùng trình duyệt từ: %s\n", chromePath)
	} else {
		fmt.Println("⚠️ Không tìm thấy Chrome tự động, dùng đường dẫn mặc định")
		l = l.Bin("C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe")
	}
	l.Append("--start-maximized")

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

		// ƯU TIÊN 2: KIỂM TRA LỖI (Đợi sau vòng lặp thứ 3 ~ 1 giây để tránh lỗi cũ)
		if i > 2 {
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
				return parseLoginError(msg)
			}
		}

		time.Sleep(300 * time.Millisecond)
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
		return parseLoginError(msg)
	}

	return fmt.Errorf("LOI_DANG_NHAP: Quá thời gian chờ đăng nhập hoặc thông tin đăng nhập không hợp lệ")
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
		fmt.Printf("⚠️ Lỗi kiểm tra tab (%s): %v\n", exactTargetText, err)
		return err
	}

	fmt.Printf("⚠️ Không tìm thấy tab [%s]\n", exactTargetText)
	return nil
}

func (c *CrawlerService) SearchAndGetInvoices(ctx context.Context, fromDate, toDate, invoiceType, category string, stream chan models.StreamData) ([]models.Invoice, error) {
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
	c.StopRequested = false

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
		if stream != nil {
			stream <- models.StreamData{Status: "❌ Trình duyệt chưa sẵn sàng", IsDone: true}
		}
		return nil, fmt.Errorf("page is nil - trình duyệt chưa sẵn sàng")
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
			if stream != nil {
				stream <- models.StreamData{Status: "❌ Không thể mở trang tra cứu", IsDone: true}
			}
			return nil, fmt.Errorf("lỗi điều hướng: %v", navErr)
		}
	} else {
		fmt.Println("📄 Đã ở trang Tra cứu, bỏ qua điều hướng.")
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

	// --- 2. TÌM KIẾM (chọn ngày thông minh: so sánh với anchor hiện tại) ---
	if stream != nil {
		stream <- models.StreamData{Status: fmt.Sprintf("📅 Đang chọn khoảng thời gian %s - %s...", fromDate, toDate), IsDone: false}
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
		stream <- models.StreamData{Status: "🔍 Đang bấm tìm kiếm...", IsDone: false}
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
		if ctx.Err() != nil || c.StopRequested {
			c.CurrentStatus = "Đã hủy"
			return allInvoices, fmt.Errorf("thao tác đã bị hủy")
		}

		// A. Đợi và lấy thông tin tổng số kết quả
		foundData := false
		for i := 0; i < 40; i++ {
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
			return allInvoices, fmt.Errorf("%s", errMsg)
		}

		// B. Cập nhật số trang từ tab đang active
		if pageIdx == 1 {
			var pageInfoText string
			_ = rod.Try(func() {
				res := c.Page.MustEval(`() => {
					const panes = Array.from(document.querySelectorAll('.ant-tabs-tabpane-active'));
					const activePane = panes[panes.length - 1] || document;
					const el = activePane.querySelector("div[class*='PageIndex'], .ant-pagination-simple-pager");
					return el ? el.innerText.trim() : '';
				}`)
				pageInfoText = res.Str()
			})
			if pageInfoText != "" {
				parts := strings.Split(pageInfoText, "/")
				if len(parts) == 2 {
					total, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
					if total > 0 {
						maxPages = total
					}
				}
			}
		}

		// C. Đợi bảng cập nhật (từ trang 2)
		if pageIdx > 1 {
			for i := 0; i < 30; i++ {
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

		fmt.Printf("📦 Trang %d: bóc được %d dòng hóa đơn (nhận diện %d cột tiêu đề).\n", pageIdx, len(rows), len(headers))

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
		if pageIdx >= maxPages {
			break
		}

		var nextClicked bool
		_ = rod.Try(func() {
			res := c.Page.MustEval(`() => {
				const panes = Array.from(document.querySelectorAll('.ant-tabs-tabpane-active'));
				const activePane = panes[panes.length - 1] || document;
				const btn = activePane.querySelector(".ant-pagination-next:not(.ant-pagination-disabled) button, button:has(.anticon-right):not([disabled])");
				if (btn && !btn.disabled) {
					btn.click();
					return true;
				}
				return false;
			}`)
			nextClicked = res.Bool()
		})
		if !nextClicked {
			break
		}
		time.Sleep(1500 * time.Millisecond)
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

	fmt.Printf("📅 Đang chọn %s cho ô %s...\n", dateStr, selector)

	// 1. Click ô date picker để mở popup calendar
	var clickErr error
	for retry := 0; retry < 3; retry++ {
		clickErr = rod.Try(func() {
			els, err := c.Page.Elements(selector)
			if err == nil && len(els) > 1 {
				for _, el := range els {
					if vis, errV := el.Visible(); errV == nil && vis {
						el.MustClick()
						return
					}
				}
			}
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
	// Quay về trang 1
	c.CurrentStatus = "Đang quay về trang đầu tiên..."
	fmt.Println("⏮️ Đang reset về trang 1 bằng nút mũi tên trái...")

	for i := 0; i < 200; i++ {
		if c.StopRequested {
			c.CurrentStatus = "Đã hủy"
			fmt.Println("🛑 Tiến trình cào đã bị người dùng hủy.")
			return
		}
		prevClicked := false
		_ = rod.Try(func() {
			res := c.Page.MustEval(`() => {
				const activePanes = Array.from(document.querySelectorAll('.ant-tabs-tabpane-active'))
					.filter(p => !p.closest('.ant-tabs-tabpane-inactive'));
				const container = activePanes[activePanes.length - 1] || document;
				const btn = container.querySelector("button i.anticon-left") ? container.querySelector("button i.anticon-left").closest("button") : null;
				if (btn && !btn.disabled && !btn.hasAttribute("disabled")) {
					btn.click();
					return true;
				}
				return false;
			}`)
			prevClicked = res.Bool()
		})
		if !prevClicked {
			fmt.Println("✅ Đã về trang 1 hoặc chỉ có 1 trang.")
			break
		}
		time.Sleep(400 * time.Millisecond)
	}
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
			res := c.Page.MustEval(fmt.Sprintf(`(marker) => {
				const activePanes = Array.from(document.querySelectorAll('.ant-tabs-tabpane-active'))
					.filter(p => !p.closest('.ant-tabs-tabpane-inactive'));
				const container = activePanes[activePanes.length - 1] || document;
				const activeRows = Array.from(container.querySelectorAll(".ant-table-tbody tr.ant-table-row"));
				activeRows.forEach((r, idx) => {
					r.setAttribute('data-crawl-row', marker + '-' + idx);
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

		for i, row := range rows {
			if c.StopRequested {
				c.CurrentStatus = "Đã hủy"
				fmt.Println("🛑 Tiến trình cào đã bị người dùng hủy.")
				break
			}
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

		// Kiểm tra nút sang trang trong tab đang active
		hasNext := false
		_ = rod.Try(func() {
			res := c.Page.MustEval(`() => {
				const activePanes = Array.from(document.querySelectorAll('.ant-tabs-tabpane-active'))
					.filter(p => !p.closest('.ant-tabs-tabpane-inactive'));
				const container = activePanes[activePanes.length - 1] || document;
				const btn = container.querySelector("button i.anticon-right") ? container.querySelector("button i.anticon-right").closest("button") : null;
				if (btn && !btn.disabled && !btn.hasAttribute("disabled")) {
					btn.click();
					return true;
				}
				return false;
			}`)
			hasNext = res.Bool()
		})

		if !hasNext {
			fmt.Printf("🏁 Đã cào xong trang cuối cùng (Trang %d). Dừng lại!\n", page)
			break
		}

		fmt.Println("⏭️ Chuyển sang trang tiếp theo...")
		// Đợi 3 giây để dữ liệu trang mới kịp thay đổi
		time.Sleep(3 * time.Second)
	}
}

func (c *CrawlerService) ProcessDownloadInvoice(row *rod.Element, index int) error {
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
		res := c.Page.MustEval(fmt.Sprintf(`(marker) => {
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
				target.setAttribute('data-dl-target', marker);
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
