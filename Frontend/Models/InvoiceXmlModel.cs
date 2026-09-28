using System;
using System.Collections.Generic;
using Newtonsoft.Json;

namespace Frontend.Models
{
    /// <summary>
    /// Đối tượng chi tiết từng mặt hàng / dịch vụ trong hóa đơn (Thẻ &lt;HHDVu&gt;)
    /// Chuẩn hóa theo Nghị định 123/2020/NĐ-CP và Thông tư 78/2021/TT-BTC
    /// </summary>
    public class InvoiceXmlItemModel
    {
        /// <summary>
        /// Tính chất dòng: "1": Hàng hóa, dịch vụ; "2": Khuyến mại; "3": Chiết khấu thương mại; "4": Ghi chú / Diễn giải
        /// </summary>
        public string TChat { get; set; } = "1";

        /// <summary>
        /// Diễn giải tính chất dòng hàng (Hàng hóa, Chiết khấu, Khuyến mại...)
        /// </summary>
        public string TinhChatMoTa => TChat switch
        {
            "1" => "Hàng hóa, dịch vụ",
            "2" => "Khuyến mại",
            "3" => "Chiết khấu thương mại",
            "4" => "Ghi chú, diễn giải",
            _ => "Hàng hóa, dịch vụ"
        };

        /// <summary>
        /// Số thứ tự dòng hàng (&lt;STT&gt;)
        /// </summary>
        public int STT { get; set; }

        /// <summary>
        /// Mã hàng hóa, dịch vụ (&lt;MHHDVu&gt;)
        /// </summary>
        public string MHHDVu { get; set; } = string.Empty;

        /// <summary>
        /// Tên hàng hóa, dịch vụ (&lt;THHDVu&gt;)
        /// </summary>
        public string THHDVu { get; set; } = string.Empty;

        /// <summary>
        /// Đơn vị tính (&lt;DVTinh&gt;)
        /// </summary>
        public string DVTinh { get; set; } = string.Empty;

        /// <summary>
        /// Số lượng (&lt;SLuong&gt;)
        /// </summary>
        public decimal? SLuong { get; set; }

        /// <summary>
        /// Đơn giá (&lt;DGia&gt;)
        /// </summary>
        public decimal? DGia { get; set; }

        /// <summary>
        /// Thành tiền chưa thuế (&lt;ThTien&gt;)
        /// </summary>
        public decimal? ThTien { get; set; }

        /// <summary>
        /// Thuế suất chuỗi gốc (&lt;TSuat&gt;) VD: "8%", "10%", "5%", "0%", "KCT", "KKKNT"
        /// </summary>
        public string TSuat { get; set; } = string.Empty;

        /// <summary>
        /// Phần trăm thuế suất dạng số (VD: 8, 10, 5, 0 hoặc null nếu không chịu thuế)
        /// </summary>
        public decimal? ThueSuatPhanTram { get; set; }

        /// <summary>
        /// Tiền thuế của dòng hàng (&lt;TThue&gt;)
        /// </summary>
        public decimal? TThue { get; set; }
    }

    /// <summary>
    /// Đối tượng tờ hóa đơn điện tử bóc tách từ file XML
    /// Chuẩn hóa theo Nghị định 123/2020/NĐ-CP và Thông tư 78/2021/TT-BTC
    /// </summary>
    public class InvoiceXmlModel
    {
        #region Thông tin chung tờ hóa đơn (<TTChung>)
        /// <summary>
        /// Ký hiệu mẫu số hóa đơn (&lt;KHMSHDon&gt;) VD: "1", "2"
        /// </summary>
        public string KHMSHDon { get; set; } = string.Empty;

        /// <summary>
        /// Ký hiệu hóa đơn (&lt;KHHDon&gt;) VD: "1C24TAA", "C24TBB"
        /// </summary>
        public string KHHDon { get; set; } = string.Empty;

        /// <summary>
        /// Số hóa đơn (&lt;SHDon&gt;) VD: "25292", "0000102"
        /// </summary>
        public string SHDon { get; set; } = string.Empty;

        /// <summary>
        /// Ngày lập hóa đơn chuỗi gốc (&lt;NLap&gt;)
        /// </summary>
        public string NLapRaw { get; set; } = string.Empty;

        /// <summary>
        /// Ngày lập hóa đơn dạng DateTime đã phân tích
        /// </summary>
        public DateTime? NLap { get; set; }

        /// <summary>
        /// Đơn vị tiền tệ (&lt;DVTTe&gt;) Mặc định: "VND"
        /// </summary>
        public string DVTTe { get; set; } = "VND";

        /// <summary>
        /// Tỷ giá (&lt;TGia&gt;)
        /// </summary>
        public decimal TGia { get; set; } = 1;

        /// <summary>
        /// Hình thức thanh toán (&lt;HTTToan&gt;) VD: TM, CK, TM/CK
        /// </summary>
        public string HTTToan { get; set; } = string.Empty;

        /// <summary>
        /// Mã của cơ quan thuế cấp cho hóa đơn (&lt;MCCQT&gt;)
        /// </summary>
        public string MCCQT { get; set; } = string.Empty;
        #endregion

        #region Thông tin Bên bán (<NBan>)
        /// <summary>
        /// Tên công ty / người bán (&lt;NBan&gt; -> &lt;Ten&gt;)
        /// </summary>
        public string NBanTen { get; set; } = string.Empty;

        /// <summary>
        /// Mã số thuế người bán (&lt;NBan&gt; -> &lt;MST&gt;)
        /// </summary>
        public string NBanMST { get; set; } = string.Empty;

        /// <summary>
        /// Địa chỉ người bán (&lt;NBan&gt; -> &lt;DChi&gt;)
        /// </summary>
        public string NBanDChi { get; set; } = string.Empty;
        #endregion

        #region Thông tin Bên mua (<NMua>)
        /// <summary>
        /// Tên công ty / người mua (&lt;NMua&gt; -> &lt;Ten&gt;)
        /// </summary>
        public string NMuaTen { get; set; } = string.Empty;

        /// <summary>
        /// Mã số thuế người mua (&lt;NMua&gt; -> &lt;MST&gt;)
        /// </summary>
        public string NMuaMST { get; set; } = string.Empty;

        /// <summary>
        /// Địa chỉ người mua (&lt;NMua&gt; -> &lt;DChi&gt;)
        /// </summary>
        public string NMuaDChi { get; set; } = string.Empty;
        #endregion

        #region Tổng tiền hóa đơn (<TToan>)
        /// <summary>
        /// Tổng tiền chưa thuế của hóa đơn (&lt;TgTCThue&gt;)
        /// </summary>
        public decimal? TgTCThue { get; set; }

        /// <summary>
        /// Tổng tiền thuế GTGT của hóa đơn (&lt;TgTThue&gt;)
        /// </summary>
        public decimal? TgTThue { get; set; }

        /// <summary>
        /// Tổng tiền thanh toán bằng số (&lt;TgTTTBSo&gt;)
        /// </summary>
        public decimal? TgTTTBSo { get; set; }

        /// <summary>
        /// Tổng tiền thanh toán bằng chữ (&lt;TgTTTBChu&gt;)
        /// </summary>
        public string TgTTTBChu { get; set; } = string.Empty;
        #endregion

        #region Chi tiết mặt hàng / dịch vụ
        /// <summary>
        /// Danh sách chi tiết từng dòng hàng hóa / dịch vụ (&lt;DSHHDVu&gt; -> &lt;HHDVu&gt;)
        /// </summary>
        public List<InvoiceXmlItemModel> HoaDons { get; set; } = new List<InvoiceXmlItemModel>();
        #endregion

        #region Metadata & Quản lý file
        /// <summary>
        /// Đường dẫn tuyệt đối đến file XML trên máy
        /// </summary>
        public string FilePath { get; set; } = string.Empty;

        /// <summary>
        /// Tên file XML
        /// </summary>
        public string FileName { get; set; } = string.Empty;

        /// <summary>
        /// Hóa đơn tóm tắt gốc trong RAM được ghép nối (nếu có)
        /// Đánh dấu JsonIgnore để tránh lỗi circular reference khi serialize gửi qua WebView2
        /// </summary>
        [JsonIgnore]
        public dynamic? MatchedExistingInvoice { get; set; }
        #endregion
    }

    /// <summary>
    /// Kết quả tổng hợp sau khi quét và bóc tách thư mục XML
    /// </summary>
    public class XmlParseResult
    {
        public bool Success { get; set; }
        public string Message { get; set; } = string.Empty;
        public int TotalFilesFound { get; set; }
        public int TotalFilesParsed { get; set; }
        public int TotalFilesFailed { get; set; }
        public List<InvoiceXmlModel> Invoices { get; set; } = new List<InvoiceXmlModel>();
        public List<string> Errors { get; set; } = new List<string>();
    }
}
