//go:build !linux

package hostmetrics

// DefaultReader returns a reader that reports ErrUnsupportedOS on every call.
func DefaultReader(opts ReaderOptions) Reader {
	_ = opts
	return stubReader{}
}

type stubReader struct{}

func (stubReader) ReadSnapshot() (*rawSnapshot, error) {
	return nil, ErrUnsupportedOS
}
