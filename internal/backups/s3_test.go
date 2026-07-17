package backups

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type recordingHTTPClient struct {
	mu       sync.Mutex
	requests []*http.Request
	status   func(attempt int, request *http.Request) int
}

func (c *recordingHTTPClient) Do(request *http.Request) (*http.Response, error) {
	if request.Body != nil {
		_, _ = io.Copy(io.Discard, request.Body)
	}
	c.mu.Lock()
	c.requests = append(c.requests, request.Clone(request.Context()))
	attempt := len(c.requests)
	status := http.StatusOK
	if c.status != nil {
		status = c.status(attempt, request)
	}
	c.mu.Unlock()
	body := ""
	if status >= 400 {
		body = `<Error><Code>AccessDenied</Code><Message>denied</Message></Error>`
	}
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: http.Header{"Content-Length": []string{"0"}, "x-amz-request-id": []string{"test-request"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func testDestination(pathStyle bool) Destination {
	return Destination{Endpoint: "https://storage.example.test", Region: "us-east-1", Bucket: "backups", PathStyle: pathStyle, AccessKey: "access", SecretKey: "secret"}
}

func TestS3ClientUsesPathStyleAddressing(t *testing.T) {
	httpClient := &recordingHTTPClient{}
	client, err := newClient(context.Background(), testDestination(true), httpClient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Size(context.Background(), "database/object.hfbk"); err != nil {
		t.Fatal(err)
	}
	request := httpClient.requests[0]
	if request.URL.Host != "storage.example.test" || request.URL.Path != "/backups/database/object.hfbk" {
		t.Fatalf("unexpected path-style request: %s", request.URL.String())
	}
}

func TestS3ClientUsesVirtualHostedAddressing(t *testing.T) {
	httpClient := &recordingHTTPClient{}
	client, err := newClient(context.Background(), testDestination(false), httpClient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Size(context.Background(), "database/object.hfbk"); err != nil {
		t.Fatal(err)
	}
	request := httpClient.requests[0]
	if request.URL.Host != "backups.storage.example.test" || request.URL.Path != "/database/object.hfbk" {
		t.Fatalf("unexpected virtual-hosted request: %s", request.URL.String())
	}
}

func TestS3UploadRetriesInterruptedProviderResponse(t *testing.T) {
	httpClient := &recordingHTTPClient{status: func(attempt int, _ *http.Request) int {
		if attempt == 1 {
			return http.StatusServiceUnavailable
		}
		return http.StatusOK
	}}
	client, err := newClient(context.Background(), testDestination(true), httpClient)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Put(context.Background(), "retry.hfbk", bytes.NewReader([]byte("encrypted backup")), "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	if len(httpClient.requests) < 2 {
		t.Fatalf("upload was not retried after a transient provider failure: requests=%d", len(httpClient.requests))
	}
}

func TestS3DestinationProbeRejectsInsufficientPermissions(t *testing.T) {
	httpClient := &recordingHTTPClient{status: func(_ int, _ *http.Request) int { return http.StatusForbidden }}
	client, err := newClient(context.Background(), testDestination(true), httpClient)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Test(context.Background(), "probe"); err == nil {
		t.Fatal("destination probe accepted credentials without object permissions")
	}
}

func TestS3UploadAppliesConfiguredProviderSideEncryption(t *testing.T) {
	httpClient := &recordingHTTPClient{}
	destination := testDestination(true)
	destination.ServerSideEncryption = "aws:kms"
	destination.SSEKMSKeyID = "arn:aws:kms:us-east-1:123456789012:key/test"
	client, err := newClient(context.Background(), destination, httpClient)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Put(context.Background(), "encrypted.hfbk", bytes.NewReader([]byte("ciphertext")), "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	request := httpClient.requests[0]
	if request.Header.Get("x-amz-server-side-encryption") != "aws:kms" || request.Header.Get("x-amz-server-side-encryption-aws-kms-key-id") != destination.SSEKMSKeyID {
		t.Fatalf("SSE-KMS headers were not applied: %v", request.Header)
	}
}
