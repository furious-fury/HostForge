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
