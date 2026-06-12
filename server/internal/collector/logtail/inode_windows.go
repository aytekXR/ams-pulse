//go:build windows

package logtail

import "os"

func inodeFromStat(fi os.FileInfo) uint64 {
	return 0
}
