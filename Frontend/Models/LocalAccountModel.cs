namespace Frontend.Models
{
    /// <summary>
    /// Lưu thông tin bản quyền và phiên làm việc cục bộ của CHÍNH tài khoản này trong SQLite.
    /// Giúp mở app siêu tốc (0ms) và chỉ kiểm tra ngầm với Google Sheet 1 lần / ngày.
    /// </summary>
    public class LocalAccountModel
    {
        public string Username { get; set; } = "";
        public string FullName { get; set; } = "";
        public string Role { get; set; } = "Staff";
        public int MaxTaxAccounts { get; set; } = 5;
        public bool AllowAI { get; set; } = false;
        public string EndDate { get; set; } = "";
        public string Status { get; set; } = "ACTIVE";
        public string PasswordHash { get; set; } = "";
        public string LastCheckedDate { get; set; } = "";
    }
}
