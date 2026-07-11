package hostmetrics

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Reader loads a raw host snapshot (implemented on Linux).
type Reader interface {
	ReadSnapshot() (*rawSnapshot, error)
}

// Sampler periodically samples the host and retains a ring buffer of Samples.
type Sampler struct {
	mu sync.Mutex

	interval time.Duration
	capacity int
	ring     []Sample
	head     int // index of oldest when len(ring)==capacity
	full     bool

	reader Reader

	prev *rawSnapshot

	supported bool // false after ErrUnsupportedOS
	started   bool
}

// NewSampler constructs a sampler with the given tick interval and max history length.
func NewSampler(interval time.Duration, capacity int, reader Reader) *Sampler {
	if interval < time.Second {
		interval = time.Second
	}
	if capacity < 2 {
		capacity = 2
	}
	return &Sampler{
		interval:  interval,
		capacity:  capacity,
		reader:    reader,
		supported: reader != nil,
	}
}

// Supported is false when the platform cannot collect host metrics (e.g. non-Linux).
func (s *Sampler) Supported() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.supported
}

// HasSamples is true once at least one sample is in the ring.
func (s *Sampler) HasSamples() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ring) > 0
}

// Latest returns the most recent sample or zero values if none yet.
func (s *Sampler) Latest() Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ring) == 0 {
		return Sample{}
	}
	if s.full {
		last := (s.head + s.capacity - 1) % s.capacity
		return s.ring[last]
	}
	return s.ring[len(s.ring)-1]
}

// History returns up to maxPoints samples oldest-first (maxPoints<=0 means all).
func (s *Sampler) History(maxPoints int) []Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	ordered := s.orderedLocked()
	if maxPoints > 0 && len(ordered) > maxPoints {
		ordered = ordered[len(ordered)-maxPoints:]
	}
	out := make([]Sample, len(ordered))
	copy(out, ordered)
	return out
}

func (s *Sampler) orderedLocked() []Sample {
	if len(s.ring) == 0 {
		return nil
	}
	if !s.full {
		out := make([]Sample, len(s.ring))
		copy(out, s.ring)
		return out
	}
	out := make([]Sample, s.capacity)
	copy(out, s.ring[s.head:])
	copy(out[s.capacity-s.head:], s.ring[:s.head])
	return out
}

func (s *Sampler) pushLocked(x Sample) {
	if len(s.ring) < s.capacity {
		s.ring = append(s.ring, x)
		return
	}
	if !s.full {
		s.full = true
	}
	s.ring[s.head] = x
	s.head = (s.head + 1) % s.capacity
}

// Start begins background sampling until ctx is cancelled.
func (s *Sampler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	go func() {
		s.tick()
		t := time.NewTicker(s.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.tick()
			}
		}
	}()
}

func (s *Sampler) tick() {
	if s.reader == nil {
		return
	}
	s.mu.Lock()
	if !s.supported {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	cur, err := s.reader.ReadSnapshot()
	if err != nil {
		if errors.Is(err, ErrUnsupportedOS) {
			s.mu.Lock()
			s.supported = false
			s.prev = nil
			s.mu.Unlock()
			return
		}
		s.mu.Lock()
		bad := Sample{At: time.Now().UTC(), Err: err.Error()}
		bad.normalizeSliceJSONFields()
		s.pushLocked(bad)
		s.prev = nil
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.supported {
		return
	}
	sample := computeSample(s.prev, cur)
	s.pushLocked(sample)
	s.prev = cur.clone()
}
