using Newtonsoft.Json;
using System;
using System.Collections.Generic;
using System.Linq;
using System.Text;
using System.Threading.Tasks;

namespace Frontend.Models
{
    public class CompanyModel
    {
        [JsonProperty("mst")]
        public string Mst { get; set; }

        [JsonProperty("company")]
        public string CompanyName { get; set; }

        [JsonProperty("pass")] 
        public string Password { get; set; }

        [JsonProperty("address")]
        public string Address { get; set; }

        [JsonProperty("taxDepartment")]
        public string TaxDepartment { get; set; }

        [JsonProperty("status")]
        public string Status { get; set; }

        [JsonProperty("orgType")]
        public string OrgType { get; set; }

        [JsonProperty("date")]
        public string Date { get; set; }
    }
}
