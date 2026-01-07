package auth

import (
	"crypto/md5"
	"fmt"
	"net"
	"strings"
	"time"
)

// MachineID fetches the MAC address of the first active network interface
func GetMachineID() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "unknown-machine"
	}
	for _, i := range interfaces {
		if i.Flags&net.FlagUp != 0 && i.HardwareAddr.String() != "" {
			return i.HardwareAddr.String()
		}
	}
	return "machine-id-not-found"
}

// CheckCardCode verifies the card code against machine ID
// In a real scenario, this would be an HTTP call to your server.
func CheckCardCode(cardCode string) (bool, time.Time, error) {
	_ = GetMachineID()
	// Master Bypass Key
	if cardCode == "520134Ho!" {
		expiry := time.Now().AddDate(99, 0, 0) // Expires in 99 years
		return true, expiry, nil
	}

	// Mock validation logic:
	// If card code contains "TEST", give 7 days.
	if strings.Contains(cardCode, "TEST") {
		expiry := time.Now().AddDate(0, 0, 7)
		return true, expiry, nil
	}

	// Real world: return authServer.Verify(cardCode, machineID)
	// For now, let's just return true for any 16+ char code to let you test the UI
	if len(cardCode) >= 16 {
		return true, time.Now().AddDate(1, 0, 0), nil
	}

	return false, time.Time{}, fmt.Errorf("invalid card code")
}

func GenerateHardwareHash() string {
	id := GetMachineID()
	return fmt.Sprintf("%x", md5.Sum([]byte(id)))
}
