package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/moby/moby/client"
)

func newDockerHTTPTestClient(t *testing.T, transport roundTripFunc) *client.Client {
	t.Helper()
	dockerClient, err := client.New(
		client.WithHost("http://docker.test"),
		client.WithHTTPClient(&http.Client{Transport: transport}),
		client.WithVersion("1.47"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dockerClient.Close() })
	return dockerClient
}

func dockerResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Status: http.StatusText(status), Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(body)), Request: request,
	}
}

func TestEnsureEnvironmentNetworkCreatesOwnedBridge(t *testing.T) {
	var calls atomic.Int32
	dockerClient := newDockerHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		switch calls.Add(1) {
		case 1:
			if request.Method != http.MethodGet || !strings.Contains(request.URL.Path, "/networks/hostforge-env-env-1") {
				t.Fatalf("unexpected inspect request %s %s", request.Method, request.URL.Path)
			}
			return dockerResponse(request, http.StatusNotFound, `{"message":"not found"}`), nil
		case 2:
			if request.Method != http.MethodPost || request.URL.Path != "/v1.47/networks/create" {
				t.Fatalf("unexpected create request %s %s", request.Method, request.URL.Path)
			}
			var payload struct {
				Name   string            `json:"Name"`
				Driver string            `json:"Driver"`
				Labels map[string]string `json:"Labels"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Name != "hostforge-env-env-1" || payload.Driver != "bridge" ||
				payload.Labels[ManagedLabel] != "true" || payload.Labels[EnvironmentIDLabel] != "env-1" {
				t.Fatalf("unexpected network payload: %+v", payload)
			}
			return dockerResponse(request, http.StatusCreated, `{"Id":"network-1","Warning":""}`), nil
		default:
			t.Fatalf("unexpected extra Docker request")
			return nil, nil
		}
	})
	id, err := EnsureEnvironmentNetwork(context.Background(), dockerClient, "app-1", "env-1")
	if err != nil {
		t.Fatal(err)
	}
	if id != "network-1" {
		t.Fatalf("network id=%q", id)
	}
}

func TestRemoveEnvironmentNetworkIfEmptyRequiresOwnershipAndNoContainers(t *testing.T) {
	var calls atomic.Int32
	dockerClient := newDockerHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		switch calls.Add(1) {
		case 1:
			return dockerResponse(request, http.StatusOK, `{"Id":"network-1","Name":"hostforge-env-env-1","Labels":{"dev.hostforge.managed":"true","dev.hostforge.resource-type":"environment-network","dev.hostforge.environment-id":"env-1"},"Containers":{}}`), nil
		case 2:
			if request.Method != http.MethodDelete || !strings.Contains(request.URL.Path, "/networks/network-1") {
				t.Fatalf("unexpected network removal request %s %s", request.Method, request.URL.Path)
			}
			return dockerResponse(request, http.StatusNoContent, ""), nil
		default:
			t.Fatalf("unexpected extra Docker request")
			return nil, nil
		}
	})
	removed, err := RemoveEnvironmentNetworkIfEmpty(context.Background(), dockerClient, "env-1")
	if err != nil || !removed {
		t.Fatalf("empty owned network was not removed: removed=%v err=%v", removed, err)
	}
}

func TestRemoveEnvironmentNetworkIfEmptyRefusesUnownedNetwork(t *testing.T) {
	dockerClient := newDockerHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet {
			t.Fatalf("unexpected destructive request %s %s", request.Method, request.URL.Path)
		}
		return dockerResponse(request, http.StatusOK, `{"Id":"network-1","Name":"hostforge-env-env-1","Labels":{"owner":"customer"},"Containers":{}}`), nil
	})
	if _, err := RemoveEnvironmentNetworkIfEmpty(context.Background(), dockerClient, "env-1"); err == nil || !strings.Contains(err.Error(), "refusing to remove") {
		t.Fatalf("expected ownership refusal, got %v", err)
	}
}

func TestRemoveManagedVolumeRefusesUnownedVolume(t *testing.T) {
	dockerClient := newDockerHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet {
			t.Fatalf("unexpected destructive request %s %s", request.Method, request.URL.Path)
		}
		return dockerResponse(request, http.StatusOK, `{"Name":"customer-data","Labels":{"owner":"customer"}}`), nil
	})
	if err := RemoveManagedVolume(context.Background(), dockerClient, "customer-data"); err == nil ||
		!strings.Contains(err.Error(), "refusing to remove") {
		t.Fatalf("expected ownership refusal, got %v", err)
	}
}

func TestRemoveManagedDatabaseVolumeRefusesDifferentInstanceOwner(t *testing.T) {
	dockerClient := newDockerHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet {
			t.Fatalf("unexpected destructive request %s %s", request.Method, request.URL.Path)
		}
		return dockerResponse(request, http.StatusOK, `{"Name":"hostforge-db-safe","Labels":{"dev.hostforge.managed":"true","dev.hostforge.resource-type":"database-volume","dev.hostforge.database-instance-id":"instance-a"}}`), nil
	})
	if err := RemoveManagedDatabaseVolume(context.Background(), dockerClient, "hostforge-db-safe", "instance-b"); err == nil ||
		!strings.Contains(err.Error(), "mismatched database instance ownership") {
		t.Fatalf("expected instance ownership refusal, got %v", err)
	}
}

func TestEnsureManagedVolumeRefusesDifferentInstanceOwner(t *testing.T) {
	dockerClient := newDockerHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		return dockerResponse(request, http.StatusOK, `{"Name":"hostforge-db-safe","Labels":{"dev.hostforge.managed":"true","dev.hostforge.resource-type":"database-volume","dev.hostforge.database-instance-id":"instance-a"}}`), nil
	})
	if _, err := EnsureManagedVolume(context.Background(), dockerClient, "hostforge-db-safe", map[string]string{
		InstanceIDLabel: "instance-b",
	}); err == nil || !strings.Contains(err.Error(), "mismatched ownership label") {
		t.Fatalf("expected ownership mismatch, got %v", err)
	}
}

func TestRunManagedContainerHasNoPublishedPortsAndUsesLimits(t *testing.T) {
	var calls atomic.Int32
	dockerClient := newDockerHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		switch calls.Add(1) {
		case 1:
			if request.Method != http.MethodPost || !strings.Contains(request.URL.Path, "/containers/create") {
				t.Fatalf("unexpected create request %s %s", request.Method, request.URL.Path)
			}
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			hostConfig, ok := payload["HostConfig"].(map[string]any)
			if !ok {
				t.Fatalf("missing host config: %+v", payload)
			}
			if bindings, exists := hostConfig["PortBindings"]; exists && bindings != nil {
				t.Fatalf("database container published host ports: %+v", bindings)
			}
			if hostConfig["NanoCpus"] != float64(500_000_000) || hostConfig["Memory"] != float64(512*1024*1024) {
				t.Fatalf("resource limits missing: %+v", hostConfig)
			}
			if mounts, ok := hostConfig["Mounts"].([]any); !ok || len(mounts) != 1 {
				t.Fatalf("persistent mount missing: %+v", hostConfig["Mounts"])
			}
			networking, ok := payload["NetworkingConfig"].(map[string]any)
			if !ok || networking["EndpointsConfig"] == nil {
				t.Fatalf("private network missing: %+v", payload["NetworkingConfig"])
			}
			return dockerResponse(request, http.StatusCreated, `{"Id":"container-1","Warnings":[]}`), nil
		case 2:
			if request.Method != http.MethodPost || !strings.Contains(request.URL.Path, "/containers/container-1/start") {
				t.Fatalf("unexpected start request %s %s", request.Method, request.URL.Path)
			}
			return dockerResponse(request, http.StatusNoContent, ""), nil
		default:
			t.Fatalf("unexpected extra Docker request")
			return nil, nil
		}
	})
	id, err := RunManagedContainer(context.Background(), dockerClient, ManagedContainerOptions{
		ImageRef: "postgres@sha256:test", ContainerName: "hostforge-db-test",
		Env: []string{"POSTGRES_PASSWORD=secret"}, NetworkName: "hostforge-env-env-1",
		NetworkAliases: []string{"database"}, VolumeName: "hostforge-db-volume",
		VolumeTarget: "/var/lib/postgresql/data", CPULimitMillis: 500,
		MemoryLimitBytes: 512 * 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "container-1" {
		t.Fatalf("container id=%q", id)
	}
}

func TestReadContainerLogsReturnsBoundedDemultiplexedTail(t *testing.T) {
	var framed bytes.Buffer
	writer := stdcopy.NewStdWriter(&framed, stdcopy.Stdout)
	if _, err := writer.Write([]byte("database system is ready\n")); err != nil {
		t.Fatal(err)
	}
	dockerClient := newDockerHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.Query().Get("tail") != "25" || request.URL.Query().Get("timestamps") != "1" {
			t.Fatalf("unexpected log options: %s", request.URL.RawQuery)
		}
		response := dockerResponse(request, http.StatusOK, "")
		response.Body = io.NopCloser(bytes.NewReader(framed.Bytes()))
		return response, nil
	})
	logs, err := ReadContainerLogs(context.Background(), dockerClient, "container-1", 25)
	if err != nil {
		t.Fatal(err)
	}
	if logs != "database system is ready\n" {
		t.Fatalf("unexpected logs %q", logs)
	}
}

func TestInspectManagedContainerReturnsReconciliationInvariants(t *testing.T) {
	dockerClient := newDockerHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		return dockerResponse(request, http.StatusOK, `{"Id":"container-1","Config":{"Image":"postgres@sha256:pinned","Labels":{"dev.hostforge.managed":"true","dev.hostforge.resource-type":"database-container"}},"HostConfig":{"NanoCpus":500000000,"Memory":536870912,"PortBindings":{}},"Mounts":[{"Type":"volume","Name":"hostforge-db-test","Destination":"/var/lib/postgresql"}],"NetworkSettings":{"Networks":{"hostforge-env-env-1":{"Aliases":["postgres-staging"]}}},"State":{"Running":true,"Status":"running"}}`), nil
	})
	inspection, err := InspectManagedContainer(context.Background(), dockerClient, "container-1")
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ImageRef != "postgres@sha256:pinned" || inspection.NanoCPUs != 500_000_000 || inspection.MemoryBytes != 536_870_912 || inspection.PublishedPorts || inspection.VolumeMounts["hostforge-db-test"] != "/var/lib/postgresql" || len(inspection.NetworkAliases["hostforge-env-env-1"]) != 1 {
		t.Fatalf("managed inspection omitted a reconciliation invariant: %+v", inspection)
	}
}

func TestInspectManagedContainerExposesNotFoundForIdempotentCleanup(t *testing.T) {
	dockerClient := newDockerHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		return dockerResponse(request, http.StatusNotFound, `{"message":"No such container: missing"}`), nil
	})
	_, err := InspectManagedContainer(context.Background(), dockerClient, "missing")
	if err == nil || !IsNotFound(err) {
		t.Fatalf("expected a Docker not-found error, got %v", err)
	}
}
