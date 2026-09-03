package services

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/docker"
	"github.com/furious-fury/HostForge/internal/dockertest"
	"github.com/furious-fury/HostForge/internal/repository"
)

// seedSweepFixture builds one service with a live active SUCCESS deployment
// (docker-live) and one FAILED non-active deployment whose container is still
// RUNNING (docker-orphan) -- the state a SIGKILL mid-deploy leaves behind. It
// returns the store and the orphan's service and environment ids, which the
// fake needs to render matching ownership labels.
func seedSweepFixture(t *testing.T) (store *repository.Store, serviceID, environmentID string) {
	t.Helper()
	dbPath := t.TempDir() + "/hostforge.sqlite"
	db, err := database.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store = repository.New(db)
	ctx := context.Background()

	application, err := store.CreateApplication(ctx, "Sweep", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, application.ID)
	if err != nil {
		t.Fatal(err)
	}
	environmentID = environments[0].ID
	service, err := store.CreateService(ctx, repository.CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/api.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	serviceID = service.ID

	live, err := store.CreateServiceDeployment(ctx, repository.CreateServiceDeploymentInput{ServiceID: serviceID, EnvironmentID: environmentID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AttachContainer(ctx, repository.AttachContainerInput{DeploymentID: live.ID, DockerContainerID: "docker-live", InternalPort: 3000, HostPort: 18080}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, live.ID, "SUCCESS", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateServiceDeployment(ctx, serviceID, environmentID, live.ID); err != nil {
		t.Fatal(err)
	}

	orphan, err := store.CreateServiceDeployment(ctx, repository.CreateServiceDeploymentInput{ServiceID: serviceID, EnvironmentID: environmentID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AttachContainer(ctx, repository.AttachContainerInput{DeploymentID: orphan.ID, DockerContainerID: "docker-orphan", InternalPort: 3000, HostPort: 18081}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, orphan.ID, "FAILED", "interrupted"); err != nil {
		t.Fatal(err)
	}
	return store, serviceID, environmentID
}

// inspectBody renders a container inspect response with the given labels.
func inspectBody(id, resourceType, serviceID, environmentID string) string {
	return fmt.Sprintf(`{"Id":%q,"Config":{"Image":"hostforge/app:tag","Labels":{%q:"true",%q:%q,%q:%q,%q:%q}},"State":{"Running":true,"Status":"running"}}`,
		id,
		docker.ManagedLabel,
		docker.ResourceTypeLabel, resourceType,
		docker.ServiceIDLabel, serviceID,
		docker.EnvironmentIDLabel, environmentID,
	)
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestSweepRemovesTheOrphanAndSparesTheLiveContainer(t *testing.T) {
	store, serviceID, environmentID := seedSweepFixture(t)

	var stopped, removed bool
	dockerClient := dockertest.NewClient(t, func(request *http.Request) (*http.Response, error) {
		path := request.URL.Path
		if strings.Contains(path, "docker-live") {
			t.Fatalf("sweep touched the live container: %s %s", request.Method, path)
		}
		switch {
		case request.Method == http.MethodGet && strings.HasSuffix(path, "/containers/docker-orphan/json"):
			return dockertest.Response(request, http.StatusOK, inspectBody("docker-orphan", "application-container", serviceID, environmentID)), nil
		case request.Method == http.MethodPost && strings.Contains(path, "/containers/docker-orphan/stop"):
			stopped = true
			return dockertest.Response(request, http.StatusNoContent, ""), nil
		case request.Method == http.MethodDelete && strings.HasSuffix(path, "/containers/docker-orphan"):
			removed = true
			return dockertest.Response(request, http.StatusNoContent, ""), nil
		default:
			t.Fatalf("unexpected docker call: %s %s", request.Method, path)
			return nil, nil
		}
	})

	count, err := SweepOrphanedDeployContainers(context.Background(), discardLog(), store, dockerClient)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("removed = %d, want 1", count)
	}
	if !stopped || !removed {
		t.Fatalf("orphan not stopped+removed: stopped=%v removed=%v", stopped, removed)
	}

	// The orphan row is now REMOVED, so it no longer selects; the live one was
	// never a candidate. Zero remaining proves the row was marked, not merely
	// that the container was stopped.
	sweepable, err := store.ListSweepableDeployContainers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sweepable) != 0 {
		t.Fatalf("orphan still sweepable after removal: %+v", sweepable)
	}
}

// A container whose live labels do not match its row is never removed: the
// sweep removes only what it can positively identify as its own.
func TestSweepLeavesMislabelledContainerAlone(t *testing.T) {
	store, _, _ := seedSweepFixture(t)

	dockerClient := dockertest.NewClient(t, func(request *http.Request) (*http.Response, error) {
		path := request.URL.Path
		if request.Method == http.MethodGet && strings.HasSuffix(path, "/containers/docker-orphan/json") {
			// Managed, but a database container -- wrong resource type.
			return dockertest.Response(request, http.StatusOK, inspectBody("docker-orphan", "database-container", "someone-else", "elsewhere")), nil
		}
		t.Fatalf("sweep tried to act on a mislabelled container: %s %s", request.Method, path)
		return nil, nil
	})

	count, err := SweepOrphanedDeployContainers(context.Background(), discardLog(), store, dockerClient)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("removed = %d, want 0", count)
	}
	// The row is untouched, so it still selects as sweepable.
	sweepable, err := store.ListSweepableDeployContainers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sweepable) != 1 {
		t.Fatalf("mislabelled container's row was disturbed: sweepable=%+v", sweepable)
	}
}
