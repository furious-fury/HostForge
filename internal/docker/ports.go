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
	return PickHostPortAvoiding(requested, start, end, nil)
}

// PickHostPortAvoiding behaves like PickHostPort but additionally treats every port in
// reserved as unavailable. This lets callers exclude ports already claimed by other
// HostForge containers (running or stopped) so deploys don't reuse a host port that
// another project still owns in the database.
func PickHostPortAvoiding(requested, start, end int, reserved map[int]struct{}) (int, error) {
	switch {
	case requested > 0:
		if _, taken := reserved[requested]; taken {
			return 0, fmt.Errorf("host port %d reserved by another container", requested)
		}
		if err := ensurePortAvailable(requested); err != nil {
			return 0, err
		}
		return requested, nil
	case requested == 0:
		const maxAttempts = 64
		for attempt := 0; attempt < maxAttempts; attempt++ {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return 0, fmt.Errorf("allocate ephemeral port: %w", err)
			}
			addr, ok := ln.Addr().(*net.TCPAddr)
			_ = ln.Close()
			if !ok {
				return 0, fmt.Errorf("unexpected listener address type %T", ln.Addr())
			}
			if _, taken := reserved[addr.Port]; taken {
				continue
			}
			return addr.Port, nil
		}
		return 0, fmt.Errorf("could not find ephemeral port outside reserved set after %d attempts", maxAttempts)
	default:
		if start <= 0 || end <= 0 || start > end {
			return 0, fmt.Errorf("invalid host port range %d..%d", start, end)
		}
		for p := start; p <= end; p++ {
			if _, taken := reserved[p]; taken {
				continue
			}
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
