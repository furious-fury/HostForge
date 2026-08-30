package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/furious-fury/HostForge/internal/backups"
	"github.com/furious-fury/HostForge/internal/repository"
)

var r2AccountIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)

type backupDestinationRequest struct {
	Name                 string `json:"name"`
	Provider             string `json:"provider"`
	AccountID            string `json:"account_id"`
	Endpoint             string `json:"endpoint"`
	Region               string `json:"region"`
	Bucket               string `json:"bucket"`
	ObjectPrefix         string `json:"object_prefix"`
	PathStyle            bool   `json:"path_style"`
	ServerSideEncryption string `json:"server_side_encryption"`
	SSEKMSKeyID          string `json:"sse_kms_key_id"`
	AccessKey            string `json:"access_key_id"`
	SecretKey            string `json:"secret_access_key"`
}

type backupDestinationPatchRequest struct {
	Name                 *string `json:"name"`
	Provider             *string `json:"provider"`
	AccountID            *string `json:"account_id"`
	Endpoint             *string `json:"endpoint"`
	Region               *string `json:"region"`
	Bucket               *string `json:"bucket"`
	ObjectPrefix         *string `json:"object_prefix"`
	PathStyle            *bool   `json:"path_style"`
	ServerSideEncryption *string `json:"server_side_encryption"`
	SSEKMSKeyID          *string `json:"sse_kms_key_id"`
	AccessKey            *string `json:"access_key_id"`
	SecretKey            *string `json:"secret_access_key"`
}

func normalizeBackupDestination(req backupDestinationRequest) (backupDestinationRequest, error) {
	req.Name, req.Provider, req.AccountID = strings.TrimSpace(req.Name), strings.ToLower(strings.TrimSpace(req.Provider)), strings.TrimSpace(req.AccountID)
	req.Endpoint, req.Region, req.Bucket = strings.TrimSpace(req.Endpoint), strings.TrimSpace(req.Region), strings.TrimSpace(req.Bucket)
	req.ObjectPrefix = strings.Trim(strings.TrimSpace(req.ObjectPrefix), "/")
	req.ServerSideEncryption, req.SSEKMSKeyID = strings.TrimSpace(req.ServerSideEncryption), strings.TrimSpace(req.SSEKMSKeyID)
	if req.Provider == "r2" {
		if !r2AccountIDPattern.MatchString(req.AccountID) {
			return req, fmt.Errorf("invalid Cloudflare account id")
		}
		if req.Endpoint == "" {
			req.Endpoint = "https://" + req.AccountID + ".r2.cloudflarestorage.com"
		}
		req.Region, req.PathStyle = "auto", false
		req.ServerSideEncryption, req.SSEKMSKeyID = "", ""
	} else if req.Provider != "s3" {
		return req, fmt.Errorf("invalid backup provider")
	}
	if req.Provider == "s3" {
		if req.ServerSideEncryption != "" && req.ServerSideEncryption != "AES256" && req.ServerSideEncryption != "aws:kms" {
			return req, fmt.Errorf("invalid S3 server-side encryption")
		}
		if req.ServerSideEncryption == "aws:kms" && req.SSEKMSKeyID == "" || req.ServerSideEncryption != "aws:kms" && req.SSEKMSKeyID != "" {
			return req, fmt.Errorf("S3 KMS key id requires aws:kms encryption")
		}
	}
	parsed, err := url.Parse(req.Endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return req, fmt.Errorf("backup endpoint must be an HTTPS origin")
	}
	if req.Provider == "r2" {
		host := strings.ToLower(parsed.Hostname())
		account := strings.ToLower(req.AccountID)
		standard := account + ".r2.cloudflarestorage.com"
		jurisdictionSuffix := ".r2.cloudflarestorage.com"
		middle := strings.TrimSuffix(strings.TrimPrefix(host, account+"."), jurisdictionSuffix)
		if host != standard && (!strings.HasPrefix(host, account+".") || !strings.HasSuffix(host, jurisdictionSuffix) || middle == "" || strings.Contains(middle, ".")) {
			return req, fmt.Errorf("invalid Cloudflare R2 endpoint")
		}
	}
	if req.Name == "" || req.Region == "" || req.Bucket == "" || strings.TrimSpace(req.AccessKey) == "" || strings.TrimSpace(req.SecretKey) == "" {
		return req, fmt.Errorf("backup destination fields required")
	}
	return req, nil
}

func destinationClient(ctx *http.Request, destination repository.BackupDestinationSealed, s *server) (*backups.Client, error) {
	if s.envSealer == nil {
		return nil, fmt.Errorf("encryption key unavailable")
	}
	access, err := s.envSealer.Open(destination.AccessKeyCT)
	if err != nil {
		return nil, err
	}
	secret, err := s.envSealer.Open(destination.SecretKeyCT)
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, value := range [][]byte{access, secret} {
			for index := range value {
				value[index] = 0
			}
		}
	}()
	return backups.NewClient(ctx.Context(), backups.Destination{Endpoint: destination.Endpoint, Region: destination.Region, Bucket: destination.Bucket, PathStyle: destination.PathStyle, ServerSideEncryption: destination.ServerSideEncryption, SSEKMSKeyID: destination.SSEKMSKeyID, AccessKey: string(access), SecretKey: string(secret)})
}

func existingR2AccountID(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	parts := strings.Split(parsed.Hostname(), ".")
	if len(parts) < 4 || parts[len(parts)-3] != "r2" || parts[len(parts)-2] != "cloudflarestorage" || parts[len(parts)-1] != "com" {
		return ""
	}
	return parts[0]
}

func (s *server) recordBackupDestinationEvent(r *http.Request, status, message, detail string) {
	_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{EventType: "database_backup_storage", Status: status, Actor: "operator", Message: message, Detail: strings.TrimSpace(detail)})
}

func (s *server) handleBackupDestinations(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/backup-destinations"), "/")
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			items, err := s.store.ListBackupDestinations(r.Context())
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_backup_destinations_failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"destinations": items})
		case http.MethodPost:
			if s.envSealer == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "error": "env_encryption_key_missing"})
				return
			}
			var req backupDestinationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
				return
			}
			req, err := normalizeBackupDestination(req)
			if err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "backup_destination_invalid"})
				return
			}
			client, err := backups.NewClient(r.Context(), backups.Destination{Endpoint: req.Endpoint, Region: req.Region, Bucket: req.Bucket, PathStyle: req.PathStyle, ServerSideEncryption: req.ServerSideEncryption, SSEKMSKeyID: req.SSEKMSKeyID, AccessKey: req.AccessKey, SecretKey: req.SecretKey})
			probeKey := strings.Trim(req.ObjectPrefix+"/hostforge-probes/connect-"+fmt.Sprint(time.Now().UTC().UnixNano()), "/")
			if err != nil || client.Test(r.Context(), probeKey) != nil {
				s.recordBackupDestinationEvent(r, "failed", "Backup destination connection failed", req.Name+":"+req.Bucket)
				writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "backup_destination_test_failed"})
				return
			}
			accessCT, err := s.envSealer.Seal([]byte(req.AccessKey))
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "backup_destination_encrypt_failed"})
				return
			}
			secretCT, err := s.envSealer.Seal([]byte(req.SecretKey))
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "backup_destination_encrypt_failed"})
				return
			}
			item, err := s.store.CreateBackupDestination(r.Context(), repository.CreateBackupDestinationInput{Name: req.Name, Provider: req.Provider, Endpoint: req.Endpoint, Region: req.Region, Bucket: req.Bucket, ObjectPrefix: req.ObjectPrefix, PathStyle: req.PathStyle, ServerSideEncryption: req.ServerSideEncryption, SSEKMSKeyID: req.SSEKMSKeyID, AccessKeyCT: accessCT, SecretKeyCT: secretCT})
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "create_backup_destination_failed"})
				return
			}
			item, _ = s.store.UpdateBackupDestinationTest(r.Context(), item.ID, "success", "Bucket write, read, and delete probe succeeded")
			s.recordBackupDestinationEvent(r, "created", "Backup destination connected", item.Name+":"+item.Bucket)
			writeJSON(w, http.StatusCreated, map[string]any{"status": "created", "destination": item})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 1 && r.Method == http.MethodPatch {
		if s.envSealer == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "error": "env_encryption_key_missing"})
			return
		}
		current, err := s.store.GetBackupDestinationSealed(r.Context(), parts[0])
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "backup_destination_not_found"})
			return
		}
		var patch backupDestinationPatchRequest
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
			return
		}
		access, accessErr := s.envSealer.Open(current.AccessKeyCT)
		secret, secretErr := s.envSealer.Open(current.SecretKeyCT)
		if accessErr != nil || secretErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "backup_destination_decrypt_failed"})
			return
		}
		decrypted := [][]byte{access, secret}
		defer func() {
			for _, value := range decrypted {
				for index := range value {
					value[index] = 0
				}
			}
		}()
		req := backupDestinationRequest{Name: current.Name, Provider: current.Provider, Endpoint: current.Endpoint, Region: current.Region, Bucket: current.Bucket, ObjectPrefix: current.ObjectPrefix, PathStyle: current.PathStyle, ServerSideEncryption: current.ServerSideEncryption, SSEKMSKeyID: current.SSEKMSKeyID, AccessKey: string(access), SecretKey: string(secret)}
		if current.Provider == "r2" {
			req.AccountID = existingR2AccountID(current.Endpoint)
		}
		if patch.Name != nil {
			req.Name = *patch.Name
		}
		if patch.Provider != nil {
			req.Provider = *patch.Provider
		}
		if patch.AccountID != nil {
			req.AccountID = *patch.AccountID
		}
		if patch.Endpoint != nil {
			req.Endpoint = *patch.Endpoint
		}
		if patch.Region != nil {
			req.Region = *patch.Region
		}
		if patch.Bucket != nil {
			req.Bucket = *patch.Bucket
		}
		if patch.ObjectPrefix != nil {
			req.ObjectPrefix = *patch.ObjectPrefix
		}
		if patch.PathStyle != nil {
			req.PathStyle = *patch.PathStyle
		}
		if patch.ServerSideEncryption != nil {
			req.ServerSideEncryption = *patch.ServerSideEncryption
		}
		if patch.SSEKMSKeyID != nil {
			req.SSEKMSKeyID = *patch.SSEKMSKeyID
		}
		if patch.AccessKey != nil && strings.TrimSpace(*patch.AccessKey) != "" {
			req.AccessKey = *patch.AccessKey
		}
		if patch.SecretKey != nil && strings.TrimSpace(*patch.SecretKey) != "" {
			req.SecretKey = *patch.SecretKey
		}
		req, err = normalizeBackupDestination(req)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "backup_destination_invalid"})
			return
		}
		client, err := backups.NewClient(r.Context(), backups.Destination{Endpoint: req.Endpoint, Region: req.Region, Bucket: req.Bucket, PathStyle: req.PathStyle, ServerSideEncryption: req.ServerSideEncryption, SSEKMSKeyID: req.SSEKMSKeyID, AccessKey: req.AccessKey, SecretKey: req.SecretKey})
		probeKey := strings.Trim(req.ObjectPrefix+"/hostforge-probes/update-"+fmt.Sprint(time.Now().UTC().UnixNano()), "/")
		if err != nil || client.Test(r.Context(), probeKey) != nil {
			s.recordBackupDestinationEvent(r, "failed", "Backup destination update probe failed", current.Name+":"+current.Bucket)
			writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "backup_destination_test_failed"})
			return
		}
		accessCT, accessErr := s.envSealer.Seal([]byte(req.AccessKey))
		secretCT, secretErr := s.envSealer.Seal([]byte(req.SecretKey))
		if accessErr != nil || secretErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "backup_destination_encrypt_failed"})
			return
		}
		item, err := s.store.UpdateBackupDestination(r.Context(), current.ID, repository.CreateBackupDestinationInput{Name: req.Name, Provider: req.Provider, Endpoint: req.Endpoint, Region: req.Region, Bucket: req.Bucket, ObjectPrefix: req.ObjectPrefix, PathStyle: req.PathStyle, ServerSideEncryption: req.ServerSideEncryption, SSEKMSKeyID: req.SSEKMSKeyID, AccessKeyCT: accessCT, SecretKeyCT: secretCT})
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "update_backup_destination_failed"})
			return
		}
		item, _ = s.store.UpdateBackupDestinationTest(r.Context(), item.ID, "success", "Bucket write, read, and delete probe succeeded")
		s.recordBackupDestinationEvent(r, "updated", "Backup destination updated", item.Name+":"+item.Bucket)
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "destination": item})
		return
	}
	if len(parts) == 2 && parts[1] == "test" && r.Method == http.MethodPost {
		destination, err := s.store.GetBackupDestinationSealed(r.Context(), parts[0])
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "backup_destination_not_found"})
			return
		}
		client, err := destinationClient(r, destination, s)
		probeKey := strings.Trim(destination.ObjectPrefix+"/hostforge-probes/test-"+fmt.Sprint(time.Now().UTC().UnixNano()), "/")
		if err != nil || client.Test(r.Context(), probeKey) != nil {
			item, _ := s.store.UpdateBackupDestinationTest(r.Context(), destination.ID, "failed", "Bucket probe failed; verify endpoint, credentials, and permissions")
			s.recordBackupDestinationEvent(r, "failed", "Backup destination verification failed", destination.Name+":"+destination.Bucket)
			writeJSON(w, http.StatusBadGateway, map[string]any{"status": "error", "error": "backup_destination_test_failed", "destination": item})
			return
		}
		item, _ := s.store.UpdateBackupDestinationTest(r.Context(), destination.ID, "success", "Bucket write, read, and delete probe succeeded")
		s.recordBackupDestinationEvent(r, "verified", "Backup destination verified", destination.Name+":"+destination.Bucket)
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "destination": item})
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		current, lookupErr := s.store.GetBackupDestination(r.Context(), parts[0])
		if lookupErr != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "backup_destination_not_found"})
			return
		}
		if err := s.store.DeleteBackupDestination(r.Context(), parts[0]); err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "delete_backup_destination_failed"})
			return
		}
		s.recordBackupDestinationEvent(r, "deleted", "Backup destination deleted", current.Name+":"+current.Bucket)
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
}
