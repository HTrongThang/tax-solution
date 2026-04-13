using Frontend.Models;
using Microsoft.Web.WebView2.Core;
using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Data;
using System.Drawing;
using System.Linq;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;
using System.Xml;
using Newtonsoft.Json;
using System.IO;
using Formatting = Newtonsoft.Json.Formatting;
namespace Frontend.Views
{
    public partial class UserForm : Form
    {
        public UserForm()
        {
            InitializeComponent();
            this.Load += UserForm_Load;
            this.FormClosed += UserForm_FormClosed;
        }
        private async void UserForm_Load(object sender, EventArgs e)
        {
            this.Opacity = 0;

            // 1. Căn chỉnh kích thước
            Rectangle screenRect = Screen.PrimaryScreen.WorkingArea;
            this.Size = new Size((int)(screenRect.Width * 0.85), (int)(screenRect.Height * 0.85));
            this.StartPosition = FormStartPosition.Manual; // Chỉnh lại để CenterScreen ăn theo Location
            this.Location = new Point((screenRect.Width - this.Width) / 2, (screenRect.Height - this.Height) / 2);

            // 2. Khởi tạo WebView2
            await webViewUser.EnsureCoreWebView2Async();

            // 3. Cấu hình Mapping (Đảm bảo folder WebResources tồn tại)
            string webResourcesPath = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "WebResources");

            webViewUser.CoreWebView2.SetVirtualHostNameToFolderMapping(
                "taxscraper.app",
                webResourcesPath,
                CoreWebView2HostResourceAccessKind.Allow
            );

            // 4. ĐĂNG KÝ SỰ KIỆN TRƯỚC KHI NAVIGATE
            webViewUser.CoreWebView2.WebMessageReceived += CoreWebView2_WebMessageReceived;

            webViewUser.NavigationCompleted += async (s, args) => {
                if (args.IsSuccess)
                {
                    await SyncHeaderWithSession();
                    this.Opacity = 1; // Chỉ hiện khi đã nạp xong và đồng bộ header
                }
                else
                {
                    MessageBox.Show($"Lỗi nạp trang: {args.WebErrorStatus}", "Lỗi hệ thống");
                    this.Opacity = 1; // Vẫn hiện để người dùng thấy lỗi mà xử lý
                }
            };

            // 5. ĐIỀU HƯỚNG (Kiểm tra kỹ chữ User viết hoa/thường)
            // Nếu trong folder là "User" thì giữ nguyên, nếu là "user" thì phải sửa lại
            webViewUser.CoreWebView2.Navigate("https://taxscraper.app/User/dashboard.html");
        }
        private async Task SyncHeaderWithSession()
        {
            // 1. Lấy thông tin từ Session
            string name = UserSession.FullName ?? "Người dùng";
            string role = UserSession.Role ?? "Nhân viên";
            string initials = UserSession.GetInitials(); // Ví dụ: "TT"

            // 2. JavaScript để cập nhật đúng các ID trong file dashboard.html của Thắng
            string jsCode = $@"
                (function() {{
                    const nameEl = document.getElementById('header_user_name');
                    const roleEl = document.getElementById('header_user_role');
                    const avatarEl = document.getElementById('header_user_avatar');

                    if(nameEl) nameEl.innerText = '{name}';
                    if(roleEl) roleEl.innerText = '{role}';
                    if(avatarEl) avatarEl.innerText = '{initials}';
                }})();";

            await webViewUser.CoreWebView2.ExecuteScriptAsync(jsCode);
        }
        private async void CoreWebView2_WebMessageReceived(object sender, CoreWebView2WebMessageReceivedEventArgs e)
        {
            // 1. Lấy dữ liệu dưới dạng JSON
            string jsonRaw = e.WebMessageAsJson;

            // 2. Parse thành đối tượng dynamic để truy cập nhanh các thuộc tính
            dynamic packet = JsonConvert.DeserializeObject(jsonRaw);
            if (packet == null) return;

            string action = packet.action;
            string filePath = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "tax_accounts.json");

            switch (action)
            {
                case "LOGOUT":
                    this.Invoke(new Action(() => {
                        this.Close();
                    }));
                    break;

                case "ADD_AND_SAVE_MST":
                    try
                    {
                        // 1. Kiểm tra dữ liệu đầu vào (phòng hờ lỗi từ JS)
                        if (packet.data == null || packet.data.mst == null)
                        {
                            MessageBox.Show("Dữ liệu không hợp lệ!", "Lỗi", MessageBoxButtons.OK, MessageBoxIcon.Warning);
                            break;
                        }

                        List<dynamic> listAdd = new List<dynamic>();
                        if (File.Exists(filePath))
                        {
                            string content = File.ReadAllText(filePath);
                            listAdd = JsonConvert.DeserializeObject<List<dynamic>>(content) ?? new List<dynamic>();
                        }

                        // 2. Thêm dữ liệu mới
                        listAdd.Add(packet.data);

                        // 3. Thực hiện ghi file
                        string jsonOutput = JsonConvert.SerializeObject(listAdd, Newtonsoft.Json.Formatting.Indented);
                        File.WriteAllText(filePath, jsonOutput);

                        // 4. CHỈ THÔNG BÁO THÀNH CÔNG KHI MỌI THỨ XONG XUÔI
                           MessageBox.Show(
                             "Hệ thống đã ghi dữ liệu thành công!",
                             "TaxSolution - Thông báo hệ thống",
                             MessageBoxButtons.OK,
                             MessageBoxIcon.Information
                         );
                    }
                    catch (Exception ex)
                    {
                        // 1. Ghi log lỗi ra cửa sổ Output của Visual Studio để Thắng dễ debug
                        System.Diagnostics.Debug.WriteLine("Lỗi lưu file: " + ex.Message);

                        // 2. Kiểm tra nếu WebView2 sẵn sàng thì hiện Modal đẹp của Thắng
                        if (webViewUser != null && webViewUser.CoreWebView2 != null)
                        {
                            string errorMsg = ex.Message.Replace("'", "").Replace("\n", " ");
                            await webViewUser.CoreWebView2.ExecuteScriptAsync(
                                $"window.showAlert('Lỗi hệ thống', 'Không thể ghi file JSON: {errorMsg}', 'error')"
                            );
                        }
                        else
                        {
                            // 3. Nếu WebView2 chưa sẵn sàng (đang bị Null) thì dùng MessageBox chữa cháy
                            MessageBox.Show(
                                $"Lỗi nghiêm trọng: {ex.Message}",
                                "TaxSolution - Lỗi ghi file",
                                MessageBoxButtons.OK,
                                MessageBoxIcon.Error
                            );
                        }
                    }
                    break;

                case "DELETE_AND_SAVE_MST":
                    if (File.Exists(filePath))
                    {
                        try
                        {
                            string content = File.ReadAllText(filePath);
                            var listDelete = JsonConvert.DeserializeObject<List<dynamic>>(content) ?? new List<dynamic>();

                            string mstToDelete = packet.mst;
                            // Dùng ToList() để thực hiện lọc
                            var filteredList = listDelete.Where(x => (string)x.mst != mstToDelete).ToList();

                            File.WriteAllText(filePath, JsonConvert.SerializeObject(filteredList, Formatting.Indented));
                        }
                        catch (Exception ex)
                        {
                            System.Diagnostics.Debug.WriteLine("Lỗi xóa: " + ex.Message);
                        }
                    }
                    break;
                case "GET_SETTINGS_DATA":
                    await SendJsonDataToWeb(filePath);
                    break;
            }
        }
        private async Task SendJsonDataToWeb(string filePath)
        {
            try
            {
                if (File.Exists(filePath))
                {
                    string jsonData = File.ReadAllText(filePath);
                    string safeJson = JsonConvert.ToString(jsonData);
                    await webViewUser.CoreWebView2.ExecuteScriptAsync($"window.loadInitialData({safeJson})");
                }
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("Lỗi bắn dữ liệu: " + ex.Message);
            }
        }
        private void UserForm_FormClosed(object sender, FormClosedEventArgs e)
        {
            // Lệnh này sẽ quét sạch tất cả các Form đang ẩn và đóng toàn bộ App
            Application.Exit();
        }
    }
}
