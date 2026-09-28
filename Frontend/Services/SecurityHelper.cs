using System;
using System.Security.Cryptography;
using System.Text;

namespace Frontend.Services
{
    public static class SecurityHelper
    {
        /// <summary>
        /// Mã hóa mật khẩu bằng Windows DPAPI (Data Protection API).
        /// Chỉ user Windows hiện tại trên máy tính này mới giải mã được.
        /// </summary>
        public static string ProtectPassword(string plainText)
        {
            if (string.IsNullOrEmpty(plainText)) return string.Empty;

            try
            {
                byte[] plainBytes = Encoding.UTF8.GetBytes(plainText);
                byte[] encryptedBytes = ProtectedData.Protect(plainBytes, null, DataProtectionScope.CurrentUser);
                return Convert.ToBase64String(encryptedBytes);
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("SecurityHelper.ProtectPassword error: " + ex.Message);
                return plainText; // Fallback nếu có lỗi
            }
        }

        /// <summary>
        /// Giải mã mật khẩu đã được bảo vệ bằng DPAPI.
        /// Nếu chuỗi không phải định dạng DPAPI (ví dụ mật khẩu cũ chưa mã hóa), trả về chuỗi gốc an toàn.
        /// </summary>
        public static string UnprotectPassword(string cipherText)
        {
            if (string.IsNullOrEmpty(cipherText)) return string.Empty;

            try
            {
                byte[] encryptedBytes = Convert.FromBase64String(cipherText);
                byte[] plainBytes = ProtectedData.Unprotect(encryptedBytes, null, DataProtectionScope.CurrentUser);
                return Encoding.UTF8.GetString(plainBytes);
            }
            catch
            {
                // Nếu chuỗi chưa được mã hóa trước đó, trả về chính chuỗi đó (backward-compatible)
                return cipherText;
            }
        }
    }
}
