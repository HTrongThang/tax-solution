// Package taxapi gọi THẲNG API của cổng hóa đơn điện tử (không điều khiển giao diện).
//
// Bố cục:
//   - client.go   : HTTP client, header chuẩn, retry (4xx → captcha mới, 5xx → chờ rồi thử lại)
//   - captcha.go  : lấy captcha SVG, vẽ thành PNG, nhờ bộ giải đọc chữ
//   - invoices.go : lấy danh sách hóa đơn (phân trang theo `state`)
//   - download.go : tải XML/HTML của hóa đơn (export-xml)
//   - mapper.go   : đổi dữ liệu JSON của cổng sang models.Invoice cho giao diện
//
// Token (JWT) và mật khẩu KHÔNG được ghi vào log.
package taxapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// Chỉ gọi đúng host của cổng thuế (không nhận URL từ người dùng).
	apiBase     = "https://hoadondientu.gdt.gov.vn/api"
	maxBodySize = 8 << 20 // giới hạn 8 MB mỗi phản hồi
)

// Session là phiên đăng nhập đang mở (crawler.CrawlerService thỏa interface này).
type Session interface {
	GetJWT() (string, error)
	RenderSVGToPNG(svg string) ([]byte, error)
}

// Client gọi API cổng thuế bằng JWT lấy từ Session.
type Client struct {
	http *http.Client
	sess Session
	// Solve đọc chữ trong ảnh captcha PNG (mặc định dùng Gemini, xem captcha.go).
	Solve func(ctx context.Context, png []byte) (string, error)
	// Logf in log tiến trình (nil = im lặng).
	Logf func(format string, args ...any)
}

// New tạo Client dùng phiên đăng nhập sess.
func New(sess Session) *Client {
	return &Client{
		http:  &http.Client{Timeout: 70 * time.Second},
		sess:  sess,
		Solve: geminiSolve,
	}
}

func (c *Client) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
}

// Chờ giữa các lần thử lại khi gặp 5xx / lỗi mạng.
var retryDelays = []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second}

// maxClientErrTries: số lần thử lại (captcha mới) khi cổng trả 4xx.
const maxClientErrTries = 3

// rateLimitDelays: chờ dần khi cổng trả 429 (quá nhiều yêu cầu). Thử lại dồn dập chỉ làm bị chặn lâu hơn.
var rateLimitDelays = []time.Duration{10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second}

// get gọi GET endpoint (đường dẫn sau /api) kèm query q.
// withCaptcha=true: mỗi lần gọi (kể cả thử lại) đều lấy và giải captcha MỚI.
// Trả về body khi HTTP 200.
func (c *Client) get(ctx context.Context, endpoint string, q url.Values, withCaptcha bool) ([]byte, error) {
	var lastErr error
	clientErrs := 0
	rateHits := 0
	for attempt := 0; attempt <= len(retryDelays); attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		body, status, err := c.once(ctx, endpoint, q, withCaptcha)
		if err == nil {
			return body, nil
		}
		lastErr = err

		// Cổng đòi captcha mà ta chưa gửi → bật captcha rồi gọi lại (không tính lần thử).
		if !withCaptcha && status >= 400 && status < 500 && strings.Contains(string(body), "ckey") {
			withCaptcha = true
			attempt--
			continue
		}
		// 429: cổng giới hạn tốc độ → nghỉ lâu rồi thử lại (không phải lỗi captcha, không tính vào lần thử).
		if status == http.StatusTooManyRequests {
			if rateHits >= len(rateLimitDelays) {
				return nil, fmt.Errorf("cổng thuế giới hạn tốc độ (429): %w", err)
			}
			wait := rateLimitDelays[rateHits]
			rateHits++
			c.logf("! %v — bị giới hạn tốc độ, nghỉ %v rồi thử lại (%d/%d)", err, wait, rateHits, len(rateLimitDelays))
			if !sleep(ctx, wait) {
				return nil, ctx.Err()
			}
			attempt--
			continue
		}
		// 4xx (thường do captcha giải sai): thử lại nhanh với captcha mới.
		if status >= 400 && status < 500 {
			clientErrs++
			if clientErrs >= maxClientErrTries {
				return nil, fmt.Errorf("bị từ chối: %w", err)
			}
			c.logf("! %v — thử lại với captcha mới (%d/%d)", err, clientErrs, maxClientErrTries)
			if !sleep(ctx, time.Second) {
				return nil, ctx.Err()
			}
			attempt--
			continue
		}
		if attempt < len(retryDelays) {
			c.logf("! %v — thử lại sau %v", err, retryDelays[attempt])
			if !sleep(ctx, retryDelays[attempt]) {
				return nil, ctx.Err()
			}
		}
	}
	return nil, lastErr
}

// once gọi đúng một lần. Trả về (body, status, err); body luôn có khi cổng có trả về.
func (c *Client) once(ctx context.Context, endpoint string, q url.Values, withCaptcha bool) ([]byte, int, error) {
	token, err := c.sess.GetJWT()
	if err != nil {
		return nil, 0, fmt.Errorf("không lấy được JWT: %w", err)
	}
	// Sao chép query để không làm đổi tham số của người gọi giữa các lần thử.
	qq := url.Values{}
	for k, v := range q {
		qq[k] = append([]string(nil), v...)
	}
	if withCaptcha {
		ckey, cvalue, cerr := c.solveCaptcha(ctx)
		if cerr != nil {
			return nil, 0, fmt.Errorf("captcha: %w", cerr)
		}
		qq.Set("ckey", ckey)
		qq.Set("cvalue", cvalue)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+endpoint+"?"+qq.Encode(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "vi")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	req.Header.Set("Referer", "https://hoadondientu.gdt.gov.vn/tra-cuu/tra-cuu-hoa-don")
	req.Header.Set("Origin", "https://hoadondientu.gdt.gov.vn")
	req.Header.Set("End-Point", "/tra-cuu/tra-cuu-hoa-don")
	req.Header.Set("Request-Id", newRequestID())
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("lỗi mạng: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBodySize))
	if err != nil {
		return nil, res.StatusCode, fmt.Errorf("đọc phản hồi: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		snippet := strings.Join(strings.Fields(string(body)), " ")
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		return body, res.StatusCode, fmt.Errorf("HTTP %d [%s]", res.StatusCode, snippet)
	}
	return body, res.StatusCode, nil
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// newRequestID tạo chuỗi dạng UUID v4 cho header Request-Id (giống trình duyệt).
func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// ErrNoData báo không có dữ liệu để xử lý.
var ErrNoData = errors.New("không có dữ liệu")
