using System;
using System.Collections.Generic;
using System.Linq;
using System.Text;
using System.Threading.Tasks;

namespace Frontend.Models
{
    public static class UserSession
    {
        public static string FullName { get; set; }
        public static string Role { get; set; }

        /// <summary>
        /// Hàm tự động tạo chữ cái đại diện (ví dụ: Hà Trọng Thắng -> HT)
        /// </summary>
        public static string GetInitials()
        {
            if (string.IsNullOrWhiteSpace(FullName)) return "??";

            var parts = FullName.Trim().Split(new[] { ' ' }, StringSplitOptions.RemoveEmptyEntries);

            if (parts.Length > 1)
            {
                return (parts[0][0].ToString() + parts[parts.Length - 1][0].ToString()).ToUpper();
            }

            return parts[0][0].ToString().ToUpper();
        }
    }
}
