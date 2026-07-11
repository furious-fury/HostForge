package hostmetrics

import "sort"

// ResolveHostRootDisk returns usage for "/" if present, else first mount by path order.
func ResolveHostRootDisk(disks []DiskUsage) *DiskUsage {
	for i := range disks {
		if disks[i].Mount == "/" {
			return &disks[i]
		}
	}
	if len(disks) == 0 {
		return nil
	}
	return &disks[0]
}

// TopDiskIOByRate returns up to n devices sorted by read+write bytes/sec descending.
func TopDiskIOByRate(io []DiskIOSample, n int) []DiskIOSample {
	if n <= 0 || len(io) == 0 {
		return nil
	}
	cp := append([]DiskIOSample(nil), io...)
	sort.Slice(cp, func(i, j int) bool {
		ai := cp[i].ReadBps + cp[i].WriteBps
		aj := cp[j].ReadBps + cp[j].WriteBps
		return ai > aj
	})
	if len(cp) > n {
		cp = cp[:n]
	}
	return cp
}
