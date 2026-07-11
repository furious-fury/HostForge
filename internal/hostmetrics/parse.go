package hostmetrics

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// CPUTimes holds jiffies from /proc/stat for one CPU line.
type CPUTimes struct {
	User, Nice, System, Idle, IOWait, IRQ, SoftIRQ, Steal uint64
}

// TotalJiffies returns the sum of fields used for utilization.
func TotalJiffies(c CPUTimes) uint64 {
	return c.User + c.Nice + c.System + c.Idle + c.IOWait + c.IRQ + c.SoftIRQ + c.Steal
}

// BusyJiffies returns non-idle jiffies.
func BusyJiffies(c CPUTimes) uint64 {
	return c.User + c.Nice + c.System + c.IRQ + c.SoftIRQ + c.Steal
}

// Meminfo holds parsed /proc/meminfo values in kB (as in the file).
type Meminfo struct {
	MemTotal     uint64
	MemAvailable uint64
	MemFree      uint64
	Buffers      uint64
	Cached       uint64
	SwapTotal    uint64
	SwapFree     uint64
}

// NetRaw holds cumulative counters from /proc/net/dev.
type NetRaw struct {
	RxBytes uint64
	TxBytes uint64
}

// DiskIORaw holds cumulative counters from /proc/diskstats.
type DiskIORaw struct {
	ReadSectors  uint64
	WriteSectors uint64
	MSDoingIO    uint64
}

// ParseCPUTimes parses /proc/stat content; first entry is aggregate cpu, rest are per-core cpuN.
func ParseCPUTimes(stat string) ([]CPUTimes, error) {
	sc := bufio.NewScanner(strings.NewReader(stat))
	var out []CPUTimes
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "cpu") {
			break
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		// cpu, cpu0, cpu1 — skip name
		name := fields[0]
		if name != "cpu" && !strings.HasPrefix(name, "cpu") {
			continue
		}
		nums := fields[1:]
		if len(nums) < 8 {
			return nil, fmt.Errorf("hostmetrics: short cpu line %q", line)
		}
		var c CPUTimes
		var err error
		c.User, err = strconv.ParseUint(nums[0], 10, 64)
		if err != nil {
			return nil, err
		}
		c.Nice, _ = strconv.ParseUint(nums[1], 10, 64)
		c.System, _ = strconv.ParseUint(nums[2], 10, 64)
		c.Idle, _ = strconv.ParseUint(nums[3], 10, 64)
		c.IOWait, _ = strconv.ParseUint(nums[4], 10, 64)
		c.IRQ, _ = strconv.ParseUint(nums[5], 10, 64)
		c.SoftIRQ, _ = strconv.ParseUint(nums[6], 10, 64)
		c.Steal, _ = strconv.ParseUint(nums[7], 10, 64)
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("hostmetrics: no cpu lines in stat")
	}
	return out, sc.Err()
}

// ParseMeminfo parses /proc/meminfo into kB fields.
func ParseMeminfo(s string) (Meminfo, error) {
	var m Meminfo
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := sc.Text()
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		rest := strings.TrimSpace(line[idx+1:])
		fields := strings.Fields(rest)
		if len(fields) < 1 {
			continue
		}
		v, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			m.MemTotal = v
		case "MemAvailable":
			m.MemAvailable = v
		case "MemFree":
			m.MemFree = v
		case "Buffers":
			m.Buffers = v
		case "Cached":
			m.Cached = v
		case "SwapTotal":
			m.SwapTotal = v
		case "SwapFree":
			m.SwapFree = v
		}
	}
	if m.MemTotal == 0 {
		return m, fmt.Errorf("hostmetrics: MemTotal missing")
	}
	return m, sc.Err()
}

// MemSampleFromMeminfo builds MemSample from meminfo (kB) with bytes and used percent.
func MemSampleFromMeminfo(m Meminfo) MemSample {
	kb := uint64(1024)
	total := int64(m.MemTotal * kb)
	avail := int64(m.MemAvailable * kb)
	if m.MemAvailable == 0 {
		// Fallback: Linux without MemAvailable (ancient)
		free := int64(m.MemFree+m.Buffers+m.Cached) * int64(kb)
		avail = free
	}
	used := total - avail
	if used < 0 {
		used = 0
	}
	var pct float64
	if total > 0 {
		pct = float64(used) / float64(total) * 100
	}
	swapTotal := int64(m.SwapTotal * kb)
	swapUsed := int64((m.SwapTotal - m.SwapFree) * kb)
	if swapUsed < 0 {
		swapUsed = 0
	}
	bc := int64((m.Buffers + m.Cached) * kb)
	return MemSample{
		TotalBytes:         total,
		UsedBytes:          used,
		AvailableBytes:     avail,
		BuffersCachedBytes: bc,
		SwapTotalBytes:     swapTotal,
		SwapUsedBytes:      swapUsed,
		UsedPct:            pct,
	}
}

// ParseNetDev parses /proc/net/dev into per-interface counters (bytes only).
func ParseNetDev(s string) map[string]NetRaw {
	out := make(map[string]NetRaw)
	sc := bufio.NewScanner(strings.NewReader(s))
	lineNo := 0
	for sc.Scan() {
		line := sc.Text()
		lineNo++
		if lineNo <= 2 {
			continue // headers
		}
		if idx := strings.IndexByte(line, ':'); idx >= 0 {
			iface := strings.TrimSpace(line[:idx])
			rest := strings.Fields(line[idx+1:])
			if len(rest) < 16 {
				continue
			}
			rx, err1 := strconv.ParseUint(rest[0], 10, 64)
			tx, err2 := strconv.ParseUint(rest[8], 10, 64)
			if err1 != nil || err2 != nil {
				continue
			}
			out[iface] = NetRaw{RxBytes: rx, TxBytes: tx}
		}
	}
	return out
}

// ParseDiskstatsLine parses one /proc/diskstats line (kernel 4+ field layout).
func ParseDiskstatsLine(line string) (name string, raw DiskIORaw, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 14 {
		return "", raw, false
	}
	name = fields[2]
	readSec, err1 := strconv.ParseUint(fields[5], 10, 64)
	writeSec, err2 := strconv.ParseUint(fields[9], 10, 64)
	msIO, err3 := strconv.ParseUint(fields[12], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return "", raw, false
	}
	return name, DiskIORaw{ReadSectors: readSec, WriteSectors: writeSec, MSDoingIO: msIO}, true
}

// ParseDiskstats parses full /proc/diskstats.
func ParseDiskstats(s string) map[string]DiskIORaw {
	out := make(map[string]DiskIORaw)
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		n, r, ok := ParseDiskstatsLine(sc.Text())
		if !ok {
			continue
		}
		out[n] = r
	}
	return out
}

// ParseLoadavg parses /proc/loadavg first three floats.
func ParseLoadavg(s string) ([3]float64, error) {
	var out [3]float64
	fields := strings.Fields(s)
	if len(fields) < 3 {
		return out, fmt.Errorf("hostmetrics: short loadavg")
	}
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return out, err
		}
		out[i] = v
	}
	return out, nil
}

// ParseUptime parses /proc/uptime first field (seconds).
func ParseUptime(s string) (float64, error) {
	fields := strings.Fields(s)
	if len(fields) < 1 {
		return 0, fmt.Errorf("hostmetrics: short uptime")
	}
	return strconv.ParseFloat(fields[0], 64)
}

const sectorBytes = 512

// physicalFSTypes allowed for disk usage from mountinfo.
var physicalFSTypes = map[string]struct{}{
	"ext4": {}, "xfs": {}, "btrfs": {}, "zfs": {}, "f2fs": {}, "vfat": {}, "ntfs": {},
}

// MountEntry is one filtered row from /proc/self/mountinfo.
type MountEntry struct {
	Mount  string
	FSType string
}

// ParseMountinfo returns physical mounts (path + fstype) from mountinfo content.
func ParseMountinfo(content string) []MountEntry {
	var out []MountEntry
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := sc.Text()
		parts := strings.Split(line, " - ")
		if len(parts) != 2 {
			continue
		}
		left := strings.Fields(parts[0])
		if len(left) < 5 {
			continue
		}
		mount := unescapeMountinfoPath(left[4])
		right := strings.Fields(parts[1])
		if len(right) < 1 {
			continue
		}
		fstype := right[0]
		if _, ok := physicalFSTypes[fstype]; !ok {
			continue
		}
		out = append(out, MountEntry{Mount: mount, FSType: fstype})
	}
	return out
}

func unescapeMountinfoPath(s string) string {
	return strings.ReplaceAll(s, `\040`, " ")
}
