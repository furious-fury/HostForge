package hostmetrics

import "time"

// rawSnapshot is one tick of counter-ish host data (Linux reader).
type rawSnapshot struct {
	At         time.Time
	CPU        []CPUTimes
	Mem        Meminfo
	Net        map[string]NetRaw
	DiskIO     map[string]DiskIORaw
	Mounts     []MountEntry
	DiskUsage  []DiskUsage // from statfs at read time
	Load       [3]float64
	UptimeSecs float64
}

func (r *rawSnapshot) clone() *rawSnapshot {
	if r == nil {
		return nil
	}
	out := *r
	out.CPU = append([]CPUTimes(nil), r.CPU...)
	if r.Net != nil {
		out.Net = make(map[string]NetRaw, len(r.Net))
		for k, v := range r.Net {
			out.Net[k] = v
		}
	}
	if r.DiskIO != nil {
		out.DiskIO = make(map[string]DiskIORaw, len(r.DiskIO))
		for k, v := range r.DiskIO {
			out.DiskIO[k] = v
		}
	}
	out.Mounts = append([]MountEntry(nil), r.Mounts...)
	out.DiskUsage = append([]DiskUsage(nil), r.DiskUsage...)
	return &out
}
