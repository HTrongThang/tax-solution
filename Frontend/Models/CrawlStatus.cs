using System;
using System.Collections.Generic;
using System.Linq;
using System.Text;
using System.Threading.Tasks;

namespace Frontend.Models
{
    public class CrawlStatus
    {
        // Khớp với struct bên Go (json:"current_status")
        public string current_status { get; set; }

        // Khớp với struct bên Go (json:"is_running")
        public bool is_running { get; set; }

        // Khớp với struct bên Go (json:"total_invoices")
        public int total_invoices { get; set; }
    }
}
