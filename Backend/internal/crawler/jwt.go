package crawler

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// GetJWT lấy giá trị cookie "jwt" của phiên đã đăng nhập vào cổng thuế.
// Dùng cho luồng gọi API trực tiếp. KHÔNG được ghi giá trị này vào log hay file.
func (c *CrawlerService) GetJWT() (string, error) {
	if c.Page == nil {
		return "", fmt.Errorf("trình duyệt chưa mở")
	}
	cookies, err := c.Page.Cookies([]string{"https://hoadondientu.gdt.gov.vn"})
	if err != nil {
		return "", fmt.Errorf("không đọc được cookie: %v", err)
	}
	for _, ck := range cookies {
		if strings.EqualFold(ck.Name, "jwt") && strings.TrimSpace(ck.Value) != "" {
			return ck.Value, nil
		}
	}
	return "", fmt.Errorf("chưa có cookie jwt (có thể chưa đăng nhập)")
}

// RenderSVGToPNG vẽ một ảnh SVG (captcha do API trả về) thành PNG bằng canvas của
// Chrome đang mở, để đưa cho bộ giải captcha (Gemini không đọc được SVG).
func (c *CrawlerService) RenderSVGToPNG(svg string) ([]byte, error) {
	if c.Page == nil {
		return nil, fmt.Errorf("trình duyệt chưa mở")
	}
	res, err := c.Page.Eval(`(svg) => new Promise((resolve, reject) => {
		const url = URL.createObjectURL(new Blob([svg], {type: 'image/svg+xml'}));
		const img = new Image();
		img.onload = () => {
			const cv = document.createElement('canvas');
			cv.width = 400; cv.height = 80;
			const ctx = cv.getContext('2d');
			ctx.fillStyle = '#ffffff';
			ctx.fillRect(0, 0, cv.width, cv.height);
			ctx.drawImage(img, 0, 0, cv.width, cv.height);
			URL.revokeObjectURL(url);
			resolve(cv.toDataURL('image/png'));
		};
		img.onerror = () => reject(new Error('không nạp được SVG'));
		img.src = url;
	})`, svg)
	if err != nil {
		return nil, fmt.Errorf("không vẽ được captcha: %v", err)
	}
	data := res.Value.String()
	if i := strings.Index(data, ","); i >= 0 {
		data = data[i+1:]
	}
	return base64.StdEncoding.DecodeString(data)
}
