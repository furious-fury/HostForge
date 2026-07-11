package hostmetrics

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ReaderOptions configures which network interfaces and mounts are included.
type ReaderOptions struct {
	NetInclude       *regexp.Regexp
	NetExclude       *regexp.Regexp
	DiskMountInclude *regexp.Regexp
}

var defaultNetExclude = regexp.MustCompile(`^(lo|docker0)$|^(br-|veth)`)

// ParseReaderOptionsFromEnv reads HOSTFORGE_HOSTMETRICS_NET_INCLUDE, _NET_EXCLUDE, _DISK_INCLUDE.
func ParseReaderOptionsFromEnv() ReaderOptions {
	var o ReaderOptions
	if s := strings.TrimSpace(os.Getenv("HOSTFORGE_HOSTMETRICS_NET_INCLUDE")); s != "" {
		if re, err := regexp.Compile(s); err == nil {
			o.NetInclude = re
		}
	}
	if s := strings.TrimSpace(os.Getenv("HOSTFORGE_HOSTMETRICS_NET_EXCLUDE")); s != "" {
		if re, err := regexp.Compile(s); err == nil {
			o.NetExclude = re
		}
	} else {
		o.NetExclude = defaultNetExclude
	}
	if s := strings.TrimSpace(os.Getenv("HOSTFORGE_HOSTMETRICS_DISK_INCLUDE")); s != "" {
		if re, err := regexp.Compile(s); err == nil {
			o.DiskMountInclude = re
		}
	}
	return o
}

// IntervalFromEnv returns HOSTFORGE_HOSTMETRICS_INTERVAL_MS or defaultMs milliseconds.
func IntervalFromEnv(defaultMs int) time.Duration {
	if defaultMs <= 0 {
		defaultMs = 5000
	}
	s := strings.TrimSpace(os.Getenv("HOSTFORGE_HOSTMETRICS_INTERVAL_MS"))
	if s == "" {
		return time.Duration(defaultMs) * time.Millisecond
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1000 {
		return time.Duration(defaultMs) * time.Millisecond
	}
	return time.Duration(n) * time.Millisecond
}

// CapacityFromEnv returns HOSTFORGE_HOSTMETRICS_CAPACITY or defaultCap.
func CapacityFromEnv(defaultCap int) int {
	if defaultCap <= 0 {
		defaultCap = 360
	}
	s := strings.TrimSpace(os.Getenv("HOSTFORGE_HOSTMETRICS_CAPACITY"))
	if s == "" {
		return defaultCap
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 10 {
		return defaultCap
	}
	return n
}
