using Frontend.Models;
using Newtonsoft.Json;
using System;
using System.Collections.Generic;
using System.Linq;
using System.Net.Http.Json;
using System.Text;
using System.Threading.Tasks;
namespace Frontend.Services
{
    internal class TaxService
    {
            private readonly HttpClient _client = new HttpClient
            {
                BaseAddress = new Uri("http://localhost:8080")
                ,
                Timeout = System.Threading.Timeout.InfiniteTimeSpan
            };

            public async Task<string> Init() => await _client.GetStringAsync("/api/tax/init");

        public async Task<string> GetCaptchaBase64()
        {
            try
            {
                // 1. Gọi API
                var response = await _client.GetAsync("/api/tax/captcha");

                // Đảm bảo request thành công (200 OK)
                response.EnsureSuccessStatusCode();

                var content = await response.Content.ReadAsStringAsync();

                // 2. Parse dùng Newtonsoft (Ép kiểu về dynamic để dùng dấu chấm)
                dynamic result = JsonConvert.DeserializeObject<dynamic>(content);

                // 3. Kiểm tra và trả về (Lưu ý: Go trả về Status viết hoa hay thường thì C# phải viết y hệt)
                if (result != null && (result.status == "success" || result.Status == "success"))
                {
                    // Trả về trường Data (viết hoa chữ D nếu Go định nghĩa vậy)
                    return result.data ?? result.Data;
                }
                else
                {
                    string msg = result?.message ?? "Lỗi không xác định từ Backend";
                    throw new Exception(msg);
                }
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("Lỗi GetCaptcha: " + ex.Message);
                throw; // Đẩy lỗi ra ngoài để Form hiện Dialog
            }
        }

        public async Task<CrawlStatus> GetStatus()
            {
                try
                {
                    return await _client.GetFromJsonAsync<CrawlStatus>("/api/tax/status");
                }
                catch
                {
                    return new CrawlStatus { current_status = "Đang chờ Server..." };
                }
            }

        public async Task<(string Status, string Message)> LoginTax(string taxCode, string password, string captcha)
        {
            try
            {
                var loginData = new { tax_code = taxCode, Password = password, Captcha = captcha };
                var content = new StringContent(JsonConvert.SerializeObject(loginData), Encoding.UTF8, "application/json");

                // Gọi API Login
                var response = await _client.PostAsync("/api/tax/login", content);
                var responseBody = await response.Content.ReadAsStringAsync();

                // Parse kết quả dùng Newtonsoft.Json
                dynamic result = JsonConvert.DeserializeObject(responseBody);

                // Trả về cả Status (để check error_captcha) và Message (để hiển thị)
                return (result?.status?.ToString() ?? "error", result?.message?.ToString() ?? "Lỗi không xác định");
            }
            catch (Exception ex)
            {
                return ("error", "Không thể kết nối, hãy khởi động lại ứng dụng: " + ex.Message);
            }
        }
        public async Task<(string status, string message)> LogoutTaxAsync()
        {
            try
            {
                // 1. Khởi tạo client (Nếu Thắng đã có HttpClient dùng chung thì dùng luôn)
                using (var client = new HttpClient())
                {
                    // Thiết lập Timeout để tránh treo app nếu Go bị lỗi
                    client.Timeout = TimeSpan.FromSeconds(10);

                    // 2. Gọi API POST tới Go (Nhớ kiểm tra đúng Port của Go)
                    var response = await client.PostAsync("http://localhost:8080/api/tax/logout", null);

                    // 3. Đọc nội dung trả về
                    var content = await response.Content.ReadAsStringAsync();

                    if (response.IsSuccessStatusCode)
                    {
                        // Parse JSON bằng Newtonsoft.Json
                        dynamic result = JsonConvert.DeserializeObject(content);
                        return (result.status.ToString(), result.message.ToString());
                    }
                    else
                    {
                        return ("error", $"Server báo lỗi: {response.StatusCode}");
                    }
                }
            }
            catch (Exception ex)
            {
                // Bẫy lỗi nếu Go chưa chạy hoặc lỗi mạng
                return ("error", "Không thể kết nối tới Backend Go: " + ex.Message);
            }
        }

        public async Task<(string Status, string Message, List<dynamic> Invoices)> SearchInvoices(string fromDate, string toDate)
        {
            try
            {
                DateTime start = DateTime.Parse(fromDate);
                DateTime end = DateTime.Parse(toDate);

                string fDate = start.ToString("dd/MM/yyyy");
                string tDate = end.ToString("dd/MM/yyyy");
                // 2. Gọi API GET sang Go với Query Params
                // URL mẫu: http://localhost:8080/api/tax/searchInvoice?from=01/04/2026&to=13/04/2026
                var response = await _client.GetAsync($"/api/tax/searchInvoice?from={fDate}&to={tDate}");
                var content = await response.Content.ReadAsStringAsync();

                // 3. Parse kết quả dùng Newtonsoft.Json
                dynamic result = JsonConvert.DeserializeObject(content);

                if (response.IsSuccessStatusCode && (result?.status == "success" || result?.Status == "success"))
                {
                    // Ép kiểu Data về List<dynamic> để WinForms dễ dùng
                    var invoiceList = result?.data != null ? result.data.ToObject<List<dynamic>>() : new List<dynamic>();
                    return ("success", result?.message?.ToString() ?? "Thành công", invoiceList);
                }
                else
                {
                    return ("error", result?.message?.ToString() ?? "Lỗi tra cứu hóa đơn", null);
                }
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("Lỗi SearchInvoices: " + ex.Message);
                return ("error", "Lỗi kết nối API Tra cứu: " + ex.Message, null);
            }

        }

    }
}
