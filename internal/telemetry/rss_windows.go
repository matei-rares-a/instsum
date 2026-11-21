//go:build windows

package telemetry

import (
	"fmt"
	"syscall"
	"unsafe"
)

type memoryCounters struct {
	Size             uint32
	PageFaults       uint32
	PeakWorkingSet   uintptr
	WorkingSet       uintptr
	PeakPagedPool    uintptr
	PagedPool        uintptr
	PeakNonPagedPool uintptr
	NonPagedPool     uintptr
	PageFile         uintptr
	PeakPageFile     uintptr
}

func PeakRSS() (uint64, error) {
	var counters memoryCounters
	counters.Size = uint32(unsafe.Sizeof(counters))
	proc := syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
	ok, _, err := proc.Call(^uintptr(0), uintptr(unsafe.Pointer(&counters)), uintptr(counters.Size))
	if ok == 0 {
		return 0, fmt.Errorf("GetProcessMemoryInfo: %w", err)
	}
	return uint64(counters.PeakWorkingSet), nil
}
