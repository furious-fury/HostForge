package services

import (
	"math"
	"strings"
	"testing"
)

func TestDatabaseDiskReserveAdmission(t *testing.T) {
	if err := requireDatabaseDiskReserveAtPath(t.TempDir(), 1); err != nil {
		t.Fatalf("small reserve should pass on the test filesystem: %v", err)
	}
	if err := requireDatabaseDiskReserveAtPath(t.TempDir(), math.MaxInt64); err == nil || !strings.Contains(err.Error(), "available") {
		t.Fatalf("impossible reserve should be rejected, got %v", err)
	}
	if err := requireDatabaseDiskReserveAtPath(t.TempDir(), 0); err == nil {
		t.Fatal("zero disk reserve should be rejected")
	}
}
