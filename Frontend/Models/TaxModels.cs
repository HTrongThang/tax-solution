using System;
using System.Collections.Generic;
using System.Linq;
using System.Text;
using System.Threading.Tasks;

namespace Frontend.Models
{
    public class CrawlStatus2
    {
        public bool is_running { get; set; }
        public string current_status { get; set; }
        public int total_invoices { get; set; }
    }
}
