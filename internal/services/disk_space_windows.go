//go:build windows

package services

import "golang.org/x/sys/windows"

func availableFilesystemBytes(path string) (int64, error) {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var available uint64
	if err := windows.GetDiskFreeSpaceEx(pathPointer, &available, nil, nil); err != nil {
		return 0, err
	}
	const maxInt64 = uint64(^uint64(0) >> 1)
	if available > maxInt64 {
		return int64(maxInt64), nil
	}
	return int64(available), nil
}
