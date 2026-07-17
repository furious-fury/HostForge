//go:build !linux && !windows

package services

import (
	"fmt"
	"runtime"
)

func availableFilesystemBytes(string) (int64, error) {
	return 0, fmt.Errorf("filesystem capacity inspection is unsupported on %s", runtime.GOOS)
}
