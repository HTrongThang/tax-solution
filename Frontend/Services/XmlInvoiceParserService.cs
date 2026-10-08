using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Xml.Linq;
using Frontend.Models;

namespace Frontend.Services
{
    /// <summary>
    /// MODULE BÓC TÁCH HÓA ĐƠN ĐIỆN TỬ XML (XML INVOICE PARSER)
    /// Chuẩn Tổng cục Thuế theo Nghị định 123/2020/NĐ-CP và Thông tư 78/2021/TT-BTC.
    /// Tương thích các nhà cung cấp phổ biến: VNPT, Viettel, MISA, BKAV, Fast, EasyInvoice, CyberBill...
    /// </summary>
    public class XmlInvoiceParserService
    {
        private static readonly Lazy<XmlInvoiceParserService> _instance = 
            new Lazy<XmlInvoiceParserService>(() => new XmlInvoiceParserService());
        public static XmlInvoiceParserService Instance => _instance.Value;

        /// <summary>
        /// Bộ nhớ lưu trữ kết quả bóc tách gần nhất để phục vụ ngay cho Module Xuất Excel
        /// </summary>
        public static List<InvoiceXmlModel> LastParsedInvoices { get; set; } = new List<InvoiceXmlModel>();

        // ═════════════════════════════════════════════════════════════════════════
        // BƯỚC 1: QUÉT THƯ MỤC & KHỞI TẠO TIẾN TRÌNH
        // ═════════════════════════════════════════════════════════════════════════

        /// <summary>
        /// Quét thư mục và bóc tách các file hóa đơn XML bất đồng bộ (tránh treo giao diện)
        /// </summary>
        public async Task<XmlParseResult> ParseDirectoryAsync(
            string directoryPath,
            XmlParseOptions? options = null,
            IEnumerable<dynamic>? existingInvoices = null,
            IProgress<(int current, int total, string currentFile)>? progress = null)
        {
            return await Task.Run(() => ParseDirectory(directoryPath, options, existingInvoices, progress));
        }

        /// <summary>
        /// Quét thư mục và bóc tách các file hóa đơn XML bất đồng bộ (overload tương thích cũ)
        /// </summary>
        public async Task<XmlParseResult> ParseDirectoryAsync(
            string directoryPath, 
            IEnumerable<dynamic>? existingInvoices,
            IProgress<(int current, int total, string currentFile)>? progress)
        {
            return await Task.Run(() => ParseDirectory(directoryPath, null, existingInvoices, progress));
        }

        /// <summary>
        /// Quét thư mục và bóc tách toàn bộ file .xml hóa đơn (đồng bộ)
        /// </summary>
        public XmlParseResult ParseDirectory(
            string directoryPath, 
            XmlParseOptions? options = null,
            IEnumerable<dynamic>? existingInvoices = null,
            IProgress<(int current, int total, string currentFile)>? progress = null)
        {
            var result = new XmlParseResult();

            // 1. Kiểm tra thư mục đầu vào
            if (string.IsNullOrWhiteSpace(directoryPath) || !Directory.Exists(directoryPath))
            {
                result.Success = false;
                result.Message = $"Thư mục không tồn tại: {directoryPath}";
                return result;
            }

            // 2. Tìm kiếm file .xml mục tiêu
            // - Nếu thư mục chỉ định đã chứa trực tiếp file .xml -> Chỉ lấy các file trong thư mục này (TopDirectoryOnly)
            // - Nếu thư mục chỉ định là thư mục cha chứa nhiều lần tải (các thư mục con dạng MST_Type_DateTime) -> Tự động chọn thư mục con mới nhất vừa tải về
            string targetFolder = directoryPath;
            string[] xmlFiles = Array.Empty<string>();

            try
            {
                var directXmlFiles = Directory.GetFiles(targetFolder, "*.xml", SearchOption.TopDirectoryOnly);
                if (directXmlFiles.Length > 0)
                {
                    xmlFiles = directXmlFiles;
                }
                else
                {
                    // Kiểm tra các thư mục con và chọn thư mục được tạo/ghi gần đây nhất
                    var subDirs = Directory.GetDirectories(targetFolder);
                    if (subDirs.Length > 0)
                    {
                        var latestDir = subDirs
                            .Select(d => new DirectoryInfo(d))
                            .OrderByDescending(d => d.LastWriteTimeUtc)
                            .FirstOrDefault();

                        if (latestDir != null)
                        {
                            targetFolder = latestDir.FullName;
                            xmlFiles = Directory.GetFiles(targetFolder, "*.xml", SearchOption.TopDirectoryOnly);
                            Debug.WriteLine($"[XmlInvoiceParserService] Đã tự động chọn thư mục mới tải về gần nhất: {targetFolder} ({xmlFiles.Length} files)");
                        }
                    }
                }
            }
            catch (Exception ex)
            {
                result.Success = false;
                result.Message = $"Không thể truy cập thư mục '{targetFolder}': {ex.Message}";
                return result;
            }

            result.TotalFilesFound = xmlFiles.Length;

            if (xmlFiles.Length == 0)
            {
                result.Success = true;
                result.Message = $"Không tìm thấy file .xml nào trong thư mục: {targetFolder}";
                return result;
            }

            // 3. Đọc và bóc tách từng file XML với khối try-catch an toàn (Fault-tolerant)
            int parsedCount = 0;
            int skippedCount = 0;
            int failedCount = 0;

            for (int i = 0; i < xmlFiles.Length; i++)
            {
                string filePath = xmlFiles[i];
                string fileName = Path.GetFileName(filePath);

                // Báo cáo tiến độ: mỗi 10 file hoặc file cuối cùng để tránh nghẽn UI
                if (progress != null && (i == 0 || (i + 1) % 10 == 0 || i == xmlFiles.Length - 1))
                {
                    progress.Report((i + 1, xmlFiles.Length, fileName));
                }

                try
                {
                    // BƯỚC 2 & 3: Bóc tách file
                    var invoice = ParseSingleFile(filePath);
                    if (invoice != null)
                    {
                        // BƯỚC 4: Bộ lọc tại chỗ (In-flight Filter)
                        if (PassesInFlightFilter(invoice, options))
                        {
                            // BƯỚC 5: Cân chỉnh sai số thuế làm tròn & Chiết khấu
                            ReconcileTaxRounding(invoice);

                            result.Invoices.Add(invoice);
                            parsedCount++;
                        }
                        else
                        {
                            skippedCount++;
                        }
                    }
                    else
                    {
                        failedCount++;
                        result.Errors.Add($"File '{fileName}' không chứa cấu trúc hóa đơn điện tử hợp lệ hoặc bị lỗi cú pháp.");
                    }
                }
                catch (Exception ex)
                {
                    failedCount++;
                    string errMsg = $"Lỗi đọc file '{fileName}': {ex.Message}";
                    result.Errors.Add(errMsg);
                    Debug.WriteLine($"[XmlInvoiceParserService] {errMsg}");
                }
            }

            result.TotalFilesParsed = parsedCount;
            result.TotalFilesSkipped = skippedCount;
            result.TotalFilesFailed = failedCount;
            result.TargetFolder = targetFolder;

            // Ghép nối dữ liệu với danh sách tóm tắt (nếu có)
            if (existingInvoices != null && result.Invoices.Count > 0)
            {
                MergeWithExistingInvoices(result.Invoices, existingInvoices);
            }

            // BƯỚC 6: ĐÓNG GÓI & BÀN GIAO BỘ NHỚ (Memory Handoff)
            LastParsedInvoices = result.Invoices;
            InvoiceMemoryService.Instance.SetParsedXmlInvoices(result.Invoices);

            // Xóa thư mục tải về tạm sau khi bóc tách xong vào bộ nhớ RAM nếu có yêu cầu
            if (options?.DeleteAfterParsing == true && !string.IsNullOrWhiteSpace(targetFolder) && Directory.Exists(targetFolder))
            {
                try
                {
                    var dirInfo = new DirectoryInfo(targetFolder);
                    if (dirInfo.Parent != null)
                    {
                        Directory.Delete(targetFolder, true);
                        Debug.WriteLine($"[XmlInvoiceParserService] Đã xóa thư mục tải về sau khi bóc tách: {targetFolder}");
                    }
                }
                catch (Exception exDel)
                {
                    Debug.WriteLine($"[XmlInvoiceParserService] Lỗi khi xóa thư mục '{targetFolder}': {exDel.Message}");
                }
            }

            result.Success = parsedCount > 0;
            result.Message = $"Đã quét {xmlFiles.Length} file: Bóc tách thành công {parsedCount} hóa đơn, bỏ qua {skippedCount} file (lọc MST/kỳ), lỗi {failedCount} file.";
            return result;
        }

        /// <summary>
        /// Bóc tách một file XML hóa đơn duy nhất từ đường dẫn file
        /// </summary>
        public InvoiceXmlModel? ParseSingleFile(string filePath)
        {
            if (!File.Exists(filePath)) return null;

            try
            {
                var fileInfo = new FileInfo(filePath);
                if (fileInfo.Length == 0) return null; // File 0 KB

                string xmlContent = File.ReadAllText(filePath);
                var invoice = ParseXmlContent(xmlContent, filePath);
                if (invoice != null)
                {
                    invoice.FilePath = filePath;
                    invoice.FileName = Path.GetFileName(filePath);
                }
                return invoice;
            }
            catch
            {
                return null;
            }
        }

        // ═════════════════════════════════════════════════════════════════════════
        // BƯỚC 2: TIỀN XỬ LÝ & KHỬ XML NAMESPACE
        // BƯỚC 3: BÓC TÁCH 5 KHỐI DỮ LIỆU NGHIỆP VỤ
        // ═════════════════════════════════════════════════════════════════════════

        /// <summary>
        /// Bóc tách dữ liệu từ chuỗi XML hóa đơn điện tử.
        /// Sử dụng XDocument và tìm kiếm theo LocalName để bỏ qua ảnh hưởng của XML Namespace (xmlns="...").
        /// </summary>
        public InvoiceXmlModel? ParseXmlContent(string xmlContent, string? filePath = null)
        {
            if (string.IsNullOrWhiteSpace(xmlContent)) return null;

            // Loại bỏ các ký tự điều khiển không hợp lệ theo chuẩn XML 1.0 (như 0x11, 0x00-0x08, 0x0B-0x0C, 0x0E-0x1F)
            xmlContent = CleanInvalidXmlChars(xmlContent);

            XDocument doc;
            try
            {
                doc = XDocument.Parse(xmlContent);
            }
            catch
            {
                return null;
            }

            if (doc.Root == null) return null;

            // 1. NHẬN DIỆN VÀ LOẠI BỎ CÁC FILE KHÔNG PHẢI HÓA ĐƠN (Thông điệp TCT, Tờ khai thuế, Thông báo)
            string rootName = doc.Root.Name.LocalName;
            if (rootName.Equals("TDiep", StringComparison.OrdinalIgnoreCase) ||
                rootName.Equals("TKhai", StringComparison.OrdinalIgnoreCase) ||
                rootName.Equals("HSoThueDTu", StringComparison.OrdinalIgnoreCase) ||
                rootName.Equals("TBao", StringComparison.OrdinalIgnoreCase))
            {
                // Chỉ tiếp tục nếu bên trong có gói hóa đơn con (<DLHDon> hoặc <HDon>)
                bool containsInvoiceChild = FindElement(doc.Root, "DLHDon") != null || FindElement(doc.Root, "HDon") != null;
                if (!containsInvoiceChild)
                {
                    return null;
                }
            }

            // Nếu chứa thẻ mã loại thông điệp MLTDiep/MTDiep mà không có node hóa đơn -> Loại bỏ
            if (FindElement(doc.Root, "MLTDiep") != null && FindElement(doc.Root, "DLHDon") == null && FindElement(doc.Root, "HDon") == null)
            {
                return null;
            }

            // Tìm node dữ liệu chính: <DLHDon> (TT78) hoặc fallback <Invoice>, <HDon>, <Content> (TT32)
            var dlHDon = FindElement(doc.Root, "DLHDon") 
                      ?? FindElement(doc.Root, "Invoice") 
                      ?? FindElement(doc.Root, "HDon") 
                      ?? FindElement(doc.Root, "Content") 
                      ?? doc.Root;

            // Kiểm tra node thông tin chung: <TTChung>
            var ttChung = FindElement(dlHDon, "TTChung") ?? FindElement(doc.Root, "TTChung");
            // Kiểm tra node nội dung hóa đơn: <NDHDon>
            var ndHDon = FindElement(dlHDon, "NDHDon") ?? FindElement(doc.Root, "NDHDon");

            // Nếu không có cả TTChung lẫn NDHDon thì kiểm tra xem chính dlHDon có chứa thông tin hóa đơn không
            if (ttChung == null && ndHDon == null)
            {
                // Thử tìm nBan hoặc nMua trực tiếp trong dlHDon
                var testNBan = FindElement(dlHDon, "NBan") ?? FindElement(doc.Root, "NBan");
                if (testNBan == null) return null;
            }

            var invoice = new InvoiceXmlModel
            {
                FilePath = filePath ?? string.Empty,
                FileName = string.IsNullOrEmpty(filePath) ? string.Empty : Path.GetFileName(filePath)
            };

            // ─────────────────────────────────────────────────────────────────
            // KHỐI 1: THÔNG TIN CHUNG HEADER (<TTChung>)
            // ─────────────────────────────────────────────────────────────────
            if (ttChung != null)
            {
                invoice.KHMSHDon = GetElementValue(ttChung, "KHMSHDon");
                invoice.KHHDon = GetElementValue(ttChung, "KHHDon");

                // Số hóa đơn: Lấy giá trị số và loại bỏ toàn bộ khoảng trắng thừa
                string rawShdon = GetElementValue(ttChung, "SHDon");
                invoice.SHDon = Regex.Replace(rawShdon ?? "", @"\s+", "").Trim();

                invoice.NLapRaw = GetElementValue(ttChung, "NLap");
                invoice.NLap = ParseDateTime(invoice.NLapRaw);

                string dvtte = GetElementValue(ttChung, "DVTTe");
                invoice.DVTTe = string.IsNullOrWhiteSpace(dvtte) ? "VND" : dvtte.Trim();

                string tGiaStr = GetElementValue(ttChung, "TGia");
                invoice.TGia = ParseDecimal(tGiaStr) ?? 1m;

                invoice.HTTToan = GetElementValue(ttChung, "HTTToan");
                invoice.MCCQT = GetElementValue(ttChung, "MCCQT");
            }

            // Fallback mã cơ quan thuế nếu không nằm trong <TTChung>
            if (string.IsNullOrEmpty(invoice.MCCQT))
            {
                invoice.MCCQT = GetElementValue(dlHDon, "MCCQT") 
                             ?? GetElementValue(doc.Root, "MCCQT") 
                             ?? string.Empty;
            }

            // ─────────────────────────────────────────────────────────────────
            // KHỐI 2: BÊN BÁN (<NBan>)
            // ─────────────────────────────────────────────────────────────────
            var nBan = ndHDon != null ? FindElement(ndHDon, "NBan") : (FindElement(dlHDon, "NBan") ?? FindElement(doc.Root, "NBan"));
            if (nBan != null)
            {
                invoice.NBanTen = GetElementValue(nBan, "Ten");
                // Chuẩn hóa MST: xóa khoảng trắng, gạch nối thừa
                invoice.NBanMST = CleanMst(GetElementValue(nBan, "MST"));
                invoice.NBanDChi = GetElementValue(nBan, "DChi");
            }

            // ─────────────────────────────────────────────────────────────────
            // KHỐI 3: BÊN MUA (<NMua>)
            // ─────────────────────────────────────────────────────────────────
            var nMua = ndHDon != null ? FindElement(ndHDon, "NMua") : (FindElement(dlHDon, "NMua") ?? FindElement(doc.Root, "NMua"));
            if (nMua != null)
            {
                invoice.NMuaTen = GetElementValue(nMua, "Ten");
                // Chuẩn hóa MST: xóa khoảng trắng, gạch nối thừa
                invoice.NMuaMST = CleanMst(GetElementValue(nMua, "MST"));
                invoice.NMuaDChi = GetElementValue(nMua, "DChi");
                invoice.HVTNMHang = GetElementValue(nMua, "HVTNMHang");
            }

            // ─────────────────────────────────────────────────────────────────
            // KHỐI 5: TỔNG THANH TOÁN (<TToan> hoặc <TTTToan>)
            // ─────────────────────────────────────────────────────────────────
            var tToan = ndHDon != null ? (FindElement(ndHDon, "TToan") ?? FindElement(ndHDon, "TTTToan")) 
                                       : (FindElement(dlHDon, "TToan") ?? FindElement(dlHDon, "TTTToan") ?? FindElement(doc.Root, "TToan") ?? FindElement(doc.Root, "TTTToan"));
            if (tToan != null)
            {
                invoice.TgTCThue = ParseDecimal(GetElementValue(tToan, "TgTCThue"));
                invoice.TgTThue = ParseDecimal(GetElementValue(tToan, "TgTThue"));
                invoice.TgTTTBSo = ParseDecimal(GetElementValue(tToan, "TgTTTBSo"));
                invoice.TgTTTBChu = GetElementValue(tToan, "TgTTTBChu");
            }

            // ─────────────────────────────────────────────────────────────────
            // KHỐI 4: DANH SÁCH CHI TIẾT DÒNG HÀNG (<DSHHDVu> -> lặp <HHDVu>)
            // ─────────────────────────────────────────────────────────────────
            var dsHHDVu = ndHDon != null ? FindElement(ndHDon, "DSHHDVu") 
                                         : (FindElement(dlHDon, "DSHHDVu") ?? FindElement(doc.Root, "DSHHDVu"));
            if (dsHHDVu != null)
            {
                var hhdVuList = FindElements(dsHHDVu, "HHDVu");
                int autoStt = 1;

                foreach (var itemElem in hhdVuList)
                {
                    var lineItem = new InvoiceXmlItemModel();

                    // 1. Tính chất dòng hàng: "1": Hàng hóa, "3": Chiết khấu thương mại
                    string tChat = GetElementValue(itemElem, "TChat");
                    lineItem.TChat = string.IsNullOrWhiteSpace(tChat) ? "1" : tChat.Trim();

                    // 2. STT
                    string sttStr = GetElementValue(itemElem, "STT");
                    if (int.TryParse(sttStr, out int sttVal))
                    {
                        lineItem.STT = sttVal;
                    }
                    else
                    {
                        lineItem.STT = autoStt;
                    }
                    autoStt++;

                    // 3. Tên hàng hóa, dịch vụ, ĐVT, Mã
                    lineItem.MHHDVu = GetElementValue(itemElem, "MHHDVu");
                    lineItem.THHDVu = GetElementValue(itemElem, "THHDVu");
                    lineItem.DVTinh = GetElementValue(itemElem, "DVTinh");

                    // 4. Số lượng, Đơn giá, Thành tiền
                    lineItem.SLuong = ParseDecimal(GetElementValue(itemElem, "SLuong"));
                    lineItem.DGia = ParseDecimal(GetElementValue(itemElem, "DGia"));
                    
                    decimal? rawThTien = ParseDecimal(GetElementValue(itemElem, "ThTien"));

                    // NGUYÊN TẮC CHIẾT KHẤU & ĐIỀU CHỈNH GIẢM:
                    // Giữ nguyên bản dấu âm chiết khấu (TChat == "3") để SUM Excel tự bù trừ chính xác
                    if (rawThTien.HasValue)
                    {
                        if (lineItem.TChat == "3" && rawThTien.Value > 0)
                        {
                            lineItem.ThTien = -rawThTien.Value;
                        }
                        else
                        {
                            lineItem.ThTien = rawThTien.Value;
                        }
                    }
                    else
                    {
                        lineItem.ThTien = null;
                    }

                    // 5. Thuế suất: Tách thành dạng số và dạng chuỗi
                    lineItem.TSuat = GetElementValue(itemElem, "TSuat");
                    lineItem.ThueSuatPhanTram = ParseTaxRate(lineItem.TSuat);

                    // 6. Tiền thuế dòng hàng (<TThue>): Giữ dấu âm nếu là chiết khấu
                    string tThueStr = GetElementValue(itemElem, "TThue");
                    decimal? rawTThue = ParseDecimal(tThueStr);

                    if (rawTThue.HasValue)
                    {
                        if (lineItem.TChat == "3" && rawTThue.Value > 0)
                        {
                            lineItem.TThue = -rawTThue.Value;
                        }
                        else
                        {
                            lineItem.TThue = rawTThue.Value;
                        }
                    }
                    else if (lineItem.ThueSuatPhanTram.HasValue && lineItem.ThueSuatPhanTram.Value > 0 && lineItem.ThTien.HasValue)
                    {
                        lineItem.TThue = lineItem.ThTien.Value * (lineItem.ThueSuatPhanTram.Value / 100m);
                    }
                    else
                    {
                        lineItem.TThue = null;
                    }

                    invoice.HoaDons.Add(lineItem);
                }
            }

            // Fallback tổng tiền nếu thẻ TToan bị thiếu
            if (!invoice.TgTCThue.HasValue && invoice.HoaDons.Count > 0)
            {
                invoice.TgTCThue = invoice.HoaDons.Sum(x => x.ThTien ?? 0m);
            }
            if (!invoice.TgTThue.HasValue && invoice.HoaDons.Count > 0)
            {
                invoice.TgTThue = invoice.HoaDons.Sum(x => x.TThue ?? 0m);
            }
            if (!invoice.TgTTTBSo.HasValue && (invoice.TgTCThue.HasValue || invoice.TgTThue.HasValue))
            {
                invoice.TgTTTBSo = (invoice.TgTCThue ?? 0m) + (invoice.TgTThue ?? 0m);
            }

            // ─────────────────────────────────────────────────────────────────
            // KIỂM TRA TÍNH HỢP LỆ & TOÀN VẸN CỦA HÓA ĐƠN:
            // Bắt buộc phải có Số HĐ hoặc Ký hiệu HĐ, có Bên bán, và có số liệu tài chính/dòng hàng
            // ─────────────────────────────────────────────────────────────────
            bool hasInvoiceNumber = !string.IsNullOrWhiteSpace(invoice.SHDon) || !string.IsNullOrWhiteSpace(invoice.KHHDon);
            bool hasSeller = !string.IsNullOrWhiteSpace(invoice.NBanMST) || !string.IsNullOrWhiteSpace(invoice.NBanTen);
            bool hasFinancialData = invoice.HoaDons.Count > 0 || invoice.TgTTTBSo.HasValue || invoice.TgTCThue.HasValue;

            if (!hasInvoiceNumber || !hasSeller || !hasFinancialData)
            {
                // Không phải hóa đơn điện tử hợp lệ (chỉ là file thông điệp/tờ khai rác)
                return null;
            }

            return invoice;
        }

        // ═════════════════════════════════════════════════════════════════════════
        // BƯỚC 4: BỘ LỌC TẠI CHỖ (IN-FLIGHT FILTER)
        // ═════════════════════════════════════════════════════════════════════════

        /// <summary>
        /// Kiểm tra hóa đơn có thỏa mãn điều kiện lọc MST và Kỳ báo cáo ngay tại chỗ (không nạp RAM nếu không khớp)
        /// </summary>
        private static bool PassesInFlightFilter(InvoiceXmlModel invoice, XmlParseOptions? options)
        {
            if (options == null) return true;

            // 1. Kiểm tra Mã số thuế (targetMst)
            if (!string.IsNullOrWhiteSpace(options.TargetMst))
            {
                string cleanTarget = CleanMst(options.TargetMst);

                if (options.IsMuaVao == true)
                {
                    // Mua vào: So sánh NMua.MST == targetMst
                    string cleanNMua = CleanMst(invoice.NMuaMST);
                    if (!string.IsNullOrEmpty(cleanNMua) && !cleanNMua.Equals(cleanTarget, StringComparison.OrdinalIgnoreCase))
                    {
                        return false;
                    }
                }
                else if (options.IsMuaVao == false)
                {
                    // Bán ra: So sánh NBan.MST == targetMst
                    string cleanNBan = CleanMst(invoice.NBanMST);
                    if (!string.IsNullOrEmpty(cleanNBan) && !cleanNBan.Equals(cleanTarget, StringComparison.OrdinalIgnoreCase))
                    {
                        return false;
                    }
                }
            }

            // 2. Kiểm tra Kỳ báo cáo (fromDate - toDate)
            DateTime? fromDate = options.FromDate ?? ParseDateTime(options.FromDateStr);
            DateTime? toDate = options.ToDate ?? ParseDateTime(options.ToDateStr);

            if (invoice.NLap.HasValue)
            {
                if (fromDate.HasValue && invoice.NLap.Value.Date < fromDate.Value.Date)
                {
                    return false;
                }
                if (toDate.HasValue && invoice.NLap.Value.Date > toDate.Value.Date)
                {
                    return false;
                }
            }

            return true;
        }

        // ═════════════════════════════════════════════════════════════════════════
        // BƯỚC 5: CÂN CHỈNH SAI SỐ THUẾ LÀM TRÒN & CHIẾT KHẤU
        // ═════════════════════════════════════════════════════════════════════════

        /// <summary>
        /// Cân chỉnh sai số làm tròn tiền thuế giữa Tổng thuế Header (TgTThue) và Tổng thuế từng dòng con (Sum(TThue))
        /// Nếu lệch 1 - 2 đồng do làm tròn tỷ lệ %, tự động phân bổ vào dòng hàng có thuế lớn nhất.
        /// </summary>
        private static void ReconcileTaxRounding(InvoiceXmlModel invoice)
        {
            if (invoice.HoaDons == null || invoice.HoaDons.Count == 0 || !invoice.TgTThue.HasValue) return;

            decimal headerTax = invoice.TgTThue.Value;
            decimal childTaxSum = invoice.HoaDons.Sum(x => x.TThue ?? 0m);
            decimal diff = headerTax - childTaxSum;

            // Nếu xuất hiện độ lệch làm tròn nhỏ (1 - 5 đồng)
            if (diff != 0 && Math.Abs(diff) <= 5m)
            {
                var targetItem = invoice.HoaDons
                    .Where(x => x.TThue.HasValue && x.TThue != 0)
                    .OrderByDescending(x => Math.Abs(x.TThue ?? 0m))
                    .FirstOrDefault()
                    ?? invoice.HoaDons.OrderByDescending(x => Math.Abs(x.ThTien ?? 0m)).FirstOrDefault();

                if (targetItem != null)
                {
                    targetItem.TThue = (targetItem.TThue ?? 0m) + diff;
                }
            }
        }

        // ═════════════════════════════════════════════════════════════════════════
        // GHÉP NỐI & CÁC HÀM TIỆN ÍCH HỖ TRỢ (HELPERS)
        // ═════════════════════════════════════════════════════════════════════════

        /// <summary>
        /// Ghép nối dữ liệu bóc tách được từ XML vào danh sách hóa đơn tóm tắt (existingInvoices)
        /// Khớp theo cặp khóa: Số HĐ (SHDon) và Ký hiệu (KHHDon)
        /// </summary>
        public List<InvoiceXmlModel> MergeWithExistingInvoices(
            List<InvoiceXmlModel> parsedInvoices, 
            IEnumerable<dynamic> existingInvoices)
        {
            if (parsedInvoices == null || existingInvoices == null) return parsedInvoices ?? new List<InvoiceXmlModel>();

            var existingList = existingInvoices.ToList();
            if (existingList.Count == 0) return parsedInvoices;

            var lookup = new Dictionary<string, dynamic>(StringComparer.OrdinalIgnoreCase);

            foreach (var ext in existingList)
            {
                string kyHieu = GetPropertyString(ext, "ky_hieu_hoa_don", "khhdon", "ky_hieu");
                string soHDon = GetPropertyString(ext, "so_hoa_don", "shdon", "so_hd");

                string key = BuildMatchKey(kyHieu, soHDon);
                if (!string.IsNullOrEmpty(key) && !lookup.ContainsKey(key))
                {
                    lookup[key] = ext;
                }
            }

            foreach (var inv in parsedInvoices)
            {
                string xmlKey = BuildMatchKey(inv.KHHDon, inv.SHDon);
                if (lookup.TryGetValue(xmlKey, out var matchedExt))
                {
                    inv.MatchedExistingInvoice = matchedExt;
                }
                else
                {
                    string strippedSoHDon = StripLeadingZeros(inv.SHDon);
                    var matchedBySoHDon = existingList.FirstOrDefault(e =>
                    {
                        string extSoHDon = GetPropertyString(e, "so_hoa_don", "shdon", "so_hd");
                        return StripLeadingZeros(extSoHDon).Equals(strippedSoHDon, StringComparison.OrdinalIgnoreCase);
                    });

                    if (matchedBySoHDon != null)
                    {
                        inv.MatchedExistingInvoice = matchedBySoHDon;
                    }
                }
            }

            return parsedInvoices;
        }

        #region Helper Functions
        /// <summary>
        /// Xóa khoảng trắng, dấu gạch nối trong MST
        /// </summary>
        public static string CleanMst(string? mst)
        {
            if (string.IsNullOrWhiteSpace(mst)) return string.Empty;
            return Regex.Replace(mst, @"[\s\-]", "").Trim();
        }

        /// <summary>
        /// Loại bỏ các ký tự điều khiển không hợp lệ theo chuẩn XML 1.0 (như 0x11, 0x00-0x08, 0x0B-0x0C, 0x0E-0x1F, 0x7F)
        /// Giúp ngăn ngừa lỗi parser từ các phần mềm xuất file XML lỗi ký tự
        /// </summary>
        public static string CleanInvalidXmlChars(string? text)
        {
            if (string.IsNullOrEmpty(text)) return string.Empty;
            return Regex.Replace(text, @"[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]", "");
        }

        /// <summary>
        /// Tạo khóa đối chiếu chuẩn hóa giữa Số HĐ và Ký hiệu HĐ (bỏ số 0 đầu, viết hoa)
        /// </summary>
        private static string BuildMatchKey(string? kyHieu, string? soHDon)
        {
            if (string.IsNullOrWhiteSpace(kyHieu) && string.IsNullOrWhiteSpace(soHDon)) return string.Empty;
            string normKyHieu = (kyHieu ?? string.Empty).Trim().ToUpperInvariant();
            string normSoHDon = StripLeadingZeros(soHDon);
            return $"{normKyHieu}_{normSoHDon}";
        }

        private static string StripLeadingZeros(string? str)
        {
            if (string.IsNullOrWhiteSpace(str)) return string.Empty;
            string trimmed = str.Trim();
            string res = trimmed.TrimStart('0');
            return string.IsNullOrEmpty(res) ? "0" : res;
        }

        /// <summary>
        /// Tìm element theo LocalName (bỏ qua XmlNamespace và không phân biệt hoa thường)
        /// </summary>
        private static XElement? FindElement(XElement? parent, string localName)
        {
            if (parent == null) return null;
            return parent.Elements().FirstOrDefault(e => e.Name.LocalName.Equals(localName, StringComparison.OrdinalIgnoreCase));
        }

        /// <summary>
        /// Tìm danh sách elements con theo LocalName
        /// </summary>
        private static IEnumerable<XElement> FindElements(XElement? parent, string localName)
        {
            if (parent == null) return Enumerable.Empty<XElement>();
            return parent.Elements().Where(e => e.Name.LocalName.Equals(localName, StringComparison.OrdinalIgnoreCase));
        }

        /// <summary>
        /// Lấy giá trị chuỗi của element con
        /// </summary>
        private static string GetElementValue(XElement? parent, string localName)
        {
            if (parent == null) return string.Empty;
            var elem = FindElement(parent, localName);
            return elem?.Value?.Trim() ?? string.Empty;
        }

        /// <summary>
        /// Ép kiểu số thập phân từ chuỗi XML (hỗ trợ cả dấu . và ,)
        /// </summary>
        public static decimal? ParseDecimal(string? str)
        {
            if (string.IsNullOrWhiteSpace(str)) return null;

            str = str.Trim();
            str = Regex.Replace(str, @"[^\d,\.\-]", "");

            if (string.IsNullOrWhiteSpace(str)) return null;

            if (str.Contains(',') && str.Contains('.'))
            {
                if (str.IndexOf('.') < str.IndexOf(','))
                {
                    str = str.Replace(".", "").Replace(',', '.');
                }
                else
                {
                    str = str.Replace(",", "");
                }
            }
            else if (str.Contains(','))
            {
                str = str.Replace(',', '.');
            }

            if (decimal.TryParse(str, NumberStyles.Any, CultureInfo.InvariantCulture, out decimal val))
            {
                return val;
            }

            return null;
        }

        /// <summary>
        /// Trích xuất phần trăm thuế suất dạng số từ chuỗi gốc
        /// VD: "8%" -> 8, "10%" -> 10, "5%" -> 5, "0%" -> 0, "KCT" -> null
        /// </summary>
        public static decimal? ParseTaxRate(string? tSuat)
        {
            if (string.IsNullOrWhiteSpace(tSuat)) return null;

            string clean = tSuat.Trim();
            var match = Regex.Match(clean, @"^(\d+(\.\d+)?)%");
            if (match.Success && decimal.TryParse(match.Groups[1].Value, NumberStyles.Any, CultureInfo.InvariantCulture, out decimal rate))
            {
                return rate;
            }

            if (decimal.TryParse(clean, NumberStyles.Any, CultureInfo.InvariantCulture, out decimal directRate))
            {
                return directRate;
            }

            return null;
        }

        /// <summary>
        /// Phân tích ngày tháng linh hoạt từ các định dạng yyyy-MM-dd, dd/MM/yyyy, dd-MM-yyyy, ISO 8601
        /// </summary>
        public static DateTime? ParseDateTime(string? dateStr)
        {
            if (string.IsNullOrWhiteSpace(dateStr)) return null;

            dateStr = dateStr.Trim();

            string[] formats = new[]
            {
                "yyyy-MM-dd",
                "dd/MM/yyyy",
                "dd-MM-yyyy",
                "yyyy/MM/dd",
                "yyyy-MM-ddTHH:mm:ss",
                "yyyy-MM-ddTHH:mm:ss.fff",
                "yyyy-MM-ddTHH:mm:ssZ"
            };

            if (DateTime.TryParseExact(dateStr, formats, CultureInfo.InvariantCulture, DateTimeStyles.None, out DateTime dt))
            {
                return dt;
            }

            if (DateTime.TryParse(dateStr, CultureInfo.GetCultureInfo("vi-VN"), DateTimeStyles.None, out dt))
            {
                return dt;
            }

            if (DateTime.TryParse(dateStr, CultureInfo.InvariantCulture, DateTimeStyles.None, out dt))
            {
                return dt;
            }

            return null;
        }

        /// <summary>
        /// Lấy giá trị chuỗi từ đối tượng dynamic theo danh sách tên thuộc tính khả dĩ
        /// </summary>
        private static string GetPropertyString(dynamic obj, params string[] propNames)
        {
            if (obj == null) return string.Empty;

            try
            {
                if (obj is IDictionary<string, object> dict)
                {
                    foreach (var name in propNames)
                    {
                        var key = dict.Keys.FirstOrDefault(k => k.Equals(name, StringComparison.OrdinalIgnoreCase));
                        if (key != null && dict[key] != null)
                        {
                            return dict[key]?.ToString()?.Trim() ?? string.Empty;
                        }
                    }
                }
                else if (obj is Newtonsoft.Json.Linq.JObject jObj)
                {
                    foreach (var name in propNames)
                    {
                        var token = jObj.Properties().FirstOrDefault(p => p.Name.Equals(name, StringComparison.OrdinalIgnoreCase))?.Value;
                        if (token != null)
                        {
                            return token.ToString().Trim();
                        }
                    }
                }
                else
                {
                    var type = obj.GetType();
                    foreach (var name in propNames)
                    {
                        var prop = type.GetProperty(name, System.Reflection.BindingFlags.IgnoreCase | System.Reflection.BindingFlags.Public | System.Reflection.BindingFlags.Instance);
                        if (prop != null)
                        {
                            var val = prop.GetValue(obj);
                            if (val != null) return val.ToString()?.Trim() ?? string.Empty;
                        }
                    }
                }
            }
            catch
            {
                // Bỏ qua lỗi truy cập dynamic
            }

            return string.Empty;
        }
        #endregion
    }
}
