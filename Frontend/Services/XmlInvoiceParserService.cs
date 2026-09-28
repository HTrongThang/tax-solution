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
    /// Service chuyên trách việc quét thư mục và bóc tách toàn bộ dữ liệu từ các file XML Hóa đơn điện tử
    /// Chuẩn Tổng cục Thuế theo Nghị định 123/2020/NĐ-CP và Thông tư 78/2021/TT-BTC.
    /// </summary>
    public class XmlInvoiceParserService
    {
        private static readonly Lazy<XmlInvoiceParserService> _instance = 
            new Lazy<XmlInvoiceParserService>(() => new XmlInvoiceParserService());
        public static XmlInvoiceParserService Instance => _instance.Value;

        /// <summary>
        /// Quét toàn bộ thư mục và bóc tách các file hóa đơn XML bất đồng bộ (tránh treo giao diện WinForms/WebView2)
        /// </summary>
        public async Task<XmlParseResult> ParseDirectoryAsync(
            string directoryPath, 
            IEnumerable<dynamic>? existingInvoices = null,
            IProgress<(int current, int total, string currentFile)>? progress = null)
        {
            return await Task.Run(() => ParseDirectory(directoryPath, existingInvoices, progress));
        }

        /// <summary>
        /// Quét thư mục và bóc tách toàn bộ file .xml hóa đơn (đồng bộ)
        /// </summary>
        public XmlParseResult ParseDirectory(
            string directoryPath, 
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

            // 2. Tìm kiếm toàn bộ file .xml (bao gồm cả thư mục con)
            string[] xmlFiles;
            try
            {
                xmlFiles = Directory.GetFiles(directoryPath, "*.xml", SearchOption.AllDirectories);
            }
            catch (Exception ex)
            {
                result.Success = false;
                result.Message = $"Không thể truy cập thư mục: {ex.Message}";
                return result;
            }

            result.TotalFilesFound = xmlFiles.Length;

            if (xmlFiles.Length == 0)
            {
                result.Success = true;
                result.Message = $"Không tìm thấy file .xml nào trong thư mục: {directoryPath}";
                return result;
            }

            // 3. Đọc và bóc tách từng file XML với khối try-catch an toàn
            int parsedCount = 0;
            int failedCount = 0;

            for (int i = 0; i < xmlFiles.Length; i++)
            {
                string filePath = xmlFiles[i];
                string fileName = Path.GetFileName(filePath);
                // Giảm tần suất báo cáo tiến độ: mỗi 10 file hoặc file cuối cùng
                if (progress != null && (i == 0 || (i + 1) % 10 == 0 || i == xmlFiles.Length - 1))
                {
                    progress.Report((i + 1, xmlFiles.Length, fileName));
                }

                try
                {
                    var invoice = ParseSingleFile(filePath);
                    if (invoice != null)
                    {
                        result.Invoices.Add(invoice);
                        parsedCount++;
                    }
                    else
                    {
                        failedCount++;
                        result.Errors.Add($"File '{fileName}' không chứa cấu trúc hóa đơn điện tử hợp lệ.");
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
            result.TotalFilesFailed = failedCount;

            // 4. Ghép nối dữ liệu với danh sách tóm tắt (nếu có)
            if (existingInvoices != null && result.Invoices.Count > 0)
            {
                MergeWithExistingInvoices(result.Invoices, existingInvoices);
            }

            result.Success = parsedCount > 0;
            result.Message = $"Đã quét {xmlFiles.Length} file: Bóc tách thành công {parsedCount} hóa đơn, bỏ qua/lỗi {failedCount} file.";
            return result;
        }

        /// <summary>
        /// Bóc tách một file XML hóa đơn duy nhất từ đường dẫn file.
        /// </summary>
        public InvoiceXmlModel? ParseSingleFile(string filePath)
        {
            if (!File.Exists(filePath)) return null;

            string xmlContent = File.ReadAllText(filePath);
            var invoice = ParseXmlContent(xmlContent, filePath);
            if (invoice != null)
            {
                invoice.FilePath = filePath;
                invoice.FileName = Path.GetFileName(filePath);
            }
            return invoice;
        }

        /// <summary>
        /// Bóc tách dữ liệu từ chuỗi XML hóa đơn điện tử.
        /// Sử dụng XDocument và tìm kiếm theo LocalName để bỏ qua ảnh hưởng của XML Namespace (xmlns).
        /// </summary>
        public InvoiceXmlModel? ParseXmlContent(string xmlContent, string? filePath = null)
        {
            if (string.IsNullOrWhiteSpace(xmlContent)) return null;

            XDocument doc;
            try
            {
                doc = XDocument.Parse(xmlContent);
            }
            catch
            {
                return null;
            }

            // Tìm node DLHDon hoặc node gốc chứa dữ liệu hóa đơn
            var dlHDon = FindElement(doc.Root, "DLHDon") ?? doc.Root;
            if (dlHDon == null) return null;

            // Kiểm tra node thông tin chung: <TTChung>
            var ttChung = FindElement(dlHDon, "TTChung") ?? FindElement(doc.Root, "TTChung");
            // Kiểm tra node nội dung hóa đơn: <NDHDon>
            var ndHDon = FindElement(dlHDon, "NDHDon") ?? FindElement(doc.Root, "NDHDon");

            // Nếu không có cả TTChung lẫn NDHDon thì file XML này không phải hóa đơn điện tử chuẩn
            if (ttChung == null && ndHDon == null)
            {
                return null;
            }

            var invoice = new InvoiceXmlModel
            {
                FilePath = filePath ?? string.Empty,
                FileName = string.IsNullOrEmpty(filePath) ? string.Empty : Path.GetFileName(filePath)
            };

            // =========================================================================
            // A. THÔNG TIN CHUNG TỜ HÓA ĐƠN (<DLHDon> -> <TTChung>)
            // =========================================================================
            if (ttChung != null)
            {
                invoice.KHMSHDon = GetElementValue(ttChung, "KHMSHDon");
                invoice.KHHDon = GetElementValue(ttChung, "KHHDon");
                invoice.SHDon = GetElementValue(ttChung, "SHDon");

                // Chuẩn hóa số hóa đơn (bỏ khoảng trắng)
                if (!string.IsNullOrEmpty(invoice.SHDon))
                {
                    invoice.SHDon = invoice.SHDon.Trim();
                }

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

            // =========================================================================
            // B. THÔNG TIN BÊN BÁN & BÊN MUA (<DLHDon> -> <NDHDon>)
            // =========================================================================
            var nBan = ndHDon != null ? FindElement(ndHDon, "NBan") : FindElement(doc.Root, "NBan");
            if (nBan != null)
            {
                invoice.NBanTen = GetElementValue(nBan, "Ten");
                invoice.NBanMST = GetElementValue(nBan, "MST");
                invoice.NBanDChi = GetElementValue(nBan, "DChi");
            }

            var nMua = ndHDon != null ? FindElement(ndHDon, "NMua") : FindElement(doc.Root, "NMua");
            if (nMua != null)
            {
                invoice.NMuaTen = GetElementValue(nMua, "Ten");
                invoice.NMuaMST = GetElementValue(nMua, "MST");
                invoice.NMuaDChi = GetElementValue(nMua, "DChi");
            }

            // =========================================================================
            // D. TỔNG TIỀN HÓA ĐƠN (<NDHDon> -> <TToan>)
            // =========================================================================
            var tToan = ndHDon != null ? FindElement(ndHDon, "TToan") : FindElement(doc.Root, "TToan");
            if (tToan != null)
            {
                invoice.TgTCThue = ParseDecimal(GetElementValue(tToan, "TgTCThue"));
                invoice.TgTThue = ParseDecimal(GetElementValue(tToan, "TgTThue"));
                invoice.TgTTTBSo = ParseDecimal(GetElementValue(tToan, "TgTTTBSo"));
                invoice.TgTTTBChu = GetElementValue(tToan, "TgTTTBChu");
            }

            // =========================================================================
            // C. CHI TIẾT TỪNG MẶT HÀNG / DỊCH VỤ (<NDHDon> -> <DSHHDVu> -> <HHDVu>)
            // =========================================================================
            var dsHHDVu = ndHDon != null ? FindElement(ndHDon, "DSHHDVu") : FindElement(doc.Root, "DSHHDVu");
            if (dsHHDVu != null)
            {
                var hhdVuList = FindElements(dsHHDVu, "HHDVu");
                int autoStt = 1;

                foreach (var itemElem in hhdVuList)
                {
                    var lineItem = new InvoiceXmlItemModel();

                    // 1. Tính chất dòng
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

                    // 3. Mã, Tên, ĐVT
                    lineItem.MHHDVu = GetElementValue(itemElem, "MHHDVu");
                    lineItem.THHDVu = GetElementValue(itemElem, "THHDVu");
                    lineItem.DVTinh = GetElementValue(itemElem, "DVTinh");

                    // 4. Số lượng, Đơn giá, Thành tiền (Data Fidelity - không tự ý làm tròn)
                    lineItem.SLuong = ParseDecimal(GetElementValue(itemElem, "SLuong"));
                    lineItem.DGia = ParseDecimal(GetElementValue(itemElem, "DGia"));
                    lineItem.ThTien = ParseDecimal(GetElementValue(itemElem, "ThTien"));

                    // 5. Thuế suất (Chuỗi gốc: "8%", "10%", "5%", "0%", "KCT", "KKKNT"...)
                    lineItem.TSuat = GetElementValue(itemElem, "TSuat");
                    lineItem.ThueSuatPhanTram = ParseTaxRate(lineItem.TSuat);

                    // 6. Tiền thuế dòng hàng (<TThue>)
                    string tThueStr = GetElementValue(itemElem, "TThue");
                    decimal? tThueVal = ParseDecimal(tThueStr);

                    // NGUYÊN TẮC BẢO TOÀN SỐ LIỆU GỐC:
                    // - Nếu thẻ <TThue> đã có giá trị: GIỮ NGUYÊN, không tự ý tính lại.
                    // - Chỉ khi <TThue> bị khuyết/null/rỗng mà thuế suất > 0: tạm gán TThue = ThTien * (TSuat / 100).
                    if (tThueVal.HasValue)
                    {
                        lineItem.TThue = tThueVal.Value;
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

            // Fallback tổng tiền nếu thẻ TToan bị thiếu nhưng có chi tiết hàng hóa
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

            return invoice;
        }

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

            // Xây dựng từ điển tra cứu cho danh sách existing theo key: normalize(ky_hieu) + "_" + normalize(so_hoa_don)
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

            // Khớp nối từng hóa đơn XML
            foreach (var inv in parsedInvoices)
            {
                string xmlKey = BuildMatchKey(inv.KHHDon, inv.SHDon);
                if (lookup.TryGetValue(xmlKey, out var matchedExt))
                {
                    inv.MatchedExistingInvoice = matchedExt;
                }
                else
                {
                    // Thử khớp nới lỏng (chỉ theo số hóa đơn nếu duy nhất)
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
            // Nếu toàn số 0 thì giữ lại "0"
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
        /// Ép kiểu số thập phân từ chuỗi XML (hỗ trợ dấu . và ,)
        /// </summary>
        public static decimal? ParseDecimal(string? str)
        {
            if (string.IsNullOrWhiteSpace(str)) return null;

            str = str.Trim();
            // Bỏ các ký tự tiền tệ nếu có
            str = Regex.Replace(str, @"[^\d,\.\-]", "");

            if (string.IsNullOrWhiteSpace(str)) return null;

            // Xử lý dấu phân cách: nếu có cả dấu . và ,
            if (str.Contains(',') && str.Contains('.'))
            {
                // Nếu dấu . xuất hiện trước dấu , -> dạng châu Âu/Việt Nam 1.000,50
                if (str.IndexOf('.') < str.IndexOf(','))
                {
                    str = str.Replace(".", "").Replace(',', '.');
                }
                else // Dạng Mỹ 1,000.50
                {
                    str = str.Replace(",", "");
                }
            }
            else if (str.Contains(','))
            {
                // Nếu chỉ có dấu phẩy -> chuyển thành dấu chấm thập phân
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

            // Nếu có ký tự T (ISO 8601), cắt lấy phần ngày nếu parse chuẩn
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
