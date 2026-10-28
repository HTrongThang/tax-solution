package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Chỉ gọi đúng host của cổng thuế (không nhận URL từ người dùng).
const exportXMLURL = "https://hoadondientu.gdt.gov.vn/api/sco-query/invoices/export-xml"

// downloadXMLs tải XML của các hóa đơn đã lấy được bằng API export-xml.
// count <= 0: tải TẤT CẢ; count > 0: chỉ tải `count` hóa đơn đầu (để thử).
// File lưu vào thư mục TAX_XML_DIR (mặc định apitest_xml), quyền 0600.
func downloadXMLs(c *http.Client, token, cookie string, all []json.RawMessage, count int) {
	if len(all) == 0 {
		return
	}
	if count <= 0 || count > len(all) {
		count = len(all)
	}
	dir := strings.TrimSpace(os.Getenv("TAX_XML_DIR"))
	if dir == "" {
		dir = "apitest_xml"
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fmt.Printf("Không tạo được thư mục %s: %v\n", dir, err)
		return
	}
	fmt.Printf("Tải XML %d hóa đơn vào: %s\n", count, dir)

	ok := 0
	for i := 0; i < count; i++ {
		var inv invoice
		if json.Unmarshal(all[i], &inv) != nil {
			continue
		}
		name := fmt.Sprintf("%s_%v_%s_%v", inv.NBMST, inv.KHMSHDon, inv.KHHDon, inv.SHDon)
		files, err := fetchExportXML(c, token, cookie, inv)
		if err != nil {
			fmt.Printf("  [%s] LỖI: %v\n", name, err)
			continue
		}
		for _, f := range files {
			out := filepath.Join(dir, name+"_"+f.name)
			if err := os.WriteFile(out, f.data, 0o600); err != nil {
				fmt.Printf("  [%s] không ghi được %s: %v\n", name, f.name, err)
				continue
			}
			fmt.Printf("  [%s] đã lưu %s (%d byte)\n", name, filepath.Base(out), len(f.data))
		}
		ok++
		time.Sleep(300 * time.Millisecond) // nghỉ nhẹ giữa các lần gọi
	}
	fmt.Printf("Tải XML thành công %d/%d hóa đơn.\n", ok, count)
}

type extracted struct {
	name string
	data []byte
}

// fetchExportXML gọi export-xml. Nếu cổng đòi captcha (báo thiếu ckey) thì lấy captcha,
// giải rồi gọi lại một lần. Kết quả là file zip → trích các file .xml/.html bên trong.
func fetchExportXML(c *http.Client, token, cookie string, inv invoice) ([]extracted, error) {
	q := url.Values{}
	q.Set("nbmst", inv.NBMST)
	q.Set("khhdon", inv.KHHDon)
	q.Set("shdon", fmt.Sprintf("%v", inv.SHDon))
	q.Set("khmshdon", fmt.Sprintf("%v", inv.KHMSHDon))

	body, status, err := doExport(c, token, cookie, q)
	if err != nil && status != http.StatusOK && strings.Contains(string(body), "ckey") && captchaFn != nil {
		ckey, cvalue, cerr := captchaFn()
		if cerr != nil {
			return nil, fmt.Errorf("captcha: %w", cerr)
		}
		q.Set("ckey", ckey)
		q.Set("cvalue", cvalue)
		body, _, err = doExport(c, token, cookie, q)
	}
	if err != nil {
		return nil, err
	}
	return unzipInvoice(body)
}

func doExport(c *http.Client, token, cookie string, q url.Values) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, exportXMLURL+"?"+q.Encode(), nil)
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
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	res, err := c.Do(req)
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
		if len(snippet) > 160 {
			snippet = snippet[:160]
		}
		return body, res.StatusCode, fmt.Errorf("HTTP %d [%s]", res.StatusCode, snippet)
	}
	return body, res.StatusCode, nil
}

// unzipInvoice trích các file .xml/.html trong zip; chỉ dùng tên file gốc (chống zip-slip).
func unzipInvoice(data []byte) ([]extracted, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("phản hồi không phải file zip hợp lệ (%d byte)", len(data))
	}
	var out []extracted
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		ext := strings.ToLower(filepath.Ext(base))
		if f.FileInfo().IsDir() || (ext != ".xml" && ext != ".html") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxBodySize))
		rc.Close()
		if err != nil {
			continue
		}
		out = append(out, extracted{name: base, data: b})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("zip không chứa file .xml/.html")
	}
	return out, nil
}
