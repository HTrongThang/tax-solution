package models

type LoginRequest struct {
	TaxCode  string `json:"tax_code"`
	Password string `json:"password"`
	Captcha  string `json:"captcha"`
}

type Response struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
type Invoice struct {
	STT              string `json:"stt"`                 // Cột 0
	MSTNguoiBan      string `json:"mst_nguoi_ban"`       // Cột 1
	KyHieuMauSo      string `json:"ky_hieu_mau_so"`      // Cột 2
	KyHieu           string `json:"ky_hieu"`             // Cột 3
	SoHoaDon         string `json:"so_hoa_don"`          // Cột 4
	NgayLap          string `json:"ngay_lap"`            // Cột 5
	ThongTinHoaDon   string `json:"thong_tin_hoa_don"`   // Cột 6
	TongChuaThue     string `json:"tong_chua_thue"`      // Cột 7
	TongTienThue     string `json:"tong_tien_thue"`      // Cột 8
	ChietKhau        string `json:"chiet_khau"`          // Cột 9
	TongPhi          string `json:"tong_phi"`            // Cột 10
	TongThanhToan    string `json:"tong_thanh_toan"`     // Cột 11
	DonViTienTe      string `json:"don_vi_tien_te"`      // Cột 12
	TrangThai        string `json:"trang_thai"`          // Cột 13
	KetQuaXuLy       string `json:"ket_qua_xu_ly"`       // Cột 14
	KetQuaKiemTra    string `json:"ket_qua_kiem_tra"`    // Cột 15
	HoaDonLienQuan   string `json:"hoa_don_lien_quan"`   // Cột 16
	ThongTinLienQuan string `json:"thong_tin_lien_quan"` // Cột 17
}
type CrawlResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	TaskID  string `json:"task_id,omitempty"`
}

type StreamData struct {
	Invoices    []Invoice `json:"invoices"`     // Dữ liệu trang vừa quét
	CurrentPage int       `json:"current_page"` // Trang vừa xong
	TotalSoFar  int       `json:"total_so_far"` // Tổng tích lũy
	TotalOnTax  int       `json:"total_on_tax"`
	Status      string    `json:"status"`  // Text hiển thị cho user
	IsDone      bool      `json:"is_done"` // Đã xong hết chưa?
}
