package logs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	// DefaultTailBytes is the default tail size returned by log APIs.
	DefaultTailBytes = 64 * 1024
	// MaxTailBytes is the hard cap for tail responses to prevent unbounded memory use.
	MaxTailBytes = 1024 * 1024
)

// TailFile returns the last maxBytes from path; when tailLines > 0 it returns only that many ending lines.
func TailFile(path string, maxBytes, tailLines int) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultTailBytes
	}
	if maxBytes > MaxTailBytes {
		maxBytes = MaxTailBytes
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	if size <= 0 {
		return []byte{}, nil
	}
	readSize := int64(maxBytes)
	if size < readSize {
		readSize = size
	}
	start := size - readSize
	buf := make([]byte, readSize)
	n, err := f.ReadAt(buf, start)
	if err != nil && err != io.EOF {
		return nil, err
	}
	buf = buf[:n]
	if tailLines > 0 {
		buf = tailLastLines(buf, tailLines)
	}
	return buf, nil
}

// FollowFile polls for appended data and invokes onChunk for new bytes.
// If the file does not exist yet, it waits for it to appear (cancellable via ctx)
// instead of failing, so log subscribers can attach before the writer creates the file.
func FollowFile(ctx context.Context, path string, pollInterval time.Duration, onChunk func([]byte) error) error {
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}
	var f *os.File
	for {
		var openErr error
		f, openErr = os.Open(path)
		if openErr == nil {
			break
		}
		if !os.IsNotExist(openErr) {
			return openErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return err
	}
	offset := st.Size()
	buf := make([]byte, 8192)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			nextStat, statErr := os.Stat(path)
			if statErr != nil {
				return statErr
			}
			if nextStat.Size() < offset {
				offset = 0
			}
			for offset < nextStat.Size() {
				toRead := int64(len(buf))
				remaining := nextStat.Size() - offset
				if remaining < toRead {
					toRead = remaining
				}
				n, readErr := f.ReadAt(buf[:toRead], offset)
				if readErr != nil && readErr != io.EOF {
					return fmt.Errorf("read log chunk: %w", readErr)
				}
				if n <= 0 {
					break
				}
				if err := onChunk(buf[:n]); err != nil {
					return err
				}
				offset += int64(n)
			}
		}
	}
}

func tailLastLines(data []byte, lines int) []byte {
	if lines <= 0 || len(data) == 0 {
		return data
	}
	trimmed := bytes.TrimRight(data, "\n")
	if len(trimmed) == 0 {
		return []byte{}
	}
	count := 0
	for i := len(trimmed) - 1; i >= 0; i-- {
		if trimmed[i] == '\n' {
			count++
			if count == lines {
				return trimmed[i+1:]
			}
		}
	}
	return trimmed
}
