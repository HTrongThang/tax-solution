using Newtonsoft.Json;
using System;
using System.Collections.Generic;
using System.Linq;
using System.Text;
using System.Threading.Tasks;

namespace Frontend.Models
{
    public class StreamData
    {
        [JsonProperty("invoices")]
        public List<dynamic> Invoices { get; set; }

        [JsonProperty("current_page")]
        public int CurrentPage { get; set; }

        [JsonProperty("total_so_far")]
        public int TotalSoFar { get; set; }

        [JsonProperty("total_on_tax")]
        public int TotalOnTax { get; set; }

        [JsonProperty("status")]
        public string Status { get; set; }

        [JsonProperty("is_done")]
        public bool IsDone { get; set; }
    }
}
