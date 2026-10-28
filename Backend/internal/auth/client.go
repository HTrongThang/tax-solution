// Package auth gọi Google Apps Script (Google Sheet) để xác thực tài khoản phần mềm.
//
// Cấu hình trong Backend/.env:
//
//	AUTH_SCRIPT_URL=https://script.google.com/macros/s/.../exec
//	AUTH_API_KEY=chuoi_bi_mat   (trùng Script property API_KEY)
//
// API key KHÔNG được ghi vào log.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Result là mã kết quả script trả về.
type Result string

const (
	ResultOK               Result = "OK"
	ResultWrongCredentials Result = "WRONG_CREDENTIALS"
	ResultPending          Result = "PENDING"
	ResultLocked           Result = "LOCKED"
	ResultWrongDevice      Result = "WRONG_DEVICE"
	ResultNotStarted       Result = "NOT_STARTED"
	ResultExpired          Result = "EXPIRED"
	ResultExists           Result = "EXISTS"
	ResultUnauthorized     Result = "UNAUTHORIZED"
	ResultBadRequest       Result = "BAD_REQUEST"
	ResultError            Result = "ERROR"
)

// Response là phản hồi của script.
type Response struct {
	Result         Result `json:"result"`
	Message        string `json:"message,omitempty"`
	FullName       string `json:"fullName,omitempty"`
	MaxTaxAccounts int    `json:"maxTaxAccounts,omitempty"`
	AllowAI        bool   `json:"allowAI,omitempty"`
	StartDate      string `json:"startDate,omitempty"`
	EndDate        string `json:"endDate,omitempty"`
}

var (
	// ErrNotConfigured: thiếu hoặc sai AUTH_SCRIPT_URL / AUTH_API_KEY.
	ErrNotConfigured = errors.New("chưa cấu hình AUTH_SCRIPT_URL / AUTH_API_KEY trong .env")
	// ErrNetwork: không gọi được máy chủ xác thực (mất mạng, timeout, HTTP lỗi...).
	ErrNetwork = errors.New("không kết nối được máy chủ xác thực")
)

const (
	requestTimeout = 10 * time.Second
	maxBodySize    = 1 << 20 // 1 MB
)

// Chỉ cho phép gọi tới Google Apps Script (không nhận URL tùy ý).
var allowedHosts = map[string]bool{
	"script.google.com":             true,
	"script.googleusercontent.com": true,
}

// Client gọi script xác thực.
type Client struct {
	URL    string
	APIKey string
	HTTP   *http.Client

	// allowAnyURL chỉ dùng trong test (httptest chạy http://127.0.0.1).
	allowAnyURL bool
}

// NewFromEnv tạo Client từ biến môi trường (đã nạp từ .env).
func NewFromEnv() *Client {
	return &Client{
		URL:    strings.TrimSpace(os.Getenv("AUTH_SCRIPT_URL")),
		APIKey: strings.TrimSpace(os.Getenv("AUTH_API_KEY")),
		HTTP:   &http.Client{Timeout: requestTimeout},
	}
}

// Configured báo đã đủ cấu hình hợp lệ chưa.
func (c *Client) Configured() bool {
	return c.validate() == nil
}

func (c *Client) validate() error {
	if c.APIKey == "" || c.URL == "" {
		return ErrNotConfigured
	}
	if c.allowAnyURL {
		return nil
	}
	u, err := url.Parse(c.URL)
	if err != nil || u.Scheme != "https" || !allowedHosts[u.Hostname()] {
		return fmt.Errorf("%w: AUTH_SCRIPT_URL phải là https://script.google.com/...", ErrNotConfigured)
	}
	return nil
}

// Call gửi một action tới script. fields là các trường kèm theo (username, passHash, hwid...).
// Lỗi kết nối trả về ErrNetwork; mã nghiệp vụ (sai mật khẩu, hết hạn...) nằm trong Response.Result.
func (c *Client) Call(ctx context.Context, action string, fields map[string]any) (*Response, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	payload := make(map[string]any, len(fields)+2)
	for k, v := range fields {
		payload[k] = v
	}
	payload["action"] = action
	payload["apiKey"] = c.APIKey

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	// text/plain tránh preflight; script vẫn đọc được postData.contents.
	// Go tự đổi POST -> GET khi script chuyển hướng (302), đúng cách Apps Script yêu cầu.
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNetwork, sanitize(err, c.APIKey))
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrNetwork, res.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(res.Body, maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNetwork, err)
	}

	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%w: phản hồi không hợp lệ", ErrNetwork)
	}
	if out.Result == "" {
		out.Result = ResultError
	}
	return &out, nil
}

// sanitize bảo đảm API key không lọt vào thông báo lỗi/log.
func sanitize(err error, secret string) string {
	msg := err.Error()
	if secret != "" {
		msg = strings.ReplaceAll(msg, secret, "***")
	}
	return msg
}

