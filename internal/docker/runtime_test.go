package docker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func TestStopAndRemoveTreatsMissingContainerAsRemoved(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     "404 Not Found",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"message":"No such container: missing"}`)),
			Request:    request,
		}, nil
	})}
	dockerClient, err := client.New(
		client.WithHost("http://docker.test"),
		client.WithHTTPClient(httpClient),
		client.WithVersion("1.47"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer dockerClient.Close()

	if err := StopAndRemove(context.Background(), dockerClient, "missing"); err != nil {
		t.Fatalf("missing container cleanup should be idempotent: %v", err)
	}
}

// RunContainer applies fleet-wide resource limits and a hardening profile to
// every application container (ADR-0002 §14.2, §14.4). This asserts what
// actually goes out over the wire, since there is no way to inspect a real
// daemon's view of the container without one running.
func TestRunContainerAppliesResourceLimitsAndHardening(t *testing.T) {
	var captured struct {
		hostConfig container.HostConfig
	}
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/containers/create"):
			var body container.CreateRequest
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode create request: %v", err)
			}
			if body.HostConfig != nil {
				captured.hostConfig = *body.HostConfig
			}
			resp, _ := json.Marshal(container.CreateResponse{ID: "test-container-id"})
			return &http.Response{
				StatusCode: http.StatusCreated,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(string(resp))),
				Request:    request,
			}, nil
		case strings.HasSuffix(request.URL.Path, "/start"):
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    request,
			}, nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
			return nil, nil
		}
	})}
	dockerClient, err := client.New(
		client.WithHost("http://docker.test"),
		client.WithHTTPClient(httpClient),
		client.WithVersion("1.47"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer dockerClient.Close()

	_, err = RunContainer(context.Background(), dockerClient, RunOptions{
		ImageRef:         "example/app:latest",
		ContainerName:    "app-test",
		ContainerPort:    8080,
		HostPort:         30080,
		MemoryLimitBytes: 512 * 1024 * 1024,
		CPULimitMillis:   1000,
		PidsLimit:        512,
	})
	if err != nil {
		t.Fatalf("RunContainer: %v", err)
	}

	hc := captured.hostConfig
	if hc.Memory != 512*1024*1024 {
		t.Errorf("Memory = %d, want %d", hc.Memory, 512*1024*1024)
	}
	if hc.NanoCPUs != 1_000_000_000 {
		t.Errorf("NanoCPUs = %d, want %d", hc.NanoCPUs, int64(1_000_000_000))
	}
	if hc.PidsLimit == nil || *hc.PidsLimit != 512 {
		t.Errorf("PidsLimit = %v, want 512", hc.PidsLimit)
	}
	if len(hc.CapDrop) != 1 || hc.CapDrop[0] != "ALL" {
		t.Errorf("CapDrop = %v, want [ALL]", hc.CapDrop)
	}
	if len(hc.SecurityOpt) != 1 || hc.SecurityOpt[0] != "no-new-privileges" {
		t.Errorf("SecurityOpt = %v, want [no-new-privileges]", hc.SecurityOpt)
	}
	if hc.Tmpfs["/tmp"] == "" {
		t.Errorf("Tmpfs[/tmp] not set: %v", hc.Tmpfs)
	}
	if hc.ReadonlyRootfs {
		t.Error("ReadonlyRootfs must stay false for app containers (no per-service opt-in exists yet)")
	}
}

// 0 disables a limit, matching Docker's own "0 = unlimited" semantics — the
// escape hatch operators use if the fleet-wide default doesn't fit a
// specific workload. PidsLimit must be left as a nil pointer (Docker's
// "don't set" sentinel), not an explicit &0.
func TestRunContainerZeroLimitsMeanUnlimited(t *testing.T) {
	var captured struct {
		hostConfig container.HostConfig
	}
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/containers/create"):
			var body container.CreateRequest
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode create request: %v", err)
			}
			if body.HostConfig != nil {
				captured.hostConfig = *body.HostConfig
			}
			resp, _ := json.Marshal(container.CreateResponse{ID: "test-container-id"})
			return &http.Response{StatusCode: http.StatusCreated, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(resp))), Request: request}, nil
		case strings.HasSuffix(request.URL.Path, "/start"):
			return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
			return nil, nil
		}
	})}
	dockerClient, err := client.New(
		client.WithHost("http://docker.test"),
		client.WithHTTPClient(httpClient),
		client.WithVersion("1.47"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer dockerClient.Close()

	_, err = RunContainer(context.Background(), dockerClient, RunOptions{
		ImageRef:      "example/app:latest",
		ContainerName: "app-test-unlimited",
		ContainerPort: 8080,
		HostPort:      30081,
		// MemoryLimitBytes, CPULimitMillis, PidsLimit all left at zero value.
	})
	if err != nil {
		t.Fatalf("RunContainer: %v", err)
	}

	hc := captured.hostConfig
	if hc.Memory != 0 {
		t.Errorf("Memory = %d, want 0 (unlimited)", hc.Memory)
	}
	if hc.NanoCPUs != 0 {
		t.Errorf("NanoCPUs = %d, want 0 (unlimited)", hc.NanoCPUs)
	}
	if hc.PidsLimit != nil {
		t.Errorf("PidsLimit = %v, want nil (unset, not &0)", *hc.PidsLimit)
	}
}
