//go:build linux

package hostmetrics

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

// DefaultReader returns a Linux /proc + /sys backed reader.
func DefaultReader(opts ReaderOptions) Reader {
	return &procReader{opts: opts}
}

type procReader struct {
	opts ReaderOptions
}

func readFileTrim(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (p *procReader) ReadSnapshot() (*rawSnapshot, error) {
	now := time.Now().UTC()
	out := &rawSnapshot{At: now}

	stat, err := readFileTrim("/proc/stat")
	if err != nil {
		return nil, err
	}
	cpu, err := ParseCPUTimes(stat)
	if err != nil {
		return nil, err
	}
	out.CPU = cpu

	memStr, err := readFileTrim("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	mem, err := ParseMeminfo(memStr)
	if err != nil {
		return nil, err
	}
	out.Mem = mem

	netStr, err := readFileTrim("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	out.Net = filterNet(ParseNetDev(netStr), p.opts)

	dsStr, err := readFileTrim("/proc/diskstats")
	if err != nil {
		return nil, err
	}
	blocks, err := listBlockDevices()
	if err != nil {
		return nil, err
	}
	out.DiskIO = filterDiskIO(ParseDiskstats(dsStr), blocks)

	mi, err := readFileTrim("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	mounts := ParseMountinfo(mi)
	out.DiskUsage, err = statMounts(mounts, p.opts.DiskMountInclude)
	if err != nil {
		return nil, err
	}

	la, err := readFileTrim("/proc/loadavg")
	if err != nil {
		return nil, err
	}
	load, err := ParseLoadavg(la)
	if err != nil {
		return nil, err
	}
	out.Load = load

	up, err := readFileTrim("/proc/uptime")
	if err != nil {
		return nil, err
	}
	out.UptimeSecs, err = ParseUptime(up)
	if err != nil {
		return nil, err
	}

	return out, nil
}

func listBlockDevices() (map[string]struct{}, error) {
	ents, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{})
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
			continue
		}
		// Whole-disk entries live directly under /sys/block (partitions are nested).
		out[name] = struct{}{}
	}
	return out, nil
}

func filterDiskIO(all map[string]DiskIORaw, allowed map[string]struct{}) map[string]DiskIORaw {
	out := make(map[string]DiskIORaw)
	for name, v := range all {
		if _, ok := allowed[name]; ok {
			out[name] = v
		}
	}
	return out
}

func filterNet(all map[string]NetRaw, opts ReaderOptions) map[string]NetRaw {
	out := make(map[string]NetRaw)
	for iface, v := range all {
		if opts.NetExclude != nil && opts.NetExclude.MatchString(iface) {
			continue
		}
		if opts.NetInclude != nil && !opts.NetInclude.MatchString(iface) {
			continue
		}
		out[iface] = v
	}
	return out
}

func statMounts(mounts []MountEntry, include *regexp.Regexp) ([]DiskUsage, error) {
	seen := make(map[string]struct{})
	var paths []string
	for _, m := range mounts {
		if include != nil && !include.MatchString(m.Mount) {
			continue
		}
		if _, ok := seen[m.Mount]; ok {
			continue
		}
		seen[m.Mount] = struct{}{}
		paths = append(paths, m.Mount)
	}
	sort.Strings(paths)

	var usage []DiskUsage
	for _, mount := range paths {
		var st syscall.Statfs_t
		err := syscall.Statfs(mount, &st)
		if err != nil {
			continue
		}
		bs := int64(st.Bsize)
		total := int64(st.Blocks) * bs
		avail := int64(st.Bavail) * bs
		if total <= 0 {
			continue
		}
		used := total - avail
		if used < 0 {
			used = 0
		}
		pct := float64(used) / float64(total) * 100
		fstype := ""
		for _, me := range mounts {
			if me.Mount == mount {
				fstype = me.FSType
				break
			}
		}
		usage = append(usage, DiskUsage{
			Mount:      mount,
			FSType:     fstype,
			TotalBytes: total,
			UsedBytes:  used,
			AvailBytes: avail,
			UsedPct:    pct,
		})
	}
	return usage, nil
}
