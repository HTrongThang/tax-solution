using Frontend.Models;
using Newtonsoft.Json;
using System;
using System.Collections.Generic;
using System.Linq;
using System.Net.Http.Json;
using System.Reflection;
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

        public async Task SearchInvoicesSSE(string fromDate, string toDate, Action<StreamData> onProgress)
        {
            try
            {
                DateTime startDate = DateTime.Parse(fromDate);
                DateTime endDate = DateTime.Parse(toDate);

                string fDate = startDate.ToString("dd/MM/yyyy");
                string tDate = endDate.ToString("dd/MM/yyyy");

                // Sử dụng HttpCompletionOption.ResponseHeadersRead để nhận dữ liệu ngay khi server vừa gửi header
                using (var response = await _client.GetAsync($"/api/tax/searchInvoice?from={fDate}&to={tDate}", HttpCompletionOption.ResponseHeadersRead))
                {
                    using (var stream = await response.Content.ReadAsStreamAsync())
                    using (var reader = new StreamReader(stream))
                    {
                        while (!reader.EndOfStream)
                        {
                            string line = await reader.ReadLineAsync();

                            // SSE gửi dữ liệu bắt đầu bằng "data: "
                            if (string.IsNullOrWhiteSpace(line) || !line.StartsWith("data: ")) continue;

                            string json = line.Substring(6).Trim();
                            var data = JsonConvert.DeserializeObject<StreamData>(json);

                            if (data != null)
                            {
                                onProgress?.Invoke(data); // Gửi về UI
                            }

                            if (data?.IsDone == true) break;
                        }
                    }
                }
            }
            catch (Exception ex)
            {
                onProgress?.Invoke(new StreamData { Status = "Lỗi kết nối: " + ex.Message, IsDone = true });
            }
        }
    }
}
