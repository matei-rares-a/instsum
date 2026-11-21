//go:build !windows && !linux

package telemetry

import "fmt"

func PeakRSS() (uint64, error) {
	return 0, fmt.Errorf("peak RSS collection supports Windows and Linux only")
}
