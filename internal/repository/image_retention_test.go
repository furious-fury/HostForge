package repository

import (
	"context"
	"testing"
)

// ListRetainedImageRefs must keep the active image, the in-flight images, and
// the newest `retain` successes per binding -- and nothing older. It is the
// keep-set the image GC trusts, so every clause is pinned here.
func TestListRetainedImageRefs(t *testing.T) {
	store := newTestStore(t)
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
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/api.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}

	// Five successful deploys, oldest to newest. created_at must differ so the
	// window is deterministic; CreateServiceDeployment stamps now, so space
	// them with explicit ordering via the helper below.
	successRefs := []string{"hostforge/svc:s1", "hostforge/svc:s2", "hostforge/svc:s3", "hostforge/svc:s4", "hostforge/svc:s5"}
	for i, ref := range successRefs {
		dep := mustDeploy(t, store, service.ID, env, ref)
		// Stagger created_at so newest-N is unambiguous.
		if _, err := store.db.ExecContext(ctx, `UPDATE deployments SET created_at=? WHERE id=?`, "2026-09-0"+string(rune('1'+i))+"T00:00:00Z", dep); err != nil {
			t.Fatal(err)
		}
		if err := store.UpdateDeploymentStatus(ctx, dep, "SUCCESS", ""); err != nil {
			t.Fatal(err)
		}
	}

	// A BUILDING deploy: kept regardless of the window.
	building := mustDeploy(t, store, service.ID, env, "hostforge/svc:building")
	if err := store.UpdateDeploymentStatus(ctx, building, "BUILDING", ""); err != nil {
		t.Fatal(err)
	}

	// A FAILED deploy with a live container attached: kept via the container
	// clause even though it is terminal and outside the window.
	failedWithContainer := mustDeploy(t, store, service.ID, env, "hostforge/svc:failed-live")
	if _, err := store.AttachContainer(ctx, AttachContainerInput{DeploymentID: failedWithContainer, DockerContainerID: "docker-x", InternalPort: 3000, HostPort: 19000}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, failedWithContainer, "FAILED", "boom"); err != nil {
		t.Fatal(err)
	}

	keep, err := store.ListRetainedImageRefs(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}

	// Newest 3 successes (s5,s4,s3) kept; s2,s1 not. Building and failed-live kept.
	wantKept := []string{"hostforge/svc:s5", "hostforge/svc:s4", "hostforge/svc:s3", "hostforge/svc:building", "hostforge/svc:failed-live"}
	wantGone := []string{"hostforge/svc:s2", "hostforge/svc:s1"}
	for _, ref := range wantKept {
		if _, ok := keep[ref]; !ok {
			t.Fatalf("%s should be retained but was not: %v", ref, keep)
		}
	}
	for _, ref := range wantGone {
		if _, ok := keep[ref]; ok {
			t.Fatalf("%s should be reclaimable but was retained: %v", ref, keep)
		}
	}
	if len(keep) != len(wantKept) {
		t.Fatalf("keep-set size = %d, want %d: %v", len(keep), len(wantKept), keep)
	}
}

// retain=0 keeps only in-use and in-flight images, no success buffer.
func TestListRetainedImageRefsZeroRetainKeepsOnlyInUse(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	application, err := store.CreateApplication(ctx, "GCZero", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, application.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/api.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	success := mustDeploy(t, store, service.ID, environments[0].ID, "hostforge/svc:only-success")
	if err := store.UpdateDeploymentStatus(ctx, success, "SUCCESS", ""); err != nil {
		t.Fatal(err)
	}

	keep, err := store.ListRetainedImageRefs(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := keep["hostforge/svc:only-success"]; ok {
		t.Fatalf("retain=0 must not keep a success with no container: %v", keep)
	}
	if len(keep) != 0 {
		t.Fatalf("keep-set = %v, want empty", keep)
	}
}

func mustDeploy(t *testing.T, store *Store, serviceID, environmentID, imageRef string) string {
	t.Helper()
	dep, err := store.CreateServiceDeployment(context.Background(), CreateServiceDeploymentInput{ServiceID: serviceID, EnvironmentID: environmentID, ImageRef: imageRef})
	if err != nil {
		t.Fatal(err)
	}
	return dep.ID
}
