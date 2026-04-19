package hostmetrics

import "errors"

// ErrUnsupportedOS is returned by readers on non-Linux builds or when host metrics are unavailable.
var ErrUnsupportedOS = errors.New("hostmetrics: unsupported operating system (linux only)")
