using System;
using System.Collections.Generic;
using System.Linq;
using System.Text;
using System.Threading.Tasks;

namespace Frontend.Models
{
    public class InvoiceResponse
    {
        public string Status { get; set; }
        public string Message { get; set; }
        public List<dynamic> Data { get; set; } 
    }
}
