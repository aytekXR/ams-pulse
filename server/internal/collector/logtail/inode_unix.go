//go:build !windows

package logtail

import (
	"os"
	"syscall"
)

func inodeFromStat(fi os.FileInfo) uint64 {
	if fi == nil {
		return 0
	}
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
		return stat.Ino
	}
	return 0
}
