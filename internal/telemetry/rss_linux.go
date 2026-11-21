//go:build linux

package telemetry

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func PeakRSS() (uint64, error) {
	b, e := os.ReadFile("/proc/self/status")
	if e != nil {
		return 0, e
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "VmHWM:" {
			n, e := strconv.ParseUint(fields[1], 10, 64)
			return n * 1024, e
		}
	}
	return 0, fmt.Errorf("VmHWM unavailable in /proc/self/status")
}
