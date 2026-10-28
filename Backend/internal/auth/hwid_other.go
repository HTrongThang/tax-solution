//go:build !windows

package auth

// machineGUID: ngoài Windows không có MachineGuid, dùng tên máy (xem buildHardwareID).
func machineGUID() string { return "" }

