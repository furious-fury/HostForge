// Package hostmetrics samples Linux host CPU, memory, network, and disk metrics
// from /proc and /sys. On non-Linux platforms reads fail with ErrUnsupportedOS.
package hostmetrics

import "time"

// MemSample is memory usage at a point in time.
type MemSample struct {
	TotalBytes          int64   `json:"total_bytes"`
	UsedBytes           int64   `json:"used_bytes"`
	AvailableBytes      int64   `json:"available_bytes"`
	BuffersCachedBytes  int64   `json:"buffers_cached_bytes"`
	SwapTotalBytes      int64   `json:"swap_total_bytes"`
	SwapUsedBytes       int64   `json:"swap_used_bytes"`
	UsedPct             float64 `json:"used_pct"`
}

// NetSample is per-interface throughput since the previous sample.
type NetSample struct {
	Iface  string  `json:"iface"`
	RxBps  float64 `json:"rx_bps"`
	TxBps  float64 `json:"tx_bps"`
}

// DiskUsage is space usage for one mount point.
type DiskUsage struct {
	Mount      string  `json:"mount"`
	FSType     string  `json:"fs_type"`
	TotalBytes int64   `json:"total_bytes"`
	UsedBytes  int64   `json:"used_bytes"`
	AvailBytes int64   `json:"avail_bytes"`
	UsedPct    float64 `json:"used_pct"`
}

// DiskIOSample is block device throughput and utilization since the previous sample.
type DiskIOSample struct {
	Device   string  `json:"device"`
	ReadBps  float64 `json:"read_bps"`
	WriteBps float64 `json:"write_bps"`
	BusyPct  float64 `json:"busy_pct"`
}

// Sample is one aggregated observation for the API and UI.
type Sample struct {
	At           time.Time   `json:"at"`
	CPUPct       float64     `json:"cpu_pct"`
	PerCorePct   []float64   `json:"per_core_pct"`
	LoadAvg      [3]float64  `json:"load_avg"`
	Mem          MemSample   `json:"mem"`
	Net          []NetSample `json:"net"`
	Disks        []DiskUsage `json:"disks"`
	DiskIO       []DiskIOSample `json:"disk_io"`
	Uptime       float64     `json:"uptime_seconds"`
	RatesReady   bool        `json:"rates_ready"`
	Err          string      `json:"err,omitempty"`
}

// normalizeSliceJSONFields replaces nil slices so encoding/json emits [] not null.
func (s *Sample) normalizeSliceJSONFields() {
	if s.Net == nil {
		s.Net = []NetSample{}
	}
	if s.DiskIO == nil {
		s.DiskIO = []DiskIOSample{}
	}
	if s.PerCorePct == nil {
		s.PerCorePct = []float64{}
	}
	if s.Disks == nil {
		s.Disks = []DiskUsage{}
	}
}
