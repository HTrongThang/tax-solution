using Frontend.Models;
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
            };

            public async Task<string> Init() => await _client.GetStringAsync("/api/tax/init");

            public async Task<string> GetCaptchaBase64()
            {
                var result = await _client.GetFromJsonAsync<dynamic>("/api/tax/captcha");
                return result?.data;
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
    }
}
