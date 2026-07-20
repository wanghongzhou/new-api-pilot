//go:build windows

package worker

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func exportFilesystemUsage(path string) (uint64, uint64, error) {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, fmt.Errorf("read export filesystem capacity: %w", err)
	}
	var freeBytes, totalBytes uint64
	if err := windows.GetDiskFreeSpaceEx(pointer, &freeBytes, &totalBytes, nil); err != nil {
		return 0, 0, fmt.Errorf("read export filesystem capacity: %w", err)
	}
	if err := validateFilesystemUsage(freeBytes, totalBytes); err != nil {
		return 0, 0, err
	}
	return freeBytes, totalBytes, nil
}
