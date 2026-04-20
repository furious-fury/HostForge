package services

import (
	"regexp"
	"strings"
)

var envVarKeyRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

const (
	// MaxEnvKeyLen is the max length of an env var name after normalization.
	MaxEnvKeyLen = 128
	// MaxEnvValueLen is the max length of a UTF-8 env value in bytes.
	MaxEnvValueLen = 8 * 1024
	// MaxEnvVarsPerProject caps rows per project.
	MaxEnvVarsPerProject = 100
)

// ValueLast4 returns up to the last four characters of plaintext for UI hints.
func ValueLast4(plain []byte) string {
	if len(plain) == 0 {
		return ""
	}
	if len(plain) <= 4 {
		return string(plain)
	}
	return string(plain[len(plain)-4:])
}

// NormalizeEnvKey uppercases, trims, maps hyphens to underscores (common .env style).
func NormalizeEnvKey(raw string) string {
	s := strings.TrimSpace(strings.ReplaceAll(raw, "-", "_"))
	return strings.ToUpper(s)
}

// ValidateEnvEntry returns normalized key or an empty errCode on success.
func ValidateEnvEntry(keyRaw, valueRaw string) (key string, errCode string) {
	key = NormalizeEnvKey(keyRaw)
	if key == "" {
		return "", "env_key_empty"
	}
	if len(key) > MaxEnvKeyLen {
		return "", "env_key_too_long"
	}
	if !envVarKeyRE.MatchString(key) {
		return "", "env_key_invalid"
	}
	if key == "PORT" {
		return "", "env_key_reserved"
	}
	v := []byte(valueRaw)
	if len(v) > MaxEnvValueLen {
		return "", "env_value_too_long"
	}
	return key, ""
}
