package taxapi

import (
	"TAXSOLUTION/internal/models"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Item là các trường tối thiểu của một hóa đơn trong danh sách do cổng trả về.
// Raw giữ nguyên dữ liệu thô để lưu/đối chiếu khi cần.
type Item struct {
	NBMST    string  `json:"nbmst"`
	KHMSHDon any     `json:"khmshdon"`
	KHHDon   string  `json:"khhdon"`
	SHDon    any     `json:"shdon"`
	TDLap    string  `json:"tdlap"`
	NBTen    string  `json:"nbten"`
	NBDChi   string  `json:"nbdchi"`
	NMMST    string  `json:"nmmst"`
	NMTen    string  `json:"nmten"`
	NMTNMua  string  `json:"nmtnmua"`
	NMDChi   string  `json:"nmdchi"`
	TgTCThue float64 `json:"tgtcthue"`
	TgTThue  float64 `json:"tgtthue"`
	TTCKTMai float64 `json:"ttcktmai"`
	TgTTTBSo float64 `json:"tgtttbso"`
	DVTTe    string  `json:"dvtte"`
	TThai    int     `json:"tthai"`
	TTXLy    int     `json:"ttxly"`

	Source Source          `json:"-"`
	Raw    json.RawMessage `json:"-"`
}

// numStr chuyển số/chuỗi bất kỳ (đặc biệt float64 từ json.Unmarshal vào any) sang chuỗi nguyên,
// tránh lỗi định dạng khoa học "1.234567e+06" khi số hóa đơn >= 1.000.000.
func numStr(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case float64:
		return fmt.Sprintf("%.0f", val)
	case json.Number:
		return val.String()
	case string:
		return strings.TrimSpace(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// Key định danh duy nhất một hóa đơn (dùng để loại trùng).
func (it Item) Key() string {
	return fmt.Sprintf("%s|%s|%s|%s", it.NBMST, numStr(it.KHMSHDon), it.KHHDon, numStr(it.SHDon))
}

func parseItem(raw json.RawMessage) (Item, bool) {
	var it Item
	if json.Unmarshal(raw, &it) != nil {
		return it, false
	}
	it.Raw = raw
	return it, true
}

// vnZone: giờ Việt Nam (cổng trả thời gian UTC, ví dụ 2026-10-02T17:00:00Z = 03/10 giờ VN).
var vnZone = time.FixedZone("ICT", 7*3600)

// dateVN trả về ngày lập theo giờ Việt Nam với layout cho trước.
func (it Item) dateVN(layout string) string {
	raw := strings.TrimSpace(it.TDLap)
	if raw == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.In(vnZone).Format(layout)
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.In(vnZone).Format(layout)
	}
	for _, f := range []string{"2006-01-02T15:04:05", "2006-01-02", "02/01/2006"} {
		if t, err := time.Parse(f, raw); err == nil {
			return t.Format(layout)
		}
	}
	return ""
}

// Số tiền theo định dạng vi-VN của giao diện (dấu chấm ngăn cách hàng nghìn).
func formatVND(v float64) string {
	n := int64(v + 0.5)
	if v < 0 {
		n = int64(v - 0.5)
	}
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	var b strings.Builder
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(ch)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// Trạng thái hóa đơn (tthai) và kết quả xử lý (ttxly) → chữ hiển thị.
// Mã không có trong bảng sẽ hiển thị nguyên số để dễ phát hiện mã mới.
var tthaiText = map[int]string{
	1: "Hóa đơn mới",
	2: "Hóa đơn thay thế",
	3: "Hóa đơn điều chỉnh",
	4: "Hóa đơn đã bị thay thế",
	5: "Hóa đơn đã bị điều chỉnh",
	6: "Hóa đơn đã bị hủy",
}

var ttxlyText = map[int]string{
	0: "Cục Thuế đã nhận",
	1: "Đang tiến hành kiểm tra điều kiện cấp mã",
	2: "CQT từ chối hóa đơn theo từng lần phát sinh",
	3: "Hóa đơn đủ điều kiện cấp mã",
	4: "Hóa đơn không đủ điều kiện cấp mã",
	5: "Đã cấp mã hóa đơn",
	6: "Cục Thuế đã nhận không mã",
	7: "Đã kiểm tra định kỳ HĐĐT không có mã",
	8: "Cục Thuế đã nhận hóa đơn có mã khởi tạo từ máy tính tiền",
}

// ParseCheckResult chuyển chuỗi "Kết quả kiểm tra" từ giao diện sang mã ttxly (0..8) của cổng thuế.
// Trả về (code, true) nếu lọc theo mã cụ thể; (-1, false) nếu là "Tất cả" hoặc rỗng.
func ParseCheckResult(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "Tất cả") {
		return -1, false
	}
	switch s {
	case "0", "Cục Thuế đã nhận", "Tổng cục Thuế đã nhận":
		return 0, true
	case "1", "Đang tiến hành kiểm tra điều kiện cấp mã":
		return 1, true
	case "2", "CQT từ chối hóa đơn theo từng lần phát sinh":
		return 2, true
	case "3", "Hóa đơn đủ điều kiện cấp mã":
		return 3, true
	case "4", "Hóa đơn không đủ điều kiện cấp mã":
		return 4, true
	case "5", "co_ma", "Đã cấp mã hóa đơn":
		return 5, true
	case "6", "khong_ma", "Cục Thuế đã nhận không mã", "Tổng cục Thuế đã nhận không mã":
		return 6, true
	case "7", "Đã kiểm tra định kỳ HĐĐT không có mã":
		return 7, true
	case "8", "may_tinh_tien",
		"Cục Thuế đã nhận hóa đơn có mã khởi tạo từ máy tính tiền",
		"Tổng cục Thuế đã nhận hóa đơn có mã khởi tạo từ máy tính tiền":
		return 8, true
	}
	return -1, false
}

// ItemsFromInvoices chuyển danh sách models.Invoice (từ luồng Chrome) sang []Item để tải XML qua API.
func ItemsFromInvoices(invs []models.Invoice, defaultSrc Source) []Item {
	out := make([]Item, 0, len(invs))
	for _, inv := range invs {
		shd := strings.TrimSpace(inv.SoHoaDon)
		if trimmed := strings.TrimLeft(shd, "0"); trimmed != "" {
			shd = trimmed
		}
		ttxly := 5
		if code, ok := ParseCheckResult(inv.KetQuaKiemTra); ok {
			ttxly = code
		} else if code, ok := ParseCheckResult(inv.KetQuaXuLy); ok {
			ttxly = code
		}
		itemSrc := defaultSrc
		if ttxly == 8 {
			itemSrc = MTT
		}
		out = append(out, Item{
			NBMST:    strings.TrimSpace(inv.MSTNguoiBan),
			KHMSHDon: strings.TrimSpace(inv.KyHieuMauSo),
			KHHDon:   strings.TrimSpace(inv.KyHieu),
			SHDon:    shd,
			TDLap:    strings.TrimSpace(inv.NgayLap),
			TTXLy:    ttxly,
			Source:   itemSrc,
		})
	}
	return out
}

func codeText(m map[int]string, code int) string {
	if s, ok := m[code]; ok {
		return s
	}
	return fmt.Sprintf("%d", code)
}

// ToInvoice đổi một hóa đơn của cổng sang models.Invoice mà giao diện đang dùng.
// stt: số thứ tự hiển thị (bắt đầu từ 1).
func (it Item) ToInvoice(stt int, kind Kind) models.Invoice {
	unit := it.DVTTe
	if unit == "" {
		unit = "VND"
	}

	// Đối tác hiển thị: bán ra → người mua; mua vào → người bán.
	buyerName := it.NMTen
	if buyerName == "" {
		buyerName = it.NMTNMua // khách lẻ không có MST: "BÁN CHO NGƯỜI TIÊU DÙNG"
	}
	var info strings.Builder
	if kind == Sold {
		if it.NMMST != "" {
			info.WriteString("MST người mua: " + it.NMMST + "\n")
		}
		info.WriteString("Tên người mua: " + buyerName + "\n")
		info.WriteString("Địa chỉ người mua: " + it.NMDChi)
	} else {
		info.WriteString("MST người bán: " + it.NBMST + "\n")
		info.WriteString("Tên người bán: " + it.NBTen + "\n")
		info.WriteString("Địa chỉ người bán: " + it.NBDChi)
	}

	return models.Invoice{
		STT:            fmt.Sprintf("%d", stt),
		MSTNguoiBan:    it.NBMST,
		KyHieuMauSo:    numStr(it.KHMSHDon),
		KyHieu:         it.KHHDon,
		SoHoaDon:       numStr(it.SHDon),
		NgayLap:        it.dateVN("02/01/2006"),
		ThongTinHoaDon: info.String(),
		TongChuaThue:   formatVND(it.TgTCThue),
		TongTienThue:   formatVND(it.TgTThue),
		ChietKhau:      formatVND(it.TTCKTMai),
		TongPhi:        "0",
		TongThanhToan:  formatVND(it.TgTTTBSo),
		DonViTienTe:    unit,
		TrangThai:      codeText(tthaiText, it.TThai),
		KetQuaXuLy:     codeText(ttxlyText, it.TTXLy),

		TenNguoiBan:    it.NBTen,
		DiaChiNguoiBan: it.NBDChi,
		MSTNguoiMua:    it.NMMST,
		TenNguoiMua:    buyerName,
		DiaChiNguoiMua: it.NBDChi,
	}
}
