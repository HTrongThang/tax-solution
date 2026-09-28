using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using ClosedXML.Excel;
using Newtonsoft.Json.Linq;

namespace Frontend.Services
{
    /// <summary>
    /// Service xuất báo cáo hóa đơn ra file Excel (.xlsx)
    /// Sử dụng thư viện ClosedXML để tạo file với đầy đủ style, font, border, màu.
    /// </summary>
    public class ExportExcelService
    {
        // ───────────────────────────────────────
        // BẢNG DỊCH MÃ TRẠNG THÁI HÓA ĐƠN
        // ───────────────────────────────────────
        private static readonly Dictionary<string, string> StatusMap = new()
        {
            { "1", "Hoá đơn mới" },
            { "2", "Hoá đơn thay thế" },
            { "3", "Hoá đơn điều chỉnh" },
            { "4", "Hoá đơn đã bị thay thế" },
            { "5", "Hoá đơn đã bị điều chỉnh" },
            { "6", "Hoá đơn đã huỷ" }
        };

        private static readonly Dictionary<string, string> ResultMap = new()
        {
            { "5", "Đã cấp mã hoá đơn" },
            { "6", "Tổng cục thuế đã nhận không mã" },
            { "8", "Tổng cục thuế đã nhận hoá đơn có mã khởi tạo từ máy tính tiền" }
        };

        // ───────────────────────────────────────
        // HÀM CHÍNH: XUẤT BÁO CÁO TỔNG HỢP
        // ───────────────────────────────────────

        /// <summary>
        /// Xuất file Excel "Báo cáo tổng hợp hóa đơn" với 17 cột dữ liệu.
        /// </summary>
        /// <param name="invoicesJson">Chuỗi JSON danh sách hóa đơn từ RAM Frontend</param>
        /// <param name="outputDir">Thư mục lưu file</param>
        /// <param name="isMuaVao">true = Mua vào, false = Bán ra</param>
        /// <param name="fromDate">Ngày bắt đầu (dd/MM/yyyy)</param>
        /// <param name="toDate">Ngày kết thúc (dd/MM/yyyy)</param>
        /// <param name="mst">Mã số thuế công ty</param>
        /// <returns>Đường dẫn file đã lưu</returns>
        public string ExportBaoCaoTongHop(
            string invoicesJson,
            string outputDir,
            bool isMuaVao,
            string fromDate,
            string toDate,
            string mst)
        {
            // 1. Parse dữ liệu
            var invoices = JArray.Parse(invoicesJson);
            if (invoices == null || invoices.Count == 0)
                throw new InvalidOperationException("Không có dữ liệu hóa đơn để xuất!");

            // 2. Tạo Workbook
            using var workbook = new XLWorkbook();
            var ws = workbook.Worksheets.Add("BaoCaoTongHop");

            // ═══════════════════════════════════════
            // DÒNG 1: TIÊU ĐỀ CHÍNH
            // ═══════════════════════════════════════
            string direction = isMuaVao ? "MUA VÀO" : "BÁN RA";
            ws.Range("A1:Q1").Merge();
            ws.Cell("A1").Value = $"DANH SÁCH HÓA ĐƠN {direction}";
            ws.Cell("A1").Style
                .Font.SetBold(true)
                .Font.SetFontSize(13)
                .Font.SetFontName("Times New Roman")
                .Alignment.SetHorizontal(XLAlignmentHorizontalValues.Center)
                .Alignment.SetVertical(XLAlignmentVerticalValues.Center);
            ws.Row(1).Height = 30;

            // ═══════════════════════════════════════
            // DÒNG 2: KHOẢNG THỜI GIAN
            // ═══════════════════════════════════════
            ws.Range("A2:Q2").Merge();
            ws.Cell("A2").Value = $"Từ ngày {fromDate} đến ngày {toDate}";
            ws.Cell("A2").Style
                .Font.SetBold(true)
                .Font.SetFontSize(10)
                .Font.SetFontName("Times New Roman")
                .Alignment.SetHorizontal(XLAlignmentHorizontalValues.Center);

            // ═══════════════════════════════════════
            // DÒNG 3: DÒNG TRỐNG PHÂN CÁCH
            // ═══════════════════════════════════════
            ws.Row(3).Height = 8;

            // ═══════════════════════════════════════
            // DÒNG 4: HEADER BẢNG (17 CỘT)
            // ═══════════════════════════════════════
            string[] headers = new[]
            {
                "STT",                                                              // A
                "Ký hiệu mẫu số",                                                  // B
                "Ký hiệu hóa đơn",                                                // C
                "Số hóa đơn",                                                       // D
                "Ngày lập",                                                         // E
                isMuaVao ? "MST người bán" : "MST người mua",                     // F
                isMuaVao ? "Tên người bán" : "Tên người mua",                     // G
                isMuaVao ? "Địa chỉ người bán" : "Địa chỉ người mua",            // H
                "Tổng tiền chưa thuế",                                             // I
                "Tổng tiền thuế",                                                   // J
                "Tổng tiền chiết khấu thương mại",                                 // K
                "Tổng tiền phí",                                                    // L
                "Tổng tiền thanh toán",                                            // M
                "Đơn vị tiền tệ",                                                  // N
                "Tỷ giá",                                                           // O
                "Trạng thái hóa đơn",                                              // P
                "Kết quả kiểm tra hóa đơn"                                         // Q
            };

            for (int col = 0; col < headers.Length; col++)
            {
                var cell = ws.Cell(4, col + 1);
                cell.Value = headers[col];
                cell.Style
                    .Font.SetBold(true)
                    .Font.SetFontSize(9)
                    .Font.SetFontName("Times New Roman")
                    .Font.SetFontColor(XLColor.Black)
                    .Fill.SetBackgroundColor(XLColor.FromHtml("#FFC000"))
                    .Alignment.SetHorizontal(XLAlignmentHorizontalValues.Center)
                    .Alignment.SetVertical(XLAlignmentVerticalValues.Center)
                    .Alignment.SetWrapText(true)
                    .Border.SetOutsideBorder(XLBorderStyleValues.Thin)
                    .Border.SetOutsideBorderColor(XLColor.Black);
            }
            ws.Row(4).Height = 32;

            // Bật AutoFilter cho header
            ws.Range(4, 1, 4, 17).SetAutoFilter();

            // ═══════════════════════════════════════
            // DÒNG 5+: DỮ LIỆU HÓA ĐƠN
            // ═══════════════════════════════════════
            int dataStartRow = 5;
            for (int i = 0; i < invoices.Count; i++)
            {
                var inv = invoices[i] as JObject;
                if (inv == null) continue;

                int row = dataStartRow + i;

                // Ánh xạ đối tác theo chiều hóa đơn
                string mstDoiTac, tenDoiTac, diaChiDoiTac;
                if (isMuaVao)
                {
                    mstDoiTac = GetStr(inv, "mst_nguoi_ban", "nbmst");
                    tenDoiTac = GetStr(inv, "ten_nguoi_ban", "nbten");
                    diaChiDoiTac = GetStr(inv, "dia_chi_nguoi_ban", "nbdchi");
                }
                else
                {
                    mstDoiTac = GetStr(inv, "mst_nguoi_mua", "nmmst", "mst_nguoi_ban");
                    tenDoiTac = GetStr(inv, "ten_nguoi_mua", "nmten");
                    diaChiDoiTac = GetStr(inv, "dia_chi_nguoi_mua", "nmdchi");
                }

                // Nếu thiếu tên/địa chỉ → tách từ thong_tin_hoa_don (chuỗi raw từ crawler)
                // VD: "MST người mua: 0303907936Tên người mua: CÔNG TY...Địa chỉ người mua: ..."
                if (string.IsNullOrEmpty(tenDoiTac) || string.IsNullOrEmpty(diaChiDoiTac))
                {
                    string rawInfo = GetStr(inv, "thong_tin_hoa_don");
                    if (!string.IsNullOrEmpty(rawInfo))
                    {
                        var parsed = ParseThongTinHoaDon(rawInfo, isMuaVao);
                        if (string.IsNullOrEmpty(mstDoiTac) && !string.IsNullOrEmpty(parsed.Mst))
                            mstDoiTac = parsed.Mst;
                        if (string.IsNullOrEmpty(tenDoiTac) && !string.IsNullOrEmpty(parsed.Ten))
                            tenDoiTac = parsed.Ten;
                        if (string.IsNullOrEmpty(diaChiDoiTac) && !string.IsNullOrEmpty(parsed.DiaChi))
                            diaChiDoiTac = parsed.DiaChi;
                    }
                }

                // Parse số tiền
                double tienChuaThue = ParseNumber(inv, "tong_chua_thue");
                double tienThue = ParseNumber(inv, "tong_tien_thue");
                double chietKhau = ParseNumber(inv, "chiet_khau");
                double phi = ParseNumber(inv, "tong_phi");
                double tongThanhToan = ParseNumber(inv, "tong_thanh_toan");

                // Gán dữ liệu vào từng ô
                ws.Cell(row, 1).Value = i + 1;                                                          // A: STT
                ws.Cell(row, 2).Value = GetStr(inv, "ky_hieu_mau_so", "khmshdon");                     // B
                ws.Cell(row, 3).Value = GetStr(inv, "ky_hieu", "khhdon");                              // C
                ws.Cell(row, 4).Value = GetStr(inv, "so_hoa_don", "shdon");                            // D
                ws.Cell(row, 5).Value = GetStr(inv, "ngay_lap", "tdlap");                              // E
                ws.Cell(row, 6).SetValue(mstDoiTac);                                                    // F: MST (dạng text)
                ws.Cell(row, 7).Value = tenDoiTac;                                                      // G
                ws.Cell(row, 8).Value = diaChiDoiTac;                                                   // H
                ws.Cell(row, 9).Value = tienChuaThue;                                                   // I
                ws.Cell(row, 10).Value = tienThue;                                                      // J
                ws.Cell(row, 11).Value = chietKhau;                                                     // K
                ws.Cell(row, 12).Value = phi;                                                           // L
                ws.Cell(row, 13).Value = tongThanhToan;                                                 // M
                ws.Cell(row, 14).Value = GetStr(inv, "don_vi_tien_te", "dvtte") is var dvtte && !string.IsNullOrEmpty(dvtte) ? dvtte : "VND"; // N
                ws.Cell(row, 15).Value = GetStr(inv, "ty_gia", "tgia") is var tgia && !string.IsNullOrEmpty(tgia) ? tgia : "1"; // O
                ws.Cell(row, 16).Value = ResolveStatus(GetStr(inv, "trang_thai", "tthai"));            // P
                ws.Cell(row, 17).Value = ResolveResult(GetStr(inv, "ket_qua_xu_ly", "ket_qua_kiem_tra", "tthaixly"));   // Q

                // ── STYLE CHO DÒNG DỮ LIỆU ──
                var dataRange = ws.Range(row, 1, row, 17);
                dataRange.Style
                    .Font.SetFontSize(9)
                    .Font.SetFontName("Times New Roman")
                    .Border.SetOutsideBorder(XLBorderStyleValues.Thin)
                    .Border.SetOutsideBorderColor(XLColor.FromHtml("#D0D0D0"))
                    .Border.SetInsideBorder(XLBorderStyleValues.Hair)
                    .Border.SetInsideBorderColor(XLColor.FromHtml("#D0D0D0"));

                // Format số cho cột tiền (I, J, K, L, M)
                for (int c = 9; c <= 13; c++)
                {
                    ws.Cell(row, c).Style.NumberFormat.SetFormat("#,##0");
                }

                // Tô dòng xen kẽ để dễ đọc (zebra striping)
                if (i % 2 == 1)
                {
                    dataRange.Style.Fill.SetBackgroundColor(XLColor.FromHtml("#F8FAFC"));
                }
            }

            // ═══════════════════════════════════════
            // DÒNG TỔNG CỘNG
            // ═══════════════════════════════════════
            int lastDataRow = dataStartRow + invoices.Count - 1;
            int summaryRow = lastDataRow + 1;

            ws.Cell(summaryRow, 2).Value = "Tổng cộng :";
            ws.Cell(summaryRow, 2).Style.Font.SetBold(true).Font.SetFontSize(10).Font.SetFontName("Times New Roman");

            // Cột H (8): Đếm số lượng hóa đơn
            ws.Cell(summaryRow, 8).FormulaA1 = $"COUNTA(A{dataStartRow}:A{lastDataRow})";
            ws.Cell(summaryRow, 8).Style.Font.SetBold(true).NumberFormat.SetFormat("#,##0");

            // Cột I-M (9-13): SUM
            for (int c = 9; c <= 13; c++)
            {
                string colLetter = GetColumnLetter(c);
                ws.Cell(summaryRow, c).FormulaA1 = $"SUM({colLetter}{dataStartRow}:{colLetter}{lastDataRow})";
                ws.Cell(summaryRow, c).Style
                    .Font.SetBold(true)
                    .Font.SetFontSize(10)
                    .Font.SetFontName("Times New Roman")
                    .NumberFormat.SetFormat("#,##0");
            }

            // Style dòng tổng cộng
            var summaryRange = ws.Range(summaryRow, 1, summaryRow, 17);
            summaryRange.Style
                .Font.SetBold(true)
                .Font.SetFontSize(10)
                .Font.SetFontName("Times New Roman")
                .Border.SetTopBorder(XLBorderStyleValues.Double)
                .Border.SetTopBorderColor(XLColor.Black)
                .Border.SetBottomBorder(XLBorderStyleValues.Double)
                .Border.SetBottomBorderColor(XLColor.Black)
                .Fill.SetBackgroundColor(XLColor.FromHtml("#FFF8E1"));

            // ═══════════════════════════════════════
            // ĐỘ RỘNG CỘT (Auto-fit thủ công)
            // ═══════════════════════════════════════
            int[] colWidths = { 6, 14, 14, 12, 12, 16, 30, 40, 18, 16, 18, 14, 18, 12, 10, 22, 30 };
            for (int c = 0; c < colWidths.Length; c++)
            {
                ws.Column(c + 1).Width = colWidths[c];
            }

            // ═══════════════════════════════════════
            // FREEZE PANE: Đóng băng header
            // ═══════════════════════════════════════
            ws.SheetView.FreezeRows(4);

            // ═══════════════════════════════════════
            // TẠO TÊN FILE VÀ LƯU
            // ═══════════════════════════════════════
            string fromClean = fromDate?.Replace("/", "") ?? "";
            string toClean = toDate?.Replace("/", "") ?? "";
            string dirLabel = isMuaVao ? "MuaVao" : "BanRa";
            string fileName = $"BaoCaoTongHop_{dirLabel}_{mst}_{fromClean}_{toClean}.xlsx";

            // Xử lý ký tự không hợp lệ trong tên file
            foreach (char c in Path.GetInvalidFileNameChars())
            {
                fileName = fileName.Replace(c, '_');
            }

            string fullPath = Path.Combine(outputDir, fileName);

            // Nếu file đang mở bởi Excel, thêm timestamp để tránh lỗi
            if (File.Exists(fullPath))
            {
                try { File.Delete(fullPath); }
                catch
                {
                    string timestamp = DateTime.Now.ToString("HHmmss");
                    fileName = $"BaoCaoTongHop_{dirLabel}_{mst}_{fromClean}_{toClean}_{timestamp}.xlsx";
                    fullPath = Path.Combine(outputDir, fileName);
                }
            }

            workbook.SaveAs(fullPath);

            Debug.WriteLine($"[ExportExcel]  Đã xuất {invoices.Count} hóa đơn → {fullPath}");
            return fullPath;
        }

        // ───────────────────────────────────────
        // HÀM TIỆN ÍCH
        // ───────────────────────────────────────

        /// <summary>
        /// Lấy giá trị string từ JObject, thử nhiều tên field theo thứ tự ưu tiên
        /// </summary>
        private static string GetStr(JObject obj, params string[] fieldNames)
        {
            foreach (var name in fieldNames)
            {
                var val = obj[name];
                if (val != null && val.Type != JTokenType.Null)
                {
                    string s = val.ToString().Trim();
                    if (!string.IsNullOrEmpty(s) && s != "---") return s;
                }
            }
            return "";
        }

        /// <summary>
        /// Parse chuỗi số tiền Việt Nam (có dấu chấm phân cách nghìn) thành double
        /// </summary>
        private static double ParseNumber(JObject obj, string fieldName)
        {
            var val = obj[fieldName];
            if (val == null || val.Type == JTokenType.Null) return 0;

            string s = val.ToString().Trim();
            if (string.IsNullOrEmpty(s) || s == "---") return 0;

            // Xóa dấu chấm phân cách nghìn, đổi dấu phẩy thành chấm thập phân
            s = s.Replace(".", "").Replace(",", ".").Trim();
            // Xóa ký tự không phải số
            s = System.Text.RegularExpressions.Regex.Replace(s, @"[^\d.\-]", "");

            if (double.TryParse(s, System.Globalization.NumberStyles.Any,
                System.Globalization.CultureInfo.InvariantCulture, out double result))
            {
                return result;
            }
            return 0;
        }

        /// <summary>
        /// Dịch mã trạng thái hóa đơn (1-6)
        /// </summary>
        private static string ResolveStatus(string code)
        {
            if (string.IsNullOrEmpty(code)) return "";
            code = code.Trim();
            return StatusMap.TryGetValue(code, out var text) ? text : code;
        }

        /// <summary>
        /// Dịch mã kết quả kiểm tra CQT (5, 6, 8)
        /// </summary>
        private static string ResolveResult(string code)
        {
            if (string.IsNullOrEmpty(code)) return "";
            code = code.Trim();
            // Bỏ qua nếu giá trị là mã tiền tệ bị lệch cột từ crawler
            if (code == "VND" || code == "USD" || code == "EUR" || code == "JPY") return "";
            return ResultMap.TryGetValue(code, out var text) ? text : code;
        }

        /// <summary>
        /// Chuyển số thứ tự cột (1-based) sang ký tự Excel (A, B, ... Q)
        /// </summary>
        private static string GetColumnLetter(int colNumber)
        {
            string result = "";
            while (colNumber > 0)
            {
                colNumber--;
                result = (char)('A' + colNumber % 26) + result;
                colNumber /= 26;
            }
            return result;
        }
        /// <summary>
        /// Tách chuỗi thong_tin_hoa_don raw từ crawler thành MST, Tên, Địa chỉ riêng biệt.
        /// VD input: "MST người mua: 0303907936Tên người mua: CÔNG TY ABC Địa chỉ người mua: 123 Đường XYZ"
        /// </summary>
        private static ParsedInfo ParseThongTinHoaDon(string raw, bool isMuaVao)
        {
            var result = new ParsedInfo();
            if (string.IsNullOrEmpty(raw)) return result;

            string doiTuong = isMuaVao ? "bán" : "mua";

            // Regex tách MST
            var mstMatch = System.Text.RegularExpressions.Regex.Match(
                raw, $@"MST\s*người\s*{doiTuong}\s*:\s*(\d{{10,14}})",
                System.Text.RegularExpressions.RegexOptions.IgnoreCase);
            if (mstMatch.Success)
                result.Mst = mstMatch.Groups[1].Value.Trim();

            // Regex tách Tên
            var tenMatch = System.Text.RegularExpressions.Regex.Match(
                raw, $@"Tên\s*người\s*{doiTuong}\s*:\s*(.+?)(?=Địa\s*chỉ|$)",
                System.Text.RegularExpressions.RegexOptions.IgnoreCase);
            if (tenMatch.Success)
                result.Ten = tenMatch.Groups[1].Value.Trim();

            // Regex tách Địa chỉ
            var dcMatch = System.Text.RegularExpressions.Regex.Match(
                raw, $@"Địa\s*chỉ\s*người\s*{doiTuong}\s*:\s*(.+?)$",
                System.Text.RegularExpressions.RegexOptions.IgnoreCase);
            if (dcMatch.Success)
                result.DiaChi = dcMatch.Groups[1].Value.Trim();

            return result;
        }

        private class ParsedInfo
        {
            public string Mst { get; set; } = "";
            public string Ten { get; set; } = "";
            public string DiaChi { get; set; } = "";
        }
    }
}
