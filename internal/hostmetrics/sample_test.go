package hostmetrics

import (
	"math"
	"testing"
	"time"
)

func TestParseCPUTimesAndCPUUtil(t *testing.T) {
	stat := `cpu  100 200 300 400 50 10 20 5 0 0
cpu0 50 100 150 200 25 5 10 2 0 0
cpu1 50 100 150 200 25 5 10 3 0 0
intr 123
`
	_, err := ParseCPUTimes(stat)
	if err != nil {
		t.Fatal(err)
	}
	prev := &rawSnapshot{
		At: time.Unix(0, 0),
		CPU: []CPUTimes{
			{User: 0, Nice: 0, System: 0, Idle: 1000, IOWait: 0, IRQ: 0, SoftIRQ: 0, Steal: 0},
			{User: 0, Nice: 0, System: 0, Idle: 500, IOWait: 0, IRQ: 0, SoftIRQ: 0, Steal: 0},
			{User: 0, Nice: 0, System: 0, Idle: 500, IOWait: 0, IRQ: 0, SoftIRQ: 0, Steal: 0},
		},
		Net:    map[string]NetRaw{},
		DiskIO: map[string]DiskIORaw{},
	}
	cur := &rawSnapshot{
		At: time.Unix(10, 0),
		CPU: []CPUTimes{
			{User: 100, Nice: 200, System: 300, Idle: 400, IOWait: 50, IRQ: 10, SoftIRQ: 20, Steal: 5},
			{User: 50, Nice: 100, System: 150, Idle: 200, IOWait: 25, IRQ: 5, SoftIRQ: 10, Steal: 2},
			{User: 50, Nice: 100, System: 150, Idle: 200, IOWait: 25, IRQ: 5, SoftIRQ: 10, Steal: 3},
		},
		Net:    map[string]NetRaw{},
		DiskIO: map[string]DiskIORaw{},
	}
	s := computeSample(prev, cur)
	dTot := float64(TotalJiffies(cur.CPU[0])) - float64(TotalJiffies(prev.CPU[0]))
	dBusy := float64(BusyJiffies(cur.CPU[0])) - float64(BusyJiffies(prev.CPU[0]))
	want := (dBusy / dTot) * 100
	if math.Abs(s.CPUPct-want) > 1e-6 {
		t.Fatalf("cpu pct got %v want %v", s.CPUPct, want)
	}
	if !s.RatesReady {
		t.Fatal("rates should be ready")
	}
}

func TestParseMeminfo(t *testing.T) {
	s := `MemTotal:       1000000 kB
MemAvailable:   400000 kB
MemFree:        100000 kB
SwapTotal:      200000 kB
SwapFree:       150000 kB
`
	m, err := ParseMeminfo(s)
	if err != nil {
		t.Fatal(err)
	}
	ms := MemSampleFromMeminfo(m)
	if ms.TotalBytes != 1000000*1024 {
		t.Fatalf("total bytes %d", ms.TotalBytes)
	}
	if ms.UsedBytes != (1000000-400000)*1024 {
		t.Fatalf("used bytes %d", ms.UsedBytes)
	}
	if math.Abs(ms.UsedPct-60) > 1e-6 {
		t.Fatalf("used pct %v", ms.UsedPct)
	}
	if ms.SwapUsedBytes != 50000*1024 {
		t.Fatalf("swap used %d", ms.SwapUsedBytes)
	}
}

func TestParseNetDevRates(t *testing.T) {
	prev := &rawSnapshot{
		At: time.Unix(0, 0),
		Net: map[string]NetRaw{
			"eth0": {RxBytes: 1000, TxBytes: 2000},
		},
		DiskIO: map[string]DiskIORaw{},
	}
	curSnap := &rawSnapshot{At: time.Unix(2, 0), Net: map[string]NetRaw{
		"eth0": {RxBytes: 3000, TxBytes: 5000},
	}, DiskIO: map[string]DiskIORaw{}}
	s := computeSample(prev, curSnap)
	if len(s.Net) != 1 || s.Net[0].Iface != "eth0" {
		t.Fatalf("net %+v", s.Net)
	}
	if math.Abs(s.Net[0].RxBps-1000) > 1e-6 || math.Abs(s.Net[0].TxBps-1500) > 1e-6 {
		t.Fatalf("bps rx=%v tx=%v", s.Net[0].RxBps, s.Net[0].TxBps)
	}
}

func TestParseDiskstatsMath(t *testing.T) {
	// fields[12] = ms doing I/O (see /proc/diskstats layout)
	line := "   8       0 sda 100 0 1000 10 200 0 4000 20 0 30 0"
	name, raw, ok := ParseDiskstatsLine(line)
	if !ok || name != "sda" {
		t.Fatalf("parse line ok=%v name=%q", ok, name)
	}
	if raw.ReadSectors != 1000 || raw.WriteSectors != 4000 || raw.MSDoingIO != 30 {
		t.Fatalf("raw %+v", raw)
	}
	prev := &rawSnapshot{
		At:     time.Unix(0, 0),
		DiskIO: map[string]DiskIORaw{"sda": {ReadSectors: 0, WriteSectors: 0, MSDoingIO: 0}},
		Net:    map[string]NetRaw{},
	}
	cur := &rawSnapshot{
		At:     time.Unix(5, 0),
		DiskIO: map[string]DiskIORaw{"sda": raw},
		Net:    map[string]NetRaw{},
	}
	s := computeSample(prev, cur)
	if len(s.DiskIO) != 1 {
		t.Fatalf("disk io len %d", len(s.DiskIO))
	}
	dio := s.DiskIO[0]
	if math.Abs(dio.ReadBps-float64(1000*sectorBytes)/5) > 1e-6 {
		t.Fatalf("read bps %v", dio.ReadBps)
	}
	if math.Abs(dio.WriteBps-float64(4000*sectorBytes)/5) > 1e-6 {
		t.Fatalf("write bps %v", dio.WriteBps)
	}
	// 30 ms over 5s => 30/5000 * 100 = 0.6%
	if math.Abs(dio.BusyPct-0.6) > 1e-6 {
		t.Fatalf("busy %v", dio.BusyPct)
	}
}

func TestParseLoadavgUptime(t *testing.T) {
	l, err := ParseLoadavg("0.12 0.34 0.56 1/100 12345")
	if err != nil {
		t.Fatal(err)
	}
	if l[0] != 0.12 || l[1] != 0.34 || l[2] != 0.56 {
		t.Fatalf("%v", l)
	}
	u, err := ParseUptime("12345.67 890.1")
	if err != nil || math.Abs(u-12345.67) > 1e-6 {
		t.Fatalf("%v %v", u, err)
	}
}
