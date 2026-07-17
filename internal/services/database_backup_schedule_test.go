package services

import (
	"testing"
	"time"
)

func TestNextDatabaseBackupScheduleUsesConfiguredTimezone(t *testing.T) {
	after := time.Date(2026, 7, 16, 22, 0, 0, 0, time.UTC)
	next, err := NextDatabaseBackupSchedule("0 2 * * *", "Africa/Lagos", after)
	if err != nil {
		t.Fatal(err)
	}
	// 02:00 in Lagos is 01:00 UTC.
	want := time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next=%s want=%s", next, want)
	}
}

func TestNextDatabaseBackupScheduleRejectsInvalidInput(t *testing.T) {
	if _, err := NextDatabaseBackupSchedule("not a cron", "UTC", time.Now()); err == nil {
		t.Fatal("invalid cron expression was accepted")
	}
	if _, err := NextDatabaseBackupSchedule("0 2 * * *", "Mars/Olympus", time.Now()); err == nil {
		t.Fatal("invalid timezone was accepted")
	}
}
