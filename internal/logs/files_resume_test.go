package logs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFollowFileFromOffset_Appends(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		time.Sleep(120 * time.Millisecond)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		_, _ = f.WriteString(" world")
		_ = f.Close()
	}()

	var got string
	err := FollowFileFromOffset(ctx, path, 5, 50*time.Millisecond, nil, func(data []byte, endOffset int64) error {
		if endOffset != 5+int64(len(data)) {
			t.Fatalf("endOffset=%d want %d", endOffset, 5+int64(len(data)))
		}
		got += string(data)
		cancel()
		return nil
	})
	if err != nil && err != context.Canceled {
		t.Fatalf("follow: %v", err)
	}
	if got != " world" {
		t.Fatalf("got %q", got)
	}
}

func TestFollowFileFromOffset_TruncateResets(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "t.log")
	if err := os.WriteFile(path, []byte("abcdefghij"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		time.Sleep(120 * time.Millisecond)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			return
		}
	}()

	rotations := 0
	err := FollowFileFromOffset(ctx, path, 20, 50*time.Millisecond, func() error {
		rotations++
		cancel()
		return nil
	}, func(data []byte, endOffset int64) error {
		_ = data
		_ = endOffset
		return nil
	})
	if err != nil && err != context.Canceled {
		t.Fatalf("follow: %v", err)
	}
	if rotations < 1 {
		t.Fatalf("expected onRotated after truncate, got %d", rotations)
	}
}

func TestTailFileWithEOF(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "x.log")
	content := []byte("0123456789abcdef")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	tail, eof, err := TailFileWithEOF(path, 4, 0)
	if err != nil {
		t.Fatal(err)
	}
	if eof != int64(len(content)) {
		t.Fatalf("eof=%d", eof)
	}
	if string(tail) != "cdef" {
		t.Fatalf("tail=%q", tail)
	}
}
