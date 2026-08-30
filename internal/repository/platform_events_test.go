package repository

import (
	"context"
	"testing"
)

func TestPlatformEventFiltersAndCursorPagination(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	application, err := store.CreateApplication(ctx, "Events", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, eventType := range []string{"application", "domain", "deployment"} {
		if err := store.RecordPlatformEvent(ctx, PlatformEventInput{ApplicationID: application.ID, EventType: eventType, Message: eventType}); err != nil {
			t.Fatal(err)
		}
	}
	first, cursor, err := store.ListPlatformEventsFiltered(ctx, PlatformEventFilter{ApplicationID: application.ID, Limit: 2})
	if err != nil || len(first) != 2 || cursor == 0 {
		t.Fatalf("first=%+v cursor=%d err=%v", first, cursor, err)
	}
	second, next, err := store.ListPlatformEventsFiltered(ctx, PlatformEventFilter{ApplicationID: application.ID, Cursor: cursor, Limit: 2})
	if err != nil || len(second) != 1 || next != 0 || second[0].ID >= cursor {
		t.Fatalf("second=%+v next=%d err=%v", second, next, err)
	}
	filtered, _, err := store.ListPlatformEventsFiltered(ctx, PlatformEventFilter{ApplicationID: application.ID, EventType: "domain", Limit: 10})
	if err != nil || len(filtered) != 1 || filtered[0].EventType != "domain" {
		t.Fatalf("filtered=%+v err=%v", filtered, err)
	}
}

// Deployment status events build their message in SQL. SQLite's `+` is
// arithmetic, not concatenation, so `'Deployment '+lower(?)` coerces both
// operands to numbers and evaluates to 0 for every status. The activity feed
// renders that message directly, so every deployment event read "0".
func TestDeploymentEventMessagesAreConcatenated(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Concat", "")
	if err != nil {
		t.Fatal(err)
	}
	envs, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "api", RepoURL: "https://github.com/acme/concat.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: envs[0].ID})
	if err != nil {
		t.Fatal(err)
	}

	// RecordDeploymentEvent and UpdateDeploymentStatus each build the message
	// with their own copy of the expression, so both need covering.
	if err := store.RecordDeploymentEvent(ctx, deployment.ID, "BUILDING", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, deployment.ID, "SUCCESS", ""); err != nil {
		t.Fatal(err)
	}

	events, _, err := store.ListPlatformEventsFiltered(ctx, PlatformEventFilter{ApplicationID: app.ID, EventType: "deployment", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range events {
		got[e.Message] = true
	}
	for _, want := range []string{"Deployment building", "Deployment success"} {
		if !got[want] {
			t.Errorf("missing event message %q; got %v", want, keysOf(got))
		}
	}
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
