package main

import "testing"

func TestNormalizeR2DestinationDerivesScopedEndpoint(t *testing.T) {
	req, err := normalizeBackupDestination(backupDestinationRequest{
		Name: "R2", Provider: "r2", AccountID: "0123456789abcdef0123456789abcdef",
		Bucket: "backups", AccessKey: "access", SecretKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.Endpoint != "https://0123456789abcdef0123456789abcdef.r2.cloudflarestorage.com" || req.Region != "auto" || req.PathStyle {
		t.Fatalf("unexpected R2 normalization: %+v", req)
	}
}

func TestNormalizeBackupDestinationRejectsInsecureEndpoint(t *testing.T) {
	_, err := normalizeBackupDestination(backupDestinationRequest{
		Name: "S3", Provider: "s3", Endpoint: "http://storage.example.com", Region: "us-east-1",
		Bucket: "backups", AccessKey: "access", SecretKey: "secret",
	})
	if err == nil {
		t.Fatal("insecure S3 endpoint was accepted")
	}
}

func TestNormalizeR2DestinationPreservesJurisdictionEndpoint(t *testing.T) {
	req, err := normalizeBackupDestination(backupDestinationRequest{
		Name: "R2 EU", Provider: "r2", AccountID: "0123456789abcdef0123456789abcdef",
		Endpoint: "https://0123456789abcdef0123456789abcdef.eu.r2.cloudflarestorage.com",
		Bucket:   "backups", AccessKey: "access", SecretKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.Endpoint != "https://0123456789abcdef0123456789abcdef.eu.r2.cloudflarestorage.com" || req.Region != "auto" || req.PathStyle {
		t.Fatalf("unexpected jurisdictional R2 normalization: %+v", req)
	}
}

func TestNormalizeR2DestinationRejectsAnotherAccountEndpoint(t *testing.T) {
	_, err := normalizeBackupDestination(backupDestinationRequest{
		Name: "R2", Provider: "r2", AccountID: "0123456789abcdef0123456789abcdef",
		Endpoint: "https://ffffffffffffffffffffffffffffffff.r2.cloudflarestorage.com",
		Bucket:   "backups", AccessKey: "access", SecretKey: "secret",
	})
	if err == nil {
		t.Fatal("R2 endpoint for another account was accepted")
	}
}

func TestNormalizeS3DestinationRequiresKMSKeyForSSEKMS(t *testing.T) {
	_, err := normalizeBackupDestination(backupDestinationRequest{
		Name: "S3", Provider: "s3", Endpoint: "https://s3.example.com", Region: "us-east-1",
		Bucket: "backups", AccessKey: "access", SecretKey: "secret", ServerSideEncryption: "aws:kms",
	})
	if err == nil {
		t.Fatal("SSE-KMS destination without a key id was accepted")
	}
	req, err := normalizeBackupDestination(backupDestinationRequest{
		Name: "S3", Provider: "s3", Endpoint: "https://s3.example.com", Region: "us-east-1",
		Bucket: "backups", AccessKey: "access", SecretKey: "secret", ServerSideEncryption: "aws:kms", SSEKMSKeyID: "alias/hostforge-backups",
	})
	if err != nil || req.SSEKMSKeyID != "alias/hostforge-backups" {
		t.Fatalf("valid SSE-KMS destination was rejected: %+v err=%v", req, err)
	}
}
