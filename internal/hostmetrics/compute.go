package hostmetrics

import (
	"math"
	"sort"
	"time"
)

func computeSample(prev, cur *rawSnapshot) Sample {
	now := cur.At
	if now.IsZero() {
		now = time.Now().UTC()
	}
	mem := MemSampleFromMeminfo(cur.Mem)
	out := Sample{
		At:         now,
		Mem:        mem,
		Disks:      append([]DiskUsage(nil), cur.DiskUsage...),
		LoadAvg:    cur.Load,
		Uptime:     cur.UptimeSecs,
		RatesReady: prev != nil,
	}
	if prev == nil {
		out.normalizeSliceJSONFields()
		return out
	}
	dt := cur.At.Sub(prev.At).Seconds()
	if dt <= 0 || math.IsNaN(dt) {
		dt = 1
	}

	// CPU aggregate + per-core
	if len(cur.CPU) > 0 && len(prev.CPU) == len(cur.CPU) {
		aggCur := cur.CPU[0]
		aggPrev := prev.CPU[0]
		dTotal := float64(TotalJiffies(aggCur)) - float64(TotalJiffies(aggPrev))
		dBusy := float64(BusyJiffies(aggCur)) - float64(BusyJiffies(aggPrev))
		if dTotal > 0 && dBusy >= 0 {
			out.CPUPct = (dBusy / dTotal) * 100
		}
		n := len(cur.CPU) - 1
		out.PerCorePct = make([]float64, n)
		for i := 0; i < n; i++ {
			c := cur.CPU[i+1]
			p := prev.CPU[i+1]
			tot := float64(TotalJiffies(c)) - float64(TotalJiffies(p))
			bus := float64(BusyJiffies(c)) - float64(BusyJiffies(p))
			if tot > 0 && bus >= 0 {
				out.PerCorePct[i] = (bus / tot) * 100
			}
		}
	}

	// Network deltas
	ifaceSet := make(map[string]struct{})
	for k := range cur.Net {
		ifaceSet[k] = struct{}{}
	}
	for k := range prev.Net {
		ifaceSet[k] = struct{}{}
	}
	var names []string
	for k := range ifaceSet {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		c := cur.Net[name]
		p := prev.Net[name]
		drx := float64(c.RxBytes) - float64(p.RxBytes)
		dtx := float64(c.TxBytes) - float64(p.TxBytes)
		if drx < 0 {
			drx = 0
		}
		if dtx < 0 {
			dtx = 0
		}
		out.Net = append(out.Net, NetSample{
			Iface: name,
			RxBps: drx / dt,
			TxBps: dtx / dt,
		})
	}

	// Disk I/O
	devSet := make(map[string]struct{})
	for k := range cur.DiskIO {
		devSet[k] = struct{}{}
	}
	for k := range prev.DiskIO {
		devSet[k] = struct{}{}
	}
	var devs []string
	for k := range devSet {
		devs = append(devs, k)
	}
	sort.Strings(devs)
	for _, dname := range devs {
		c := cur.DiskIO[dname]
		p := prev.DiskIO[dname]
		drs := float64(c.ReadSectors) - float64(p.ReadSectors)
		dws := float64(c.WriteSectors) - float64(p.WriteSectors)
		dms := float64(c.MSDoingIO) - float64(p.MSDoingIO)
		if drs < 0 {
			drs = 0
		}
		if dws < 0 {
			dws = 0
		}
		if dms < 0 {
			dms = 0
		}
		dtMs := dt * 1000
		busy := 0.0
		if dtMs > 0 {
			busy = math.Min(100, (dms/dtMs)*100)
		}
		out.DiskIO = append(out.DiskIO, DiskIOSample{
			Device:   dname,
			ReadBps:  drs * sectorBytes / dt,
			WriteBps: dws * sectorBytes / dt,
			BusyPct:  busy,
		})
	}

	out.normalizeSliceJSONFields()
	return out
}
