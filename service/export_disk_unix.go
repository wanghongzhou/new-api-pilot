//go:build !windows

package service

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func availableExportDiskBytes(path string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("read export disk free space: %w", err)
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}
