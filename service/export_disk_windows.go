//go:build windows

package service

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func availableExportDiskBytes(path string) (uint64, error) {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var free uint64
	if err := windows.GetDiskFreeSpaceEx(pointer, &free, nil, nil); err != nil {
		return 0, fmt.Errorf("read export disk free space: %w", err)
	}
	return free, nil
}
