package docker

import (
	"fmt"
	"net"
)

// PickHostPort picks a host port according to the provided strategy.
// requested > 0: use that exact port if available.
// requested == 0: ask OS for an ephemeral free port.
// requested < 0: scan [start, end] for first available port.
func PickHostPort(requested, start, end int) (int, error) {
	switch {
	case requested > 0:
		if err := ensurePortAvailable(requested); err != nil {
			return 0, err
		}
		return requested, nil
	case requested == 0:
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, fmt.Errorf("allocate ephemeral port: %w", err)
		}
		defer ln.Close()
		addr, ok := ln.Addr().(*net.TCPAddr)
		if !ok {
			return 0, fmt.Errorf("unexpected listener address type %T", ln.Addr())
		}
		return addr.Port, nil
	default:
		if start <= 0 || end <= 0 || start > end {
			return 0, fmt.Errorf("invalid host port range %d..%d", start, end)
		}
		for p := start; p <= end; p++ {
			if ensurePortAvailable(p) == nil {
				return p, nil
			}
		}
		return 0, fmt.Errorf("no free ports in range %d..%d", start, end)
	}
}

func ensurePortAvailable(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("host port %d unavailable: %w", port, err)
	}
	_ = ln.Close()
	return nil
}
