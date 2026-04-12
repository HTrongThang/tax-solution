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
using Frontend.Models;
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
        private void CoreWebView2_WebMessageReceived(object sender, CoreWebView2WebMessageReceivedEventArgs e)
        {
            // Lấy tin nhắn từ JavaScript gửi về
            string message = e.TryGetWebMessageAsString();

            if (message == "LOGOUT")
            {
                this.Invoke(new Action(() => {
                    this.Close();

                }));
            }
        }
        private void UserForm_FormClosed(object sender, FormClosedEventArgs e)
        {
            // Lệnh này sẽ quét sạch tất cả các Form đang ẩn và đóng toàn bộ App
            Application.Exit();
        }
    }
}
