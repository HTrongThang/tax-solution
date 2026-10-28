package taxapi

import (
	"TAXSOLUTION/internal/captcha"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// geminiSolve đọc chữ captcha bằng bộ giải của app (Gemini).
func geminiSolve(ctx context.Context, png []byte) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	return captcha.SolveCaptchaImage(cctx, png)
}

// solveCaptcha lấy captcha mới từ /api/captcha ({key, content=SVG}), vẽ SVG thành PNG
// bằng Chrome đang mở rồi nhờ Solve đọc chữ. Trả về (ckey, cvalue).
func (c *Client) solveCaptcha(ctx context.Context) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/captcha", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("lấy captcha: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBodySize))
	if err != nil || res.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("lấy captcha: HTTP %d", res.StatusCode)
	}
	var cp struct {
		Key     string `json:"key"`
		Content string `json:"content"`
	}
	if json.Unmarshal(body, &cp) != nil || cp.Key == "" || cp.Content == "" {
		return "", "", errors.New("captcha trả về không hợp lệ")
	}
	png, err := c.sess.RenderSVGToPNG(cp.Content)
	if err != nil {
		return "", "", err
	}
	code, err := c.Solve(ctx, png)
	if err != nil {
		return "", "", fmt.Errorf("giải captcha: %w", err)
	}
	return cp.Key, code, nil
}
