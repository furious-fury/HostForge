package obs

import (
	"context"
	"testing"

	"github.com/furious-fury/HostForge/internal/models"
)

func TestWithStore_nilSafe(t *testing.T) {
	ctx := WithStore(context.Background(), nil)
	if StoreFrom(ctx) != nil {
		t.Fatal("expected nil store")
	}
}

// recordingWriter captures what a record call actually reached the store
// with, including the context, so a test can assert on cancellation state
// rather than on the return value -- these writers are best-effort and
// report nothing to their caller.
type recordingWriter struct {
	deploySteps  []models.DeployStepRecord
	httpRequests []models.HTTPRequestRecord
	seenCtxErr   error
}

func (w *recordingWriter) InsertDeployStep(ctx context.Context, in models.DeployStepRecord) error {
	w.seenCtxErr = ctx.Err()
	// Behave as the SQLite driver does: a cancelled context fails the
	// insert outright, which is how this was first seen in production.
	if err := ctx.Err(); err != nil {
		return err
	}
	w.deploySteps = append(w.deploySteps, in)
	return nil
}

func (w *recordingWriter) InsertHTTPRequest(ctx context.Context, in models.HTTPRequestRecord) error {
	w.seenCtxErr = ctx.Err()
	if err := ctx.Err(); err != nil {
		return err
	}
	w.httpRequests = append(w.httpRequests, in)
	return nil
}

// A cancelled deploy is exactly when its record matters most: it is the only
// evidence of where the deploy stopped and how long the teardown took. When
// the write inherited the deploy's own context, every step recorded after a
// cancel was dropped by the database driver before it could be stored --
// observed on a real host as "insert deploy_step: context canceled" for both
// health_check and deploy_total on a cancelled deploy.
func TestRecordDeployStepWritesAfterTheOperationIsCancelled(t *testing.T) {
	writer := &recordingWriter{}
	ctx, cancel := context.WithCancel(WithStore(context.Background(), writer))
	cancel()

	RecordDeployStep(ctx, nil, models.DeployStepRecord{Step: "health_check", Status: "failed"})

	if len(writer.deploySteps) != 1 {
		t.Fatalf("deploy_step rows written = %d, want 1: the record was discarded because the deploy was cancelled", len(writer.deploySteps))
	}
	if writer.seenCtxErr != nil {
		t.Fatalf("store saw ctx.Err() = %v, want nil: the write must not carry the cancellation it is describing", writer.seenCtxErr)
	}
}

// Same defect, same fix, other writer: a client that disconnects cancels the
// request context, which is precisely the request worth having a sample of.
func TestRecordHTTPRequestWritesAfterTheRequestIsCancelled(t *testing.T) {
	writer := &recordingWriter{}
	ctx, cancel := context.WithCancel(WithStore(context.Background(), writer))
	cancel()

	RecordHTTPRequest(ctx, nil, models.HTTPRequestRecord{Method: "GET", Path: "/api/x", Status: 200})

	if len(writer.httpRequests) != 1 {
		t.Fatalf("http_request rows written = %d, want 1", len(writer.httpRequests))
	}
	if writer.seenCtxErr != nil {
		t.Fatalf("store saw ctx.Err() = %v, want nil", writer.seenCtxErr)
	}
}

// Detaching must not also detach the values: the writer itself is carried on
// the context, so a WithoutCancel that dropped values would silently turn
// every record into a no-op.
func TestRecordDeployStepKeepsContextValues(t *testing.T) {
	writer := &recordingWriter{}
	ctx := WithStore(context.Background(), writer)

	RecordDeployStep(ctx, nil, models.DeployStepRecord{Step: "clone", Status: "ok"})

	if len(writer.deploySteps) != 1 {
		t.Fatalf("deploy_step rows written = %d, want 1", len(writer.deploySteps))
	}
}
