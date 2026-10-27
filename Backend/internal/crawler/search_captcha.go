package crawler

import (
	"TAXSOLUTION/internal/captcha"
	"TAXSOLUTION/internal/models"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

// Captcha khi tìm kiếm hóa đơn có mã khởi tạo từ máy tính tiền (MTT).
// Cổng thuế hiện popup "Nhập mã captcha" mỗi lần bấm Tìm kiếm (không hỏi khi chuyển trang / tải XML).
// Bot KHÔNG tự giải captcha: ảnh được gửi về app để người dùng tự nhập, bot chỉ điền mã và bấm Xác nhận.

const (
	searchCaptchaMarker      = "data-search-captcha"
	searchCaptchaAppearWait  = 6 * time.Second // chờ popup xuất hiện sau khi bấm Tìm kiếm
	searchCaptchaUserTimeout = 5 * time.Minute // tối đa chờ người dùng nhập mỗi lần
	searchCaptchaMaxAttempts = 5               // số lần nhập sai tối đa
	searchCaptchaKeepAlive   = 30 * time.Second
)

// ErrNoSearchCaptchaPending: không có captcha tìm kiếm nào đang chờ nhập.
var ErrNoSearchCaptchaPending = errors.New("không có captcha tìm kiếm nào đang chờ nhập")

// markSearchCaptchaModal: tìm popup captcha tìm kiếm đang hiển thị (có #cvalue nhưng KHÔNG có #username
// để không nhầm với form đăng nhập) và gắn thuộc tính đánh dấu để các bước sau thao tác đúng popup.
func (c *CrawlerService) markSearchCaptchaModal() bool {
	found := false
	_ = rod.Try(func() {
		res := c.Page.Timeout(3*time.Second).MustEval(`(marker) => {
			document.querySelectorAll('[' + marker + ']').forEach(e => e.removeAttribute(marker));
			const visible = (el) => el && el.getClientRects().length > 0;
			const modal = Array.from(document.querySelectorAll('.ant-modal-content')).find(m =>
				m.querySelector('#cvalue') && !m.querySelector('#username') && visible(m));
			if (!modal) return false;
			modal.setAttribute(marker, '1');
			return true;
		}`, searchCaptchaMarker)
		found = res.Bool()
	})
	return found
}

// waitSearchCaptchaModal: chờ popup captcha xuất hiện trong khoảng thời gian cho phép.
func (c *CrawlerService) waitSearchCaptchaModal(ctx context.Context, maxWait time.Duration) bool {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil || c.StopRequested {
			return false
		}
		if c.markSearchCaptchaModal() {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

// captureSearchCaptcha: lấy ảnh captcha dưới dạng data URL.
// Ưu tiên đọc thẳng src (cổng thuế dùng SVG base64), nếu không được thì chụp ảnh phần tử.
func (c *CrawlerService) captureSearchCaptcha() (string, error) {
	var dataURL string
	err := rod.Try(func() {
		sel := fmt.Sprintf("[%s] img", searchCaptchaMarker)
		img := c.Page.Timeout(5 * time.Second).MustElement(sel)
		img.MustWaitVisible()
		time.Sleep(200 * time.Millisecond)

		if src := img.MustEval(`() => this.getAttribute('src') || ''`).Str(); strings.HasPrefix(src, "data:image/") {
			dataURL = src
			return
		}
		tmpFile := filepath.Join(os.TempDir(), "taxsolution_search_captcha.png")
		data := img.MustScreenshot(tmpFile)
		_ = os.Remove(tmpFile)
		dataURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	})
	if err != nil {
		return "", fmt.Errorf("không lấy được ảnh captcha: %v", err)
	}
	if dataURL == "" {
		return "", errors.New("ảnh captcha rỗng")
	}
	return dataURL, nil
}

// sendStream: gửi tiến trình về app nhưng không bị treo nếu app đã ngắt kết nối.
func sendStream(ctx context.Context, stream chan models.StreamData, data models.StreamData) {
	if stream == nil {
		return
	}
	select {
	case stream <- data:
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
	}
}

// handleSearchCaptcha: gọi ngay sau khi bấm Tìm kiếm (chỉ với hóa đơn MTT).
// Trả về nil khi không có popup hoặc đã nhập đúng; trả về lỗi khi người dùng hủy / hết giờ / sai quá số lần.
func (c *CrawlerService) handleSearchCaptcha(ctx context.Context, stream chan models.StreamData) error {
	if !c.waitSearchCaptchaModal(ctx, searchCaptchaAppearWait) {
		if ctx.Err() != nil || c.StopRequested {
			return errors.New("thao tác đã bị hủy")
		}
		// Cổng thuế không hỏi captcha lần này → chạy tiếp như bình thường
		return nil
	}
	fmt.Println("🔐 Cổng thuế yêu cầu nhập captcha cho tra cứu hóa đơn MTT.")

	status := "🔐 Cổng thuế yêu cầu nhập mã captcha. Vui lòng nhập mã trên màn hình."
	for attempt := 1; attempt <= searchCaptchaMaxAttempts; attempt++ {
		img, err := c.captureSearchCaptcha()
		if err != nil {
			c.cancelSearchCaptchaModal()
			return err
		}

		var code string
		// Thử giải tự động bằng Gemini AI trước nếu có cấu hình
		var aiSolved bool
		var imgBytes []byte
		if commaIdx := strings.Index(img, ","); commaIdx != -1 {
			imgBytes, _ = base64.StdEncoding.DecodeString(img[commaIdx+1:])
		}
		if len(imgBytes) > 0 && captcha.Enabled() {
			sendStream(ctx, stream, models.StreamData{Status: "🤖 Đang giải mã CAPTCHA  ..."})
			if aiCode, aiErr := captcha.SolveCaptchaImage(ctx, imgBytes); aiErr == nil && len(aiCode) == 6 {
				fmt.Printf("Giải captcha được: %s\n", aiCode)
				code = aiCode
				aiSolved = true
			} else if aiErr != nil {
				fmt.Printf("⚠️  Giải captcha thất bại: %v\n", aiErr)
			}
		}

		// Nếu AI không giải được hoặc chưa cấu hình API key, rơi về chờ người dùng nhập
		if !aiSolved {
			var errWait error
			code, errWait = c.waitSearchCaptchaCode(ctx, stream, status, img)
			if errWait != nil {
				c.cancelSearchCaptchaModal()
				return errWait
			}
		}

		ok, closed, errMsg := c.submitSearchCaptchaCode(code)
		if ok {
			fmt.Println("✅ Captcha tra cứu MTT hợp lệ, tiếp tục tìm kiếm.")
			sendStream(ctx, stream, models.StreamData{Status: "✅ Đã xác nhận captcha, đang tìm kiếm..."})
			return nil
		}
		if closed {
			// Cổng thuế đóng popup nhưng báo sai mã → không thể nhập lại trên popup cũ
			return fmt.Errorf("cổng thuế báo: %s. Vui lòng tìm kiếm lại", errMsg)
		}

		fmt.Printf("⚠️ Captcha sai (lần %d/%d): %s\n", attempt, searchCaptchaMaxAttempts, errMsg)
		status = "⚠️ Mã captcha không đúng, vui lòng nhập lại."
		if errMsg != "" {
			status = "⚠️ " + errMsg + " — vui lòng nhập lại."
		}
		// Cổng thuế thường tự đổi ảnh khi sai; đánh dấu lại popup trước khi chụp ảnh mới
		time.Sleep(500 * time.Millisecond)
		if !c.markSearchCaptchaModal() {
			// Popup đã đóng dù không xác nhận được là đúng → để các bước sau tự kiểm tra kết quả
			return nil
		}
	}

	c.cancelSearchCaptchaModal()
	return fmt.Errorf("nhập sai captcha quá %d lần, vui lòng tìm kiếm lại", searchCaptchaMaxAttempts)
}

// waitSearchCaptchaCode: gửi ảnh captcha về app rồi chờ người dùng nhập mã.
func (c *CrawlerService) waitSearchCaptchaCode(ctx context.Context, stream chan models.StreamData, status, img string) (string, error) {
	ch := make(chan string, 1)
	c.searchCaptchaMu.Lock()
	c.searchCaptchaCh = ch
	c.searchCaptchaMu.Unlock()
	defer func() {
		c.searchCaptchaMu.Lock()
		if c.searchCaptchaCh == ch {
			c.searchCaptchaCh = nil
		}
		c.searchCaptchaMu.Unlock()
	}()

	c.CurrentStatus = "Đang chờ nhập captcha..."
	sendStream(ctx, stream, models.StreamData{Status: status, Captcha: img})

	timeout := time.NewTimer(searchCaptchaUserTimeout)
	defer timeout.Stop()
	keepAlive := time.NewTicker(searchCaptchaKeepAlive)
	defer keepAlive.Stop()
	stopCheck := time.NewTicker(500 * time.Millisecond)
	defer stopCheck.Stop()

	for {
		select {
		case code := <-ch:
			return code, nil
		case <-ctx.Done():
			return "", errors.New("thao tác đã bị hủy")
		case <-stopCheck.C:
			if c.StopRequested {
				return "", errors.New("thao tác đã bị hủy")
			}
		case <-keepAlive.C:
			// Giữ kết nối và gia hạn watchdog giao diện (không gửi lại ảnh để tránh mở popup lần nữa)
			sendStream(ctx, stream, models.StreamData{Status: "🔐 Đang chờ nhập mã captcha..."})
		case <-timeout.C:
			return "", errors.New("quá thời gian chờ nhập captcha")
		}
	}
}

// submitSearchCaptchaCode: điền mã vào popup của cổng thuế và bấm Xác nhận.
// Kết quả: ok = mã đúng; closed = popup đã đóng; errMsg = thông báo lỗi của cổng thuế (nếu có).
func (c *CrawlerService) submitSearchCaptchaCode(code string) (ok bool, closed bool, errMsg string) {
	// Xóa thông báo cũ để không đọc nhầm lỗi của lần trước
	c.dismissNotifications()

	inputSel := fmt.Sprintf("[%s] #cvalue", searchCaptchaMarker)
	err := rod.Try(func() {
		el := c.Page.Timeout(5 * time.Second).MustElement(inputSel)
		el.MustClick().MustSelectAllText().MustInput(code)
		el.MustEval(`() => {
			this.dispatchEvent(new Event('input', { bubbles: true }));
			this.dispatchEvent(new Event('change', { bubbles: true }));
		}`)
		time.Sleep(150 * time.Millisecond)

		btnSel := fmt.Sprintf("[%s] button[type='submit']", searchCaptchaMarker)
		if btn, errBtn := c.Page.Timeout(2 * time.Second).Element(btnSel); errBtn == nil {
			btn.MustClick()
		} else {
			c.Page.Timeout(2*time.Second).MustElementR(fmt.Sprintf("[%s] button", searchCaptchaMarker), "Xác nhận").MustClick()
		}
	})
	if err != nil {
		return false, false, "không điền được mã captcha"
	}

	// Chờ popup đóng (đúng) hoặc cổng thuế báo lỗi (sai)
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)

		var stillOpen bool
		var errText string
		_ = rod.Try(func() {
			res := c.Page.Timeout(3*time.Second).MustEval(`(marker) => {
				const modal = document.querySelector('[' + marker + ']');
				const open = !!modal && modal.isConnected && modal.getClientRects().length > 0;
				const texts = [];
				if (open) modal.querySelectorAll('.ant-form-explain, .ant-form-item-explain').forEach(e => texts.push(e.innerText));
				document.querySelectorAll('.ant-notification-notice-message, .ant-notification-notice-description, .ant-message-notice')
					.forEach(e => texts.push(e.innerText));
				const err = texts.map(t => (t || '').trim()).find(t => t) || '';
				return { open, err };
			}`, searchCaptchaMarker)
			stillOpen = res.Get("open").Bool()
			errText = res.Get("err").Str()
		})

		if !stillOpen {
			if isCaptchaErrorText(errText) {
				return false, true, errText
			}
			return true, true, ""
		}
		if errText != "" {
			return false, false, errText
		}
	}
	return false, false, ""
}

// isCaptchaErrorText: thông báo của cổng thuế có phải là lỗi sai captcha không.
func isCaptchaErrorText(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "captcha") || strings.Contains(l, "mã xác") || strings.Contains(l, "mã bảo vệ") || strings.Contains(l, "mã kiểm tra")
}

// cancelSearchCaptchaModal: bấm Hủy trên popup captcha của cổng thuế (khi người dùng hủy / hết giờ).
func (c *CrawlerService) cancelSearchCaptchaModal() {
	_ = rod.Try(func() {
		c.Page.Timeout(3*time.Second).MustEval(`(marker) => {
			const modal = document.querySelector('[' + marker + ']');
			if (!modal) return;
			const btn = Array.from(modal.querySelectorAll('button')).find(b => (b.innerText || '').trim() === 'Hủy')
				|| document.querySelector('.ant-modal-close');
			if (btn) btn.click();
			modal.removeAttribute(marker);
		}`, searchCaptchaMarker)
	})
}

// dismissNotifications: đóng các thông báo cũ (vd "sai captcha") để bước kiểm tra lỗi sau tìm kiếm không đọc nhầm.
func (c *CrawlerService) dismissNotifications() {
	_ = rod.Try(func() {
		c.Page.Timeout(3 * time.Second).MustEval(`() => {
			document.querySelectorAll('.ant-notification-notice-close').forEach(b => b.click());
			document.querySelectorAll('.ant-notification-notice, .ant-message-notice').forEach(n => n.remove());
		}`)
	})
	time.Sleep(200 * time.Millisecond)
}

// SubmitSearchCaptcha: nhận mã captcha người dùng nhập từ app (gọi từ API).
func (c *CrawlerService) SubmitSearchCaptcha(code string) error {
	c.searchCaptchaMu.Lock()
	ch := c.searchCaptchaCh
	c.searchCaptchaMu.Unlock()
	if ch == nil {
		return ErrNoSearchCaptchaPending
	}
	select {
	case ch <- code:
		return nil
	default:
		return errors.New("mã captcha đang được xử lý, vui lòng chờ")
	}
}

// RefreshSearchCaptcha: bấm nút đổi mã trên popup của cổng thuế và trả về ảnh mới.
func (c *CrawlerService) RefreshSearchCaptcha() (string, error) {
	c.searchCaptchaMu.Lock()
	pending := c.searchCaptchaCh != nil
	c.searchCaptchaMu.Unlock()
	if !pending {
		return "", ErrNoSearchCaptchaPending
	}
	if !c.markSearchCaptchaModal() {
		return "", errors.New("không tìm thấy popup captcha trên cổng thuế")
	}

	old, _ := c.captureSearchCaptcha()
	err := rod.Try(func() {
		c.Page.Timeout(3 * time.Second).MustElement(fmt.Sprintf("[%s] button.ant-btn-icon-only", searchCaptchaMarker)).MustClick()
	})
	if err != nil {
		return "", fmt.Errorf("không bấm được nút đổi mã: %v", err)
	}

	// Chờ ảnh mới khác ảnh cũ (tối đa ~3 giây)
	for i := 0; i < 10; i++ {
		time.Sleep(300 * time.Millisecond)
		img, errCap := c.captureSearchCaptcha()
		if errCap == nil && img != old {
			return img, nil
		}
	}
	return c.captureSearchCaptcha()
}
