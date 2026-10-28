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

	Raw json.RawMessage `json:"-"`
}

// Key định danh duy nhất một hóa đơn (dùng để loại trùng).
func (it Item) Key() string {
	return fmt.Sprintf("%s|%v|%s|%v", it.NBMST, it.KHMSHDon, it.KHHDon, it.SHDon)
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
	t, err := time.Parse(time.RFC3339, it.TDLap)
	if err != nil {
		return ""
	}
	return t.In(vnZone).Format(layout)
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
	5: "Đã cấp mã hóa đơn",
	6: "Tổng cục Thuế đã nhận không mã",
	8: "Tổng cục Thuế đã nhận hóa đơn có mã khởi tạo từ máy tính tiền",
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
		KyHieuMauSo:    fmt.Sprintf("%v", it.KHMSHDon),
		KyHieu:         it.KHHDon,
		SoHoaDon:       fmt.Sprintf("%v", it.SHDon),
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
		DiaChiNguoiMua: it.NMDChi,
	}
}
