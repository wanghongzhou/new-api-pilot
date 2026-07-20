//go:build !windows

package worker

import (
	"fmt"
	"math"

	"golang.org/x/sys/unix"
)

func exportFilesystemUsage(path string) (uint64, uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("read export filesystem capacity: %w", err)
	}
	blockSize := uint64(stat.Bsize)
	available := uint64(stat.Bavail)
	totalBlocks := uint64(stat.Blocks)
	if blockSize == 0 || available > math.MaxUint64/blockSize || totalBlocks > math.MaxUint64/blockSize {
		return 0, 0, fmt.Errorf("invalid export filesystem capacity")
	}
	freeBytes, totalBytes := available*blockSize, totalBlocks*blockSize
	if err := validateFilesystemUsage(freeBytes, totalBytes); err != nil {
		return 0, 0, err
	}
	return freeBytes, totalBytes, nil
}
