using System;
using System.Reflection;

namespace Frontend.Services
{
    public static class AppVersionHelper
    {
        private static readonly Version _defaultVersion = new Version(1, 0, 0);

        /// <summary>
        /// Phiên bản dạng đối tượng Version (để so sánh)
        /// </summary>
        public static Version VersionObj
        {
            get
            {
                var asmVer = Assembly.GetExecutingAssembly().GetName().Version;
                return asmVer != null ? new Version(asmVer.Major, asmVer.Minor, Math.Max(0, asmVer.Build)) : _defaultVersion;
            }
        }

        /// <summary>
        /// Chuỗi phiên bản ngắn gọn, ví dụ: "1.0.0"
        /// </summary>
        public static string CurrentVersion => $"{VersionObj.Major}.{VersionObj.Minor}.{VersionObj.Build}";

        /// <summary>
        /// Chuỗi phiên bản hiển thị kèm chữ v, ví dụ: "v1.0.0"
        /// </summary>
        public static string DisplayVersion => $"v{CurrentVersion}";
    }
}
