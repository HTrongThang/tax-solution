package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
)

var (
	hwidOnce  sync.Once
	hwidValue string
)

// HardwareID trả về Mã máy ổn định, dạng TS-XXXX-XXXX-XXXX, tạo từ MachineGuid của Windows
// (hoặc tên máy nếu không đọc được). Cùng một máy luôn ra cùng một mã.
func HardwareID() string {
	hwidOnce.Do(func() {
		hwidValue = buildHardwareID(machineGUID())
	})
	return hwidValue
}

func buildHardwareID(guid string) string {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		host, _ := os.Hostname()
		guid = "host:" + host
	}
	sum := sha256.Sum256([]byte("TaxSolution|" + guid))
	h := strings.ToUpper(hex.EncodeToString(sum[:]))
	return fmt.Sprintf("TS-%s-%s-%s", h[0:4], h[4:8], h[8:12])
}

