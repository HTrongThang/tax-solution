using System;
using System.Collections.Generic;

namespace Frontend.Services
{
    /// <summary>
    /// Bộ nhớ RAM quản lý danh sách hóa đơn của công ty đang làm việc.
    /// Toàn bộ dữ liệu nằm trên RAM, tự động giải phóng khi đổi công ty, tìm kiếm mới hoặc đóng ứng dụng.
    /// Bất kỳ nơi nào trong ứng dụng chỉ cần gọi: InvoiceMemoryService.Instance.GetInvoices()
    /// </summary>
    public class InvoiceMemoryService
    {
        private static readonly Lazy<InvoiceMemoryService> _instance = new Lazy<InvoiceMemoryService>(() => new InvoiceMemoryService());
        public static InvoiceMemoryService Instance => _instance.Value;

        private readonly List<dynamic> _invoices = new List<dynamic>();
        private readonly object _lock = new object();

        public string? CurrentMst { get; private set; }
        public DateTime? LastUpdated { get; private set; }

        public int Count
        {
            get
            {
                lock (_lock)
                {
                    return _invoices.Count;
                }
            }
        }

        public bool HasData => Count > 0;

        /// <summary>
        /// 1. LƯU (SET): Nạp danh sách hóa đơn vào RAM cho công ty hiện tại.
        /// </summary>
        public void SetInvoices(string mst, IEnumerable<dynamic> invoices)
        {
            lock (_lock)
            {
                _invoices.Clear();
                if (invoices != null)
                {
                    _invoices.AddRange(invoices);
                }
                CurrentMst = mst;
                LastUpdated = DateTime.Now;
                System.Diagnostics.Debug.WriteLine($"[InvoiceMemoryService] Đã nạp {_invoices.Count} hóa đơn vào RAM (MST: {mst})");
            }
        }

        /// <summary>
        /// 2. LẤY (GET): Lấy toàn bộ danh sách hóa đơn từ RAM ra để hiển thị, lọc hoặc xuất Excel.
        /// </summary>
        public List<dynamic> GetInvoices()
        {
            lock (_lock)
            {
                return new List<dynamic>(_invoices);
            }
        }

        /// <summary>
        /// 3. XÓA (CLEAR): Giải phóng hoàn toàn bộ nhớ RAM.
        /// </summary>
        public void Clear(string? reason = null)
        {
            lock (_lock)
            {
                int prevCount = _invoices.Count;
                _invoices.Clear();
                CurrentMst = null;
                LastUpdated = null;
                System.Diagnostics.Debug.WriteLine($"[InvoiceMemoryService] Đã giải phóng RAM ({prevCount} hóa đơn). Lý do: {reason ?? "Không rõ"}");
            }
        }
    }
}
