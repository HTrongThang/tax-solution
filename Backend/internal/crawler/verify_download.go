package crawler

import (
	"archive/zip"
	"io"
	"regexp"
	"strings"
)

var (
	reXmlSHDon  = regexp.MustCompile(`<SHDon>\s*([^<]*?)\s*</SHDon>`)
	reXmlKHHDon = regexp.MustCompile(`<KHHDon>\s*([^<]*?)\s*</KHHDon>`)
)

// normInvoiceNo: bỏ khoảng trắng và số 0 đầu để so sánh số hóa đơn ("00004143" == "4143").
func normInvoiceNo(s string) string {
	s = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	t := strings.TrimLeft(s, "0")
	if t == "" && s != "" {
		return "0"
	}
	return t
}

// zipMatchesRow đọc file XML trong zip và đối chiếu Số HĐ + Ký hiệu với dòng đang xử lý trên bảng.
// Trả về (khớp, "ký hiệu/số" thực tế trong file). Nếu không đọc được hoặc dòng/file thiếu thông tin
// thì coi như khớp để không chặn nhầm.
func zipMatchesRow(zipPath, rowKyHieu, rowSoHD string) (bool, string) {
	if strings.TrimSpace(rowSoHD) == "" {
		return true, ""
	}
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return true, ""
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() || !strings.EqualFold(zipExt(f.Name), ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(rc, 4<<20))
		rc.Close()

		content := string(data)
		mSo := reXmlSHDon.FindStringSubmatch(content)
		if mSo == nil {
			continue
		}
		gotSo := mSo[1]
		gotKy := ""
		if mKy := reXmlKHHDon.FindStringSubmatch(content); mKy != nil {
			gotKy = mKy[1]
		}
		got := gotKy + "/" + gotSo

		if normInvoiceNo(gotSo) != normInvoiceNo(rowSoHD) {
			return false, got
		}
		if strings.TrimSpace(rowKyHieu) != "" && gotKy != "" &&
			!strings.EqualFold(strings.TrimSpace(gotKy), strings.TrimSpace(rowKyHieu)) {
			return false, got
		}
		return true, got
	}
	return true, ""
}

func zipExt(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i:]
	}
	return ""
}
