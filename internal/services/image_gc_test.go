package services

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/dockertest"
	"github.com/furious-fury/HostForge/internal/repository"
)

// The sweep removes deploy image tags no deployment still needs, keeps the
// live and in-flight ones, and treats an in-use image (409 from the daemon) as
// a skip rather than a failure. This drives the real store and a scripted
// Docker daemon so both the keep-set and the daemon's refusal are exercised.
func TestSweepUnreferencedImages(t *testing.T) {
	dbPath := t.TempDir() + "/hostforge.sqlite"
	db, err := database.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := repository.New(db)
	ctx := context.Background()

	application, err := store.CreateApplication(ctx, "GC", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, application.ID)
	if err != nil {
		t.Fatal(err)
	}
	env := environments[0].ID
	service, err := store.CreateService(ctx, repository.CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/api.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}

	// active: newest SUCCESS, live container -> kept.
	active, err := store.CreateServiceDeployment(ctx, repository.CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: env, ImageRef: "hostforge/svc:active"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AttachContainer(ctx, repository.AttachContainerInput{DeploymentID: active.ID, DockerContainerID: "docker-active", InternalPort: 3000, HostPort: 18080}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, active.ID, "SUCCESS", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateServiceDeployment(ctx, service.ID, env, active.ID); err != nil {
		t.Fatal(err)
	}
	// building: in flight -> kept.
	building, err := store.CreateServiceDeployment(ctx, repository.CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: env, ImageRef: "hostforge/svc:building"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, building.ID, "BUILDING", ""); err != nil {
		t.Fatal(err)
	}
	// old: FAILED with no container -> in no keep-set clause -> removed. (The
	// SUCCESS-window drop is pinned precisely in the repository test, which can
	// stagger created_at; here the point is the sweep's remove/skip behaviour.)
	old, err := store.CreateServiceDeployment(ctx, repository.CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: env, ImageRef: "hostforge/svc:old"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, old.ID, "FAILED", "superseded"); err != nil {
		t.Fatal(err)
	}

	// The daemon reports five hostforge/* tags. "dangling" has no deployment at
	// all; "inuse" is not in the keep-set but the daemon refuses it (409).
	listBody := `[` +
		`{"Id":"sha1","RepoTags":["hostforge/svc:active"]},` +
		`{"Id":"sha2","RepoTags":["hostforge/svc:building"]},` +
		`{"Id":"sha3","RepoTags":["hostforge/svc:old"]},` +
		`{"Id":"sha4","RepoTags":["hostforge/svc:dangling"]},` +
		`{"Id":"sha5","RepoTags":["hostforge/svc:inuse"]}` +
		`]`

	var mu sync.Mutex
	deleted := map[string]bool{}
	dockerClient := dockertest.NewClient(t, func(request *http.Request) (*http.Response, error) {
		path := request.URL.Path
		switch {
		case request.Method == http.MethodGet && strings.HasSuffix(path, "/images/json"):
			return dockertest.Response(request, http.StatusOK, listBody), nil
		case request.Method == http.MethodDelete && strings.Contains(path, "/images/"):
			switch {
			case strings.Contains(path, "active") || strings.Contains(path, "building"):
				t.Fatalf("GC deleted a retained image: %s", path)
			case strings.Contains(path, "inuse"):
				return dockertest.Response(request, http.StatusConflict, `{"message":"conflict: unable to delete: image is being used by running container"}`), nil
			case strings.Contains(path, "old") || strings.Contains(path, "dangling"):
				mu.Lock()
				if strings.Contains(path, "old") {
					deleted["old"] = true
				} else {
					deleted["dangling"] = true
				}
				mu.Unlock()
				return dockertest.Response(request, http.StatusOK, `[{"Untagged":"x"}]`), nil
			}
			t.Fatalf("unexpected delete: %s", path)
			return nil, nil
		default:
			t.Fatalf("unexpected docker call: %s %s", request.Method, path)
			return nil, nil
		}
	})

	removed, err := SweepUnreferencedImages(ctx, discardLog(), store, dockerClient, 1)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2 (old + dangling)", removed)
	}
	if !deleted["old"] || !deleted["dangling"] {
		t.Fatalf("expected old and dangling deleted, got %v", deleted)
	}
}
