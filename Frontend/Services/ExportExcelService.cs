using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using ClosedXML.Excel;
using Frontend.Models;
using Newtonsoft.Json.Linq;

namespace Frontend.Services
{
    /// <summary>
    /// Service xuất báo cáo hóa đơn ra file Excel (.xlsx)
    /// Sử dụng thư viện ClosedXML để tạo file với đầy đủ style, font, border, màu.
    /// </summary>
    public class ExportExcelService
    {
        // ─────────────────────────────────────────────────────────────
        // BẢNG DỊCH MÃ TRẠNG THÁI HÓA ĐƠN
        // ─────────────────────────────────────────────────────────────
        private static readonly Dictionary<string, string> StatusMap = new()
        {
            { "1", "Hóa đơn mới" },
            { "2", "Hóa đơn thay thế" },
            { "3", "Hóa đơn điều chỉnh" },
            { "4", "Hóa đơn đã bị thay thế" },
            { "5", "Hóa đơn đã bị điều chỉnh" },
            { "6", "Hóa đơn đã hủy" }
        };

        private static readonly Dictionary<string, string> ResultMap = new()
        {
            { "5", "Đã cấp mã hóa đơn" },
            { "6", "Tổng cục thuế đã nhận không mã" },
            { "8", "Tổng cục thuế đã nhận hóa đơn có mã khởi tạo từ máy tính tiền" }
        };

        // ─────────────────────────────────────────────────────────────
        // 1. HÀM CHÍNH: XUẤT BÁO CÁO TỔNG HỢP (17 CỘT)
        // ─────────────────────────────────────────────────────────────

        /// <summary>
        /// Xuất file Excel "Báo cáo tổng hợp hóa đơn" với 17 cột dữ liệu.
        /// </summary>
        public string ExportBaoCaoTongHop(
            string invoicesJson,
            string targetPathOrDir,
            bool isMuaVao,
            string fromDate,
            string toDate,
            string mst)
        {
            var invoices = JArray.Parse(invoicesJson);
            if (invoices == null || invoices.Count == 0)
                throw new InvalidOperationException("Không có dữ liệu hóa đơn để xuất!");

            using var workbook = new XLWorkbook();
            var ws = workbook.Worksheets.Add("BaoCaoTongHop");

            // DÒNG 1: TIÊU ĐỀ CHÍNH
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

            // DÒNG 2: KHOẢNG THỜI GIAN
            ws.Range("A2:Q2").Merge();
            ws.Cell("A2").Value = $"Từ ngày {fromDate} đến ngày {toDate}";
            ws.Cell("A2").Style
                .Font.SetBold(true)
                .Font.SetFontSize(10)
                .Font.SetFontName("Times New Roman")
                .Alignment.SetHorizontal(XLAlignmentHorizontalValues.Center);

            // DÒNG 3: DÒNG TRẮNG PHÂN CÁCH
            ws.Row(3).Height = 8;

            // DÒNG 4: HEADER BẢNG (17 CỘT)
            string[] headers = new[]
            {
                "STT",
                "Ký hiệu mẫu số",
                "Ký hiệu hóa đơn",
                "Số hóa đơn",
                "Ngày lập",
                isMuaVao ? "MST người bán" : "MST người mua",
                isMuaVao ? "Tên người bán" : "Tên người mua",
                isMuaVao ? "Địa chỉ người bán" : "Địa chỉ người mua",
                "Tổng tiền chưa thuế",
                "Tổng tiền thuế",
                "Tổng tiền chiết khấu thương mại",
                "Tổng tiền phí",
                "Tổng tiền thanh toán",
                "Đơn vị tiền tệ",
                "Tỷ giá",
                "Trạng thái hóa đơn",
                "Kết quả kiểm tra hóa đơn"
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
                    .Fill.SetBackgroundColor(XLColor.FromHtml("#E2EFDA"))
                    .Alignment.SetHorizontal(XLAlignmentHorizontalValues.Center)
                    .Alignment.SetVertical(XLAlignmentVerticalValues.Center)
                    .Alignment.SetWrapText(true);

                cell.Style.Border.OutsideBorder = XLBorderStyleValues.Thin;
                cell.Style.Border.OutsideBorderColor = XLColor.FromHtml("#B0C4DE");
            }
            ws.Row(4).Height = 32;

            // DÒNG 5 TRỞ ĐI: DỮ LIỆU
            int dataStartRow = 5;
            for (int i = 0; i < invoices.Count; i++)
            {
                int row = dataStartRow + i;
                var inv = invoices[i] as JObject;
                if (inv == null) continue;

                var rawInfo = ParseThongTinHoaDon(GetStr(inv, "thong_tin_hoa_don"), isMuaVao);

                string colMst = isMuaVao
                    ? GetStr(inv, "mst_nguoi_ban", "NBanMST", "nbmst", "mst", "MSTNguoiBan")
                    : GetStr(inv, "mst_nguoi_mua", "NMuaMST", "nmmst", "mst", "MSTNguoiMua");
                if (string.IsNullOrEmpty(colMst)) colMst = rawInfo.Mst;

                string colTen = isMuaVao
                    ? GetStr(inv, "ten_nguoi_ban", "NBanTen", "nbten", "ten", "TenNguoiBan")
                    : GetStr(inv, "ten_nguoi_mua", "NMuaTen", "nmten", "ten", "TenNguoiMua");
                if (string.IsNullOrEmpty(colTen)) colTen = rawInfo.Ten;

                string colDchi = isMuaVao
                    ? GetStr(inv, "dia_chi_nguoi_ban", "NBanDChi", "nbdchi", "DiaChiNguoiBan")
                    : GetStr(inv, "dia_chi_nguoi_mua", "NMuaDChi", "nmdchi", "DiaChiNguoiMua");
                if (string.IsNullOrEmpty(colDchi)) colDchi = rawInfo.DiaChi;

                string khmshdon = GetStr(inv, "ky_hieu_mau_so", "KHMSHDon", "khmshdon", "khms", "MauSo");
                string khhdon = GetStr(inv, "ky_hieu", "KHHDon", "khhdon", "kh", "KyHieu");
                string shdon = GetStr(inv, "so_hoa_don", "SHDon", "shdon", "so", "SoHoaDon", "soHD");

                if (string.IsNullOrEmpty(khmshdon) && !string.IsNullOrEmpty(khhdon) && khhdon.Length >= 6 && char.IsDigit(khhdon[0]))
                {
                    khmshdon = khhdon.Substring(0, 1);
                    khhdon = khhdon.Substring(1);
                }

                ws.Cell(row, 1).Value = i + 1;
                ws.Cell(row, 2).Value = khmshdon;
                ws.Cell(row, 3).Value = khhdon;
                ws.Cell(row, 4).Value = shdon;

                string ngayLapStr = GetStr(inv, "ngay_lap", "NLapRaw", "NLap", "tdlap", "ntdlap", "ngayLap", "NgayLap");
                DateTime? ngayLap = ParseDate(ngayLapStr);
                if (ngayLap.HasValue)
                {
                    ws.Cell(row, 5).Value = ngayLap.Value;
                    ws.Cell(row, 5).Style.DateFormat.Format = "dd/MM/yyyy";
                }
                else
                {
                    ws.Cell(row, 5).Value = ngayLapStr;
                }

                ws.Cell(row, 6).Value = colMst;
                ws.Cell(row, 7).Value = colTen;
                ws.Cell(row, 8).Value = colDchi;

                double tt = ParseNumber(inv, "tong_chua_thue", "TgTCThue", "tgtcthue", "tong_tien_chua_thue", "TongChuaThue");
                double thue = ParseNumber(inv, "tong_tien_thue", "TgTThue", "tgtthue", "tien_thue", "TongTienThue");
                double cktm = ParseNumber(inv, "chiet_khau", "ttchkhau", "tong_tien_cktm", "ChietKhau");
                double phi = ParseNumber(inv, "tong_phi", "ttphi", "tong_tien_phi", "TongPhi");
                double tong = ParseNumber(inv, "tong_thanh_toan", "TgTTTBSo", "tgtttbso", "tong_tien_thanh_toan", "TongThanhToan");
                if (tong == 0 && (tt > 0 || thue > 0)) tong = tt + thue - cktm + phi;
                if (tt == 0 && tong > 0 && thue > 0) tt = tong - thue + cktm - phi;

                ws.Cell(row, 9).Value = tt;
                ws.Cell(row, 10).Value = thue;
                ws.Cell(row, 11).Value = cktm;
                ws.Cell(row, 12).Value = phi;
                ws.Cell(row, 13).Value = tong;

                ws.Cell(row, 14).Value = GetStr(inv, "don_vi_tien_te", "dvtte", "dvt", "DVTTe") switch { "" => "VND", var s => s };
                ws.Cell(row, 15).Value = ParseNumber(inv, "ty_gia", "tgia") switch { 0 => 1, var v => v };

                string rawStatus = GetStr(inv, "trang_thai", "tthai", "TrangThai");
                string rawResult = GetStr(inv, "ket_qua_kiem_tra", "ket_qua_xu_ly", "kqktngay", "ket_qua");
                ws.Cell(row, 16).Value = ResolveStatus(rawStatus);
                ws.Cell(row, 17).Value = ResolveResult(rawResult);

                ws.Row(row).Height = 20;

                for (int c = 1; c <= 17; c++)
                {
                    var cell = ws.Cell(row, c);
                    cell.Style.Font.SetFontName("Times New Roman").Font.SetFontSize(9);

                    cell.Style.Border.OutsideBorder = XLBorderStyleValues.Thin;
                    cell.Style.Border.OutsideBorderColor = XLColor.FromHtml("#E0E0E0");

                    cell.Style.Alignment.SetVertical(XLAlignmentVerticalValues.Center);

                    if (c == 1)
                    {
                        cell.Style.NumberFormat.SetFormat("#,##0");
                    }
                    else if (c >= 9 && c <= 13)
                    {
                        cell.Style.NumberFormat.SetFormat("#,##0");
                    }
                    else if (c == 15)
                    {
                        cell.Style.NumberFormat.SetFormat("#,##0.00");
                    }
                }
            }

            // DÒNG TỔNG CỘNG
            int lastDataRow = dataStartRow + invoices.Count - 1;
            int summaryRow = lastDataRow + 1;

            // Gom cột 1 -> 8 hiển thị "TỔNG CỘNG :"
            ws.Range(summaryRow, 1, summaryRow, 8).Merge();
            ws.Cell(summaryRow, 1).Value = "TỔNG CỘNG :";
            ws.Cell(summaryRow, 1).Style
                .Font.SetBold(true)
                .Font.SetFontSize(10)
                .Font.SetFontName("Times New Roman")
                .Alignment.SetHorizontal(XLAlignmentHorizontalValues.Right)
                .Alignment.SetVertical(XLAlignmentVerticalValues.Center);

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

            int[] colWidths = { 6, 14, 14, 12, 12, 16, 30, 40, 18, 16, 18, 14, 18, 12, 10, 22, 30 };
            for (int c = 0; c < colWidths.Length; c++)
            {
                ws.Column(c + 1).Width = colWidths[c];
            }

            ws.SheetView.FreezeRows(4);

            string finalSavePath;
            if (targetPathOrDir.EndsWith(".xlsx", StringComparison.OrdinalIgnoreCase))
            {
                finalSavePath = targetPathOrDir;
            }
            else
            {
                string fromClean = fromDate?.Replace("/", "") ?? "";
                string toClean = toDate?.Replace("/", "") ?? "";
                string dirLabel = isMuaVao ? "MuaVao" : "BanRa";
                string rand = $"{DateTime.Now:HHmmss}_{Random.Shared.Next(100, 999)}";
                string fileName = $"BaoCaoTongHop_{dirLabel}_{mst}_{fromClean}_{toClean}_{rand}.xlsx";

                foreach (char c in Path.GetInvalidFileNameChars())
                {
                    fileName = fileName.Replace(c, '_');
                }
                finalSavePath = Path.Combine(targetPathOrDir, fileName);
            }

            if (File.Exists(finalSavePath))
            {
                try { File.Delete(finalSavePath); }
                catch
                {
                    string dir = Path.GetDirectoryName(finalSavePath) ?? "";
                    string baseName = Path.GetFileNameWithoutExtension(finalSavePath);
                    string rand = $"{DateTime.Now:HHmmss}_{Random.Shared.Next(100, 999)}";
                    finalSavePath = Path.Combine(dir, $"{baseName}_{rand}.xlsx");
                }
            }

            workbook.SaveAs(finalSavePath);
            Debug.WriteLine($"[ExportExcel] Đã xuất {invoices.Count} hóa đơn -> {finalSavePath}");
            return finalSavePath;
        }

        // ─────────────────────────────────────────────────────────────
        // 2. HÀM CHÍNH: XUẤT BẢNG KÊ CHI TIẾT HÀNG HÓA (16 CỘT)
        // ─────────────────────────────────────────────────────────────

        /// <summary>
        /// Xuất file Excel "Bảng kê chi tiết hóa đơn chứng từ hàng hóa, dịch vụ" theo từng dòng hàng hóa.
        /// Chuẩn 16 cột đối chiếu nghiệp vụ kế toán thuế.
        /// </summary>
        public string ExportBangKeChiTiet(
            string invoicesJson,
            string targetPathOrDir,
            bool isMuaVao,
            string fromDate,
            string toDate,
            string mst)
        {
            var invoices = JArray.Parse(invoicesJson);
            if (invoices == null || invoices.Count == 0)
                throw new InvalidOperationException("Không có dữ liệu hóa đơn để xuất!");

            // 1. Lấy danh sách XML đã bóc tách trong bộ nhớ RAM (nếu có) để lookup dòng hàng
            var memoryXmls = InvoiceMemoryService.Instance.GetParsedXmlInvoices();
            if ((memoryXmls == null || memoryXmls.Count == 0) && XmlInvoiceParserService.LastParsedInvoices != null && XmlInvoiceParserService.LastParsedInvoices.Count > 0)
            {
                memoryXmls = XmlInvoiceParserService.LastParsedInvoices;
            }

            var xmlLookup = new Dictionary<string, InvoiceXmlModel>(StringComparer.OrdinalIgnoreCase);
            if (memoryXmls != null)
            {
                foreach (var x in memoryXmls)
                {
                    void AddLookupKey(string? k)
                    {
                        if (!string.IsNullOrWhiteSpace(k) && !xmlLookup.ContainsKey(k))
                        {
                            xmlLookup[k] = x;
                        }
                    }

                    AddLookupKey(BuildMatchKey(x.KHHDon, x.SHDon));
                    AddLookupKey(BuildMatchKey(x.KHMSHDon + x.KHHDon, x.SHDon));
                    string strippedKHH = System.Text.RegularExpressions.Regex.Replace(x.KHHDon ?? "", @"^[1-6]", "");
                    AddLookupKey(BuildMatchKey(strippedKHH, x.SHDon));
                    string numOnly = (x.SHDon ?? "").Trim().TrimStart('0');
                    if (!string.IsNullOrEmpty(numOnly))
                    {
                        AddLookupKey("so_" + numOnly);
                    }
                }
            }

            // 2. Gom phẳng toàn bộ dòng hàng hóa (Flatten Line Items)
            var flatItems = new List<FlatLineItem>();

            foreach (var token in invoices)
            {
                var invObj = token as JObject;
                if (invObj == null) continue;

                string khmshdon = GetStr(invObj, "ky_hieu_mau_so", "KHMSHDon", "khmshdon", "khms", "MauSo");
                string khhdon = GetStr(invObj, "ky_hieu", "KHHDon", "khhdon", "kh", "KyHieu");
                string shdon = GetStr(invObj, "so_hoa_don", "SHDon", "shdon", "so", "SoHoaDon", "soHD");
                string nlapRaw = GetStr(invObj, "ngay_lap", "NLapRaw", "tdlap", "ntdlap", "ngayLap", "NLap", "NgayLap");
                DateTime? ngayLap = ParseDate(nlapRaw);

                // Nếu ký hiệu có số mẫu đầu (vd: 1C26TBB) mà chưa tách mẫu số
                if (string.IsNullOrEmpty(khmshdon) && !string.IsNullOrEmpty(khhdon) && khhdon.Length >= 6 && char.IsDigit(khhdon[0]))
                {
                    khmshdon = khhdon.Substring(0, 1);
                    khhdon = khhdon.Substring(1);
                }

                var rawInfo = ParseThongTinHoaDon(GetStr(invObj, "thong_tin_hoa_don"), isMuaVao);

                string colMst = isMuaVao
                    ? GetStr(invObj, "mst_nguoi_ban", "NBanMST", "nbmst", "mst", "MSTNguoiBan")
                    : GetStr(invObj, "mst_nguoi_mua", "NMuaMST", "nmmst", "mst", "MSTNguoiMua");
                if (string.IsNullOrEmpty(colMst)) colMst = rawInfo.Mst;

                string colTen = isMuaVao
                    ? GetStr(invObj, "ten_nguoi_ban", "NBanTen", "nbten", "tenNguoiBan", "ten", "TenNguoiBan")
                    : GetStr(invObj, "ten_nguoi_mua", "NMuaTen", "nmten", "tenNguoiMua", "ten", "TenNguoiMua");
                if (string.IsNullOrEmpty(colTen)) colTen = rawInfo.Ten;

                // Kiểm tra xem hóa đơn này có danh sách dòng hàng không
                JArray? itemsArray = invObj["HoaDons"] as JArray 
                                  ?? invObj["hoaDons"] as JArray 
                                  ?? invObj["items"] as JArray;

                // Nếu trong JSON gửi xuống chưa có, thử tìm trong memory lookup XML
                InvoiceXmlModel? matchedXml = null;
                List<InvoiceXmlItemModel>? memoryItems = null;

                if (itemsArray == null || itemsArray.Count == 0)
                {
                    string matchKey1 = BuildMatchKey(khhdon, shdon);
                    string matchKey2 = BuildMatchKey(khmshdon + khhdon, shdon);
                    string strippedKhh = System.Text.RegularExpressions.Regex.Replace(khhdon ?? "", @"^[1-6]", "");
                    string matchKey3 = BuildMatchKey(strippedKhh, shdon);
                    string numKey = "so_" + (shdon ?? "").Trim().TrimStart('0');

                    if (!string.IsNullOrEmpty(matchKey1) && xmlLookup.TryGetValue(matchKey1, out matchedXml)) { }
                    else if (!string.IsNullOrEmpty(matchKey2) && xmlLookup.TryGetValue(matchKey2, out matchedXml)) { }
                    else if (!string.IsNullOrEmpty(matchKey3) && xmlLookup.TryGetValue(matchKey3, out matchedXml)) { }
                    else if (!string.IsNullOrEmpty(shdon) && xmlLookup.TryGetValue(numKey, out matchedXml)) { }

                    if (matchedXml != null)
                    {
                        memoryItems = matchedXml.HoaDons;
                        if (string.IsNullOrEmpty(khmshdon)) khmshdon = matchedXml.KHMSHDon ?? "";
                        if (string.IsNullOrEmpty(khhdon)) khhdon = matchedXml.KHHDon ?? "";
                        if (string.IsNullOrEmpty(shdon)) shdon = matchedXml.SHDon ?? "";
                        if (!ngayLap.HasValue) ngayLap = matchedXml.NLap;
                        if (string.IsNullOrEmpty(colMst)) colMst = (isMuaVao ? matchedXml.NBanMST : matchedXml.NMuaMST) ?? "";
                        if (string.IsNullOrEmpty(colTen)) colTen = (isMuaVao ? matchedXml.NBanTen : matchedXml.NMuaTen) ?? "";
                    }
                }

                if (itemsArray != null && itemsArray.Count > 0)
                {
                    foreach (var itemToken in itemsArray)
                    {
                        var itemObj = itemToken as JObject;
                        if (itemObj == null) continue;

                        double sl = ParseNumber(itemObj, "SLuong", "sluong", "SoLuong");
                        double dg = ParseNumber(itemObj, "DGia", "dgia", "DonGia");
                        double tt = ParseNumber(itemObj, "ThTien", "thtien");
                        double thue = ParseNumber(itemObj, "TThue", "tthue");

                        string tchat = GetStr(itemObj, "TinhChatMoTa", "tchat", "TChat");
                        if (tchat == "3") tchat = "Chiết khấu thương mại";
                        else if (string.IsNullOrEmpty(tchat) || tchat == "1") tchat = "Hàng hóa, dịch vụ";

                        flatItems.Add(new FlatLineItem
                        {
                            Khmshdon = khmshdon,
                            Khhdon = khhdon,
                            Shdon = shdon,
                            NgayLap = ngayLap,
                            TenDoiTac = colTen,
                            MstDoiTac = colMst,
                            TenHangHoa = GetStr(itemObj, "THHDVu", "thhdvu", "TenVT", "ten", "Ten"),
                            Dvtinh = GetStr(itemObj, "DVTinh", "dvtinh", "DVT", "dvt"),
                            SoLuong = sl,
                            DonGia = dg,
                            ThTien = tt,
                            TSuat = GetStr(itemObj, "TSuat", "tsuat", "thue_suat"),
                            TThue = thue,
                            TongTien = tt + thue,
                            TinhChat = tchat
                        });
                    }
                }
                else if (memoryItems != null && memoryItems.Count > 0)
                {
                    foreach (var mItem in memoryItems)
                    {
                        double sl = (double)(mItem.SLuong ?? 0);
                        double dg = (double)(mItem.DGia ?? 0);
                        double tt = (double)(mItem.ThTien ?? 0);
                        double thue = (double)(mItem.TThue ?? 0);

                        flatItems.Add(new FlatLineItem
                        {
                            Khmshdon = string.IsNullOrEmpty(khmshdon) ? matchedXml?.KHMSHDon ?? "" : khmshdon,
                            Khhdon = string.IsNullOrEmpty(khhdon) ? matchedXml?.KHHDon ?? "" : khhdon,
                            Shdon = string.IsNullOrEmpty(shdon) ? matchedXml?.SHDon ?? "" : shdon,
                            NgayLap = ngayLap ?? matchedXml?.NLap,
                            TenDoiTac = string.IsNullOrEmpty(colTen) ? (isMuaVao ? matchedXml?.NBanTen : matchedXml?.NMuaTen) ?? "" : colTen,
                            MstDoiTac = string.IsNullOrEmpty(colMst) ? (isMuaVao ? matchedXml?.NBanMST : matchedXml?.NMuaMST) ?? "" : colMst,
                            TenHangHoa = mItem.THHDVu,
                            Dvtinh = mItem.DVTinh,
                            SoLuong = sl,
                            DonGia = dg,
                            ThTien = tt,
                            TSuat = mItem.TSuat,
                            TThue = thue,
                            TongTien = tt + thue,
                            TinhChat = mItem.TinhChatMoTa
                        });
                    }
                }
                else
                {
                    // Fallback: Nếu chưa có chi tiết dòng hàng XML, xuất 1 dòng theo tổng số liệu hóa đơn
                    double tt = ParseNumber(invObj, "tong_chua_thue", "TgTCThue", "tgtcthue", "tong_tien_chua_thue");
                    double thue = ParseNumber(invObj, "tong_tien_thue", "TgTThue", "tgtthue", "tien_thue");
                    double tong = ParseNumber(invObj, "tong_thanh_toan", "TgTTTBSo", "tgtttbso", "tong_tien_thanh_toan");
                    if (tong == 0) tong = tt + thue;
                    if (tt == 0 && tong > 0 && thue > 0) tt = tong - thue; // Tự tính nếu bị khuyết

                    flatItems.Add(new FlatLineItem
                    {
                        Khmshdon = khmshdon,
                        Khhdon = khhdon,
                        Shdon = shdon,
                        NgayLap = ngayLap,
                        TenDoiTac = colTen,
                        MstDoiTac = colMst,
                        TenHangHoa = "(Chưa bóc tách XML - dữ liệu tổng quan)",
                        Dvtinh = "",
                        SoLuong = 0,
                        DonGia = 0,
                        ThTien = tt,
                        TSuat = GetStr(invObj, "tsuat", "TSuat"),
                        TThue = thue,
                        TongTien = tong,
                        TinhChat = "Hàng hóa, dịch vụ"
                    });
                }
            }

            if (flatItems.Count == 0)
                throw new InvalidOperationException("Không tìm thấy dòng hàng hóa nào để xuất!");

            // 3. Khởi tạo Workbook ClosedXML
            using var workbook = new XLWorkbook();
            string sheetName = isMuaVao ? "BangKeChiTietMuaVao" : "BangKeChiTietBanRa";
            var ws = workbook.Worksheets.Add(sheetName);

            // DÒNG 1: TIÊU ĐỀ BÁO CÁO
            string direction = isMuaVao ? "MUA VÀO" : "BÁN RA";
            ws.Range("A1:P1").Merge();
            ws.Cell("A1").Value = $"BẢNG KÊ CHI TIẾT HÓA ĐƠN ĐIỆN TỬ {direction}";
            ws.Cell("A1").Style
                .Font.SetBold(true)
                .Font.SetFontSize(14)
                .Font.SetFontName("Times New Roman")
                .Alignment.SetHorizontal(XLAlignmentHorizontalValues.Center)
                .Alignment.SetVertical(XLAlignmentVerticalValues.Center);
            ws.Row(1).Height = 32;

            // DÒNG 2: THỜI GIAN / KỲ KÊ KHAI
            ws.Range("A2:P2").Merge();
            ws.Cell("A2").Value = $"Kỳ kê khai / Thời gian tra cứu: Từ ngày {fromDate} đến ngày {toDate}";
            ws.Cell("A2").Style
                .Font.SetBold(true)
                .Font.SetItalic(true)
                .Font.SetFontSize(10)
                .Font.SetFontName("Times New Roman")
                .Alignment.SetHorizontal(XLAlignmentHorizontalValues.Center);

            // DÒNG 3: MÃ SỐ THUẾ DOANH NGHIỆP
            ws.Range("A3:P3").Merge();
            ws.Cell("A3").Value = $"Mã số thuế doanh nghiệp: {mst}";
            ws.Cell("A3").Style
                .Font.SetBold(true)
                .Font.SetFontSize(10)
                .Font.SetFontName("Times New Roman")
                .Alignment.SetHorizontal(XLAlignmentHorizontalValues.Center);

            // DÒNG 4: DÒNG TRẮNG PHÂN CÁCH
            ws.Row(4).Height = 8;

            // DÒNG 5: HEADERS (16 CỘT CHUẨN KẾ TOÁN)
            string[] headers = new[]
            {
                "STT",                                          // 1 - A
                "Ký hiệu mẫu số",                               // 2 - B
                "Ký hiệu hóa đơn",                             // 3 - C
                "Số hóa đơn",                                   // 4 - D
                "Ngày, tháng, năm lập",                         // 5 - E
                isMuaVao ? "Tên người bán" : "Tên người mua",   // 6 - F
                isMuaVao ? "Mã số thuế người bán" : "Mã số thuế người mua", // 7 - G
                "Mặt hàng (Tên hàng hóa, dịch vụ)",             // 8 - H
                "Đơn vị tính",                                  // 9 - I
                "Số lượng",                                     // 10 - J
                "Đơn giá",                                      // 11 - K
                "Doanh số (Thành tiền chưa VAT)",               // 12 - L
                "Thuế suất (%)",                                // 13 - M
                "Tiền thuế GTGT",                               // 14 - N
                "Tổng tiền thanh toán",                         // 15 - O
                "Ghi chú / Tính chất"                           // 16 - P
            };

            for (int col = 0; col < headers.Length; col++)
            {
                var cell = ws.Cell(5, col + 1);
                cell.Value = headers[col];
                cell.Style
                    .Font.SetBold(true)
                    .Font.SetFontSize(9)
                    .Font.SetFontName("Times New Roman")
                    .Font.SetFontColor(XLColor.Black)
                    .Fill.SetBackgroundColor(XLColor.FromHtml("#D9E1F2"))
                    .Alignment.SetHorizontal(XLAlignmentHorizontalValues.Center)
                    .Alignment.SetVertical(XLAlignmentVerticalValues.Center)
                    .Alignment.SetWrapText(true);

                cell.Style.Border.OutsideBorder = XLBorderStyleValues.Thin;
                cell.Style.Border.OutsideBorderColor = XLColor.FromHtml("#B0C4DE");
            }
            ws.Row(5).Height = 32;

            // DÒNG 6 TRỞ ĐI: DỮ LIỆU TỪNG DÒNG HÀNG HÓA
            int dataStartRow = 6;
            for (int i = 0; i < flatItems.Count; i++)
            {
                int row = dataStartRow + i;
                var item = flatItems[i];

                ws.Cell(row, 1).Value = i + 1;
                ws.Cell(row, 2).Value = item.Khmshdon;
                ws.Cell(row, 3).Value = item.Khhdon;
                ws.Cell(row, 4).Value = item.Shdon;

                if (item.NgayLap.HasValue)
                {
                    ws.Cell(row, 5).Value = item.NgayLap.Value;
                    ws.Cell(row, 5).Style.DateFormat.Format = "dd/MM/yyyy";
                }
                else
                {
                    ws.Cell(row, 5).Value = "";
                }

                ws.Cell(row, 6).Value = item.TenDoiTac;
                ws.Cell(row, 7).Value = item.MstDoiTac;
                ws.Cell(row, 8).Value = item.TenHangHoa;
                ws.Cell(row, 9).Value = item.Dvtinh;

                if (item.SoLuong != 0)
                {
                    ws.Cell(row, 10).Value = item.SoLuong;
                    ws.Cell(row, 10).Style.NumberFormat.SetFormat(item.SoLuong % 1 == 0 ? "#,##0" : "#,##0.###");
                }
                else ws.Cell(row, 10).Value = "";

                if (item.DonGia != 0)
                {
                    ws.Cell(row, 11).Value = item.DonGia;
                    ws.Cell(row, 11).Style.NumberFormat.SetFormat(item.DonGia % 1 == 0 ? "#,##0" : "#,##0.##");
                }
                else ws.Cell(row, 11).Value = "";

                ws.Cell(row, 12).Value = item.ThTien;
                ws.Cell(row, 12).Style.NumberFormat.SetFormat("#,##0");

                string tsuatRaw = (item.TSuat ?? "").Trim();
                string cleanTsuat = tsuatRaw.Replace("%", "").Trim();
                if (double.TryParse(cleanTsuat, System.Globalization.NumberStyles.Any, System.Globalization.CultureInfo.InvariantCulture, out double rateVal) && rateVal >= 0 && rateVal <= 100)
                {
                    ws.Cell(row, 13).Value = rateVal / 100.0;
                    ws.Cell(row, 13).Style.NumberFormat.SetFormat("0%");
                }
                else
                {
                    ws.Cell(row, 13).Value = tsuatRaw;
                }

                ws.Cell(row, 14).Value = item.TThue;
                ws.Cell(row, 14).Style.NumberFormat.SetFormat("#,##0");

                ws.Cell(row, 15).Value = item.TongTien;
                ws.Cell(row, 15).Style.NumberFormat.SetFormat("#,##0");

                ws.Cell(row, 16).Value = item.TinhChat;

                ws.Row(row).Height = 20;

                for (int c = 1; c <= 16; c++)
                {
                    var cell = ws.Cell(row, c);
                    cell.Style.Font.SetFontName("Times New Roman").Font.SetFontSize(9);

                    cell.Style.Border.OutsideBorder = XLBorderStyleValues.Thin;
                    cell.Style.Border.OutsideBorderColor = XLColor.FromHtml("#E0E0E0");

                    cell.Style.Alignment.SetVertical(XLAlignmentVerticalValues.Center);

                    if (c == 1)
                    {
                        cell.Style.NumberFormat.SetFormat("#,##0");
                    }
                }
            }

            // DÒNG TỔNG CỘNG
            int lastDataRow = dataStartRow + flatItems.Count - 1;
            int summaryRow = lastDataRow + 1;

            // Gom cột 1 -> 9 hiển thị "TỔNG CỘNG :"
            ws.Range(summaryRow, 1, summaryRow, 9).Merge();
            ws.Cell(summaryRow, 1).Value = "TỔNG CỘNG :";
            ws.Cell(summaryRow, 1).Style
                .Font.SetBold(true)
                .Font.SetFontSize(10)
                .Font.SetFontName("Times New Roman")
                .Alignment.SetHorizontal(XLAlignmentHorizontalValues.Right)
                .Alignment.SetVertical(XLAlignmentVerticalValues.Center);

            // SUM: Số lượng (J), Thành tiền (L), Tiền thuế (N), Tổng thanh toán (O)
            int[] sumCols = { 10, 12, 14, 15 };
            bool hasDecimalQty = flatItems.Any(x => x.SoLuong % 1 != 0);
            foreach (int c in sumCols)
            {
                string colLetter = GetColumnLetter(c);
                ws.Cell(summaryRow, c).FormulaA1 = $"SUM({colLetter}{dataStartRow}:{colLetter}{lastDataRow})";
                ws.Cell(summaryRow, c).Style
                    .Font.SetBold(true)
                    .Font.SetFontSize(10)
                    .Font.SetFontName("Times New Roman")
                    .NumberFormat.SetFormat(c == 10 ? (hasDecimalQty ? "#,##0.###" : "#,##0") : "#,##0");
            }

            var summaryRange = ws.Range(summaryRow, 1, summaryRow, 16);
            summaryRange.Style
                .Font.SetBold(true)
                .Font.SetFontSize(10)
                .Font.SetFontName("Times New Roman")
                .Border.SetTopBorder(XLBorderStyleValues.Double)
                .Border.SetTopBorderColor(XLColor.Black)
                .Border.SetBottomBorder(XLBorderStyleValues.Double)
                .Border.SetBottomBorderColor(XLColor.Black)
                .Fill.SetBackgroundColor(XLColor.FromHtml("#FFF8E1"));

            // ĐỘ RỘNG CỘT
            int[] colWidths = { 6, 14, 14, 12, 14, 34, 18, 38, 12, 12, 14, 18, 12, 16, 18, 22 };
            for (int c = 0; c < colWidths.Length; c++)
            {
                ws.Column(c + 1).Width = colWidths[c];
            }

            // ĐÓNG BĂNG DÒNG HEADER
            ws.SheetView.FreezeRows(5);

            // TẠO ĐƯỜNG DẪN VÀ LƯU FILE
            string finalSavePath;
            if (targetPathOrDir.EndsWith(".xlsx", StringComparison.OrdinalIgnoreCase))
            {
                finalSavePath = targetPathOrDir;
            }
            else
            {
                string fromClean = fromDate?.Replace("/", "") ?? "";
                string toClean = toDate?.Replace("/", "") ?? "";
                string dirLabel = isMuaVao ? "MuaVao" : "BanRa";
                string rand = $"{DateTime.Now:HHmmss}_{Random.Shared.Next(100, 999)}";
                string fileName = $"BangKeChiTiet_{dirLabel}_{mst}_{fromClean}_{toClean}_{rand}.xlsx";

                foreach (char c in Path.GetInvalidFileNameChars())
                {
                    fileName = fileName.Replace(c, '_');
                }
                finalSavePath = Path.Combine(targetPathOrDir, fileName);
            }

            if (File.Exists(finalSavePath))
            {
                try { File.Delete(finalSavePath); }
                catch
                {
                    string dir = Path.GetDirectoryName(finalSavePath) ?? "";
                    string baseName = Path.GetFileNameWithoutExtension(finalSavePath);
                    string timestamp = DateTime.Now.ToString("HHmmss");
                    finalSavePath = Path.Combine(dir, $"{baseName}_{timestamp}.xlsx");
                }
            }

            workbook.SaveAs(finalSavePath);
            Debug.WriteLine($"[ExportExcel] Đã xuất {flatItems.Count} dòng chi tiết hàng hóa -> {finalSavePath}");
            return finalSavePath;
        }

        // ─────────────────────────────────────────────────────────────
        // HÀM TIỆN ÍCH HỖ TRỢ
        // ─────────────────────────────────────────────────────────────

        /// <summary>
        /// Tạo key đối chiếu chuẩn hóa giữa Số HĐ và Ký hiệu HĐ (viết hoa ký hiệu, bỏ 0 đầu số HĐ)
        /// </summary>
        private static string BuildMatchKey(string? kyHieu, string? soHDon)
        {
            if (string.IsNullOrWhiteSpace(kyHieu) && string.IsNullOrWhiteSpace(soHDon)) return string.Empty;
            string normKyHieu = (kyHieu ?? string.Empty).Trim().ToUpperInvariant();
            string normSoHDon = (soHDon ?? string.Empty).Trim().TrimStart('0');
            return $"{normKyHieu}_{normSoHDon}";
        }

        /// <summary>
        /// Parse chuỗi ngày tháng từ nhiều định dạng khác nhau
        /// </summary>
        private static DateTime? ParseDate(string? dateStr)
        {
            if (string.IsNullOrWhiteSpace(dateStr)) return null;
            dateStr = dateStr.Trim();

            string[] formats = new[]
            {
                "dd/MM/yyyy",
                "yyyy-MM-dd",
                "yyyy-MM-ddTHH:mm:ss",
                "yyyy-MM-ddTHH:mm:ss.fffZ",
                "yyyy-MM-ddTHH:mm:ssZ",
                "yyyy-MM-dd HH:mm:ss"
            };

            if (DateTime.TryParseExact(dateStr, formats,
                System.Globalization.CultureInfo.InvariantCulture,
                System.Globalization.DateTimeStyles.None, out DateTime dt))
            {
                return dt;
            }

            if (DateTime.TryParse(dateStr, out DateTime dtAny))
            {
                return dtAny;
            }

            return null;
        }

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
        /// Parse chuỗi số tiền Việt Nam thành double
        /// </summary>
        private static double ParseNumber(JObject obj, params string[] fieldNames)
        {
            foreach (var fieldName in fieldNames)
            {
                var val = obj[fieldName];
                if (val == null || val.Type == JTokenType.Null) continue;

                string s = val.ToString().Trim();
                if (string.IsNullOrEmpty(s) || s == "---") continue;

                s = s.Replace(".", "").Replace(",", ".").Trim();
                s = System.Text.RegularExpressions.Regex.Replace(s, @"[^\d.\-]", "");

                if (double.TryParse(s, System.Globalization.NumberStyles.Any,
                    System.Globalization.CultureInfo.InvariantCulture, out double result))
                {
                    return result;
                }
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
        /// </summary>
        private static ParsedInfo ParseThongTinHoaDon(string raw, bool isMuaVao)
        {
            var result = new ParsedInfo();
            if (string.IsNullOrEmpty(raw)) return result;

            string doiTuong = isMuaVao ? "bán" : "mua";

            var mstMatch = System.Text.RegularExpressions.Regex.Match(
                raw, $@"MST\s*người\s*{doiTuong}\s*:\s*(\d{{10,14}})",
                System.Text.RegularExpressions.RegexOptions.IgnoreCase);
            if (mstMatch.Success)
                result.Mst = mstMatch.Groups[1].Value.Trim();

            var tenMatch = System.Text.RegularExpressions.Regex.Match(
                raw, $@"Tên\s*người\s*{doiTuong}\s*:\s*(.+?)(?=Địa\s*chỉ|$)",
                System.Text.RegularExpressions.RegexOptions.IgnoreCase);
            if (tenMatch.Success)
                result.Ten = tenMatch.Groups[1].Value.Trim();

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

        private class FlatLineItem
        {
            public string Khmshdon { get; set; } = "";
            public string Khhdon { get; set; } = "";
            public string Shdon { get; set; } = "";
            public DateTime? NgayLap { get; set; }
            public string TenDoiTac { get; set; } = "";
            public string MstDoiTac { get; set; } = "";
            public string TenHangHoa { get; set; } = "";
            public string Dvtinh { get; set; } = "";
            public double SoLuong { get; set; }
            public double DonGia { get; set; }
            public double ThTien { get; set; }
            public string TSuat { get; set; } = "";
            public double TThue { get; set; }
            public double TongTien { get; set; }
            public string TinhChat { get; set; } = "Hàng hóa, dịch vụ";
        }
    }
}
