package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"

	"github.com/hostforge/hostforge/internal/repository"
	"golang.org/x/crypto/ssh"
)

type apiProjectSSHKey struct {
	Configured  bool   `json:"configured"`
	PublicKey   string `json:"public_key,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// handleProjectSSHKeyGet returns the public key + fingerprint for the project,
// or {configured:false} if none is configured.
func (s *server) handleProjectSSHKeyGet(w http.ResponseWriter, r *http.Request, projectID string) {
	if _, err := s.store.GetProjectByID(r.Context(), projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}
	meta, err := s.store.GetProjectSSHKeyMeta(r.Context(), projectID)
	if err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusOK, map[string]any{"ssh_key": apiProjectSSHKey{Configured: false}})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "ssh_key_lookup_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ssh_key": apiProjectSSHKey{
		Configured:  true,
		PublicKey:   meta.PublicKey,
		Fingerprint: meta.Fingerprint,
		CreatedAt:   formatTime(meta.CreatedAt),
	}})
}

// handleProjectSSHKeyGenerate creates a fresh ed25519 keypair, seals the
// private key and stores it. The public key is returned so the user can paste
// it into the repo's deploy-keys settings. Any existing key is replaced.
func (s *server) handleProjectSSHKeyGenerate(w http.ResponseWriter, r *http.Request, projectID string) {
	if !s.requireEnvSealer(w) {
		return
	}
	if _, err := s.store.GetProjectByID(r.Context(), projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "ssh_key_generate_failed"})
		return
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "ssh_key_encode_failed"})
		return
	}
	pubLine := fmt.Sprintf("%s %s hostforge-%s", sshPub.Type(), base64.StdEncoding.EncodeToString(sshPub.Marshal()), shortProjectLabel(projectID))
	fingerprint := ssh.FingerprintSHA256(sshPub)

	pemBlock, err := ssh.MarshalPrivateKey(priv, "hostforge-"+shortProjectLabel(projectID))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "ssh_key_marshal_failed"})
		return
	}
	privPEM := pem.EncodeToMemory(pemBlock)
	privCT, err := s.envSealer.Seal(privPEM)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "ssh_key_seal_failed"})
		return
	}

	meta, err := s.store.UpsertProjectSSHKey(r.Context(), repository.UpsertProjectSSHKeyInput{
		ProjectID:    projectID,
		PublicKey:    pubLine,
		PrivateKeyCT: privCT,
		Fingerprint:  fingerprint,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "ssh_key_upsert_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "ssh_key": apiProjectSSHKey{
		Configured:  true,
		PublicKey:   meta.PublicKey,
		Fingerprint: meta.Fingerprint,
		CreatedAt:   formatTime(meta.CreatedAt),
	}})
}

// handleProjectSSHKeyDelete removes a project's ssh deploy key.
func (s *server) handleProjectSSHKeyDelete(w http.ResponseWriter, r *http.Request, projectID string) {
	if _, err := s.store.GetProjectByID(r.Context(), projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}
	if err := s.store.DeleteProjectSSHKey(r.Context(), projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "ssh_key_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "ssh_key_delete_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// shortProjectLabel is a short human label used in ssh key comments.
func shortProjectLabel(projectID string) string {
	id := strings.TrimSpace(projectID)
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

