package main

import "net/http"

func (s *server) requireEnvSealer(w http.ResponseWriter) bool {
	if s.envSealer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "error": "env_encryption_key_missing"})
		return false
	}
	return true
}
