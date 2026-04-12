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
	STT         string `json:"stt"`
	KyHieu      string `json:"ky_hieu"`
	SoHoaDon    string `json:"so_hoa_don"`
	NgayLap     string `json:"ngay_lap"`
	NguoiBan    string `json:"nguoi_ban"`
	MSTNguoiBan string `json:"mst_nguoi_ban"`
	TongTien    string `json:"tong_tien"`
	TrangThai   string `json:"trang_thai"`
}
type CrawlResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	TaskID  string `json:"task_id,omitempty"`
}
