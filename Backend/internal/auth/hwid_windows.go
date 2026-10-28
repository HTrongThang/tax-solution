//go:build windows

package auth

import "golang.org/x/sys/windows/registry"

// machineGUID đọc HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid.
func machineGUID() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`,
		registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return ""
	}
	defer k.Close()

	v, _, err := k.GetStringValue("MachineGuid")
	if err != nil {
		return ""
	}
	return v
}

