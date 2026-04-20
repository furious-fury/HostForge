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

// TailFileWithEOF returns the same tail as TailFile plus the file's size at read time (exclusive byte offset / EOF).
func TailFileWithEOF(path string, maxBytes, tailLines int) (content []byte, eof int64, err error) {
	content, err = TailFile(path, maxBytes, tailLines)
	if err != nil {
		return nil, 0, err
	}
	st, statErr := os.Stat(path)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return content, 0, nil
		}
		return nil, 0, statErr
	}
	return content, st.Size(), nil
}

// openLogAtEnd waits until path exists, opens it, and returns the file positioned at EOF for follow reads.
func openLogAtEnd(ctx context.Context, path string, pollInterval time.Duration) (*os.File, int64, error) {
	for {
		f, err := os.Open(path)
		if err == nil {
			st, err := f.Stat()
			if err != nil {
				_ = f.Close()
				return nil, 0, err
			}
			return f, st.Size(), nil
		}
		if !os.IsNotExist(err) {
			return nil, 0, err
		}
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// FollowFile polls for appended data and invokes onChunk for new bytes.
// If the file does not exist yet, it waits for it to appear (cancellable via ctx)
// instead of failing, so log subscribers can attach before the writer creates the file.
func FollowFile(ctx context.Context, path string, pollInterval time.Duration, onChunk func([]byte) error) error {
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}
	f, offset, err := openLogAtEnd(ctx, path, pollInterval)
	if err != nil {
		return err
	}
	defer func() {
		if f != nil {
			_ = f.Close()
		}
	}()

	buf := make([]byte, 8192)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if f == nil {
				nf, off, reopenErr := openLogAtEnd(ctx, path, pollInterval)
				if reopenErr != nil {
					return reopenErr
				}
				f = nf
				offset = off
				continue
			}
			pathInfo, statErr := os.Stat(path)
			if statErr != nil {
				if os.IsNotExist(statErr) {
					_ = f.Close()
					f = nil
					continue
				}
				return statErr
			}
			fi, ferr := f.Stat()
			if ferr != nil {
				return ferr
			}
			if !os.SameFile(fi, pathInfo) {
				_ = f.Close()
				f = nil
				continue
			}
			nextStat := pathInfo
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

// FollowFileFromOffset polls like FollowFile but begins reading at initialOffset (next byte to read).
// If the file shrinks below initialOffset or rotates, onRotated is invoked (if non-nil), reading restarts from offset 0.
// onChunk receives each new slice and the exclusive end offset in the current file after that slice.
func FollowFileFromOffset(ctx context.Context, path string, initialOffset int64, pollInterval time.Duration, onRotated func() error, onChunk func(data []byte, endOffset int64) error) error {
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}
	offset := initialOffset
	if offset < 0 {
		offset = 0
	}

	var f *os.File
	openFresh := func() error {
		if f != nil {
			_ = f.Close()
			f = nil
		}
		nf, size, err := openLogAtEnd(ctx, path, pollInterval)
		if err != nil {
			return err
		}
		if offset > size {
			if onRotated != nil {
				if err := onRotated(); err != nil {
					_ = nf.Close()
					return err
				}
			}
			offset = 0
		}
		if offset < 0 {
			offset = 0
		}
		f = nf
		return nil
	}

	if err := openFresh(); err != nil {
		return err
	}
	defer func() {
		if f != nil {
			_ = f.Close()
		}
	}()

	buf := make([]byte, 8192)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if f == nil {
				if err := openFresh(); err != nil {
					return err
				}
				continue
			}
			pathInfo, statErr := os.Stat(path)
			if statErr != nil {
				if os.IsNotExist(statErr) {
					_ = f.Close()
					f = nil
					offset = 0
					continue
				}
				return statErr
			}
			fi, ferr := f.Stat()
			if ferr != nil {
				return ferr
			}
			if !os.SameFile(fi, pathInfo) {
				_ = f.Close()
				f = nil
				if onRotated != nil {
					if err := onRotated(); err != nil {
						return err
					}
				}
				offset = 0
				continue
			}
			nextStat := pathInfo
			if nextStat.Size() < offset {
				if onRotated != nil {
					if err := onRotated(); err != nil {
						return err
					}
				}
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
				offset += int64(n)
				if err := onChunk(buf[:n], offset); err != nil {
					return err
				}
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
