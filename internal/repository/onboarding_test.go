package repository

import (
	"context"
	"testing"
	"time"
)

func TestOnboardingStateTransitions(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	state, err := store.GetOnboardingState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.GitHubAppComplete || state.BootstrapComplete {
		t.Fatalf("unexpected initial state: %+v", state)
	}
	if err := store.CompleteOnboarding(ctx, "forge.example.com"); err == nil {
		t.Fatal("completion without GitHub App must fail")
	}
	if err := store.MarkGitHubAppComplete(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteOnboarding(ctx, "forge.example.com"); err != nil {
		t.Fatal(err)
	}
	state, err = store.GetOnboardingState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !state.GitHubAppComplete || !state.PermanentIngressComplete || !state.BootstrapComplete || state.PlatformDomain != "forge.example.com" {
		t.Fatalf("unexpected completed state: %+v", state)
	}
}

func TestOnboardingRecognizesExistingGitHubApp(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO github_app(id,app_id,slug,html_url,client_id,client_secret_ct,private_key_ct,webhook_secret_ct,created_at,updated_at)
		VALUES(1,42,'hostforge-test','https://github.com/apps/hostforge-test','client',X'01',X'02',X'03',?,?)`,
		now, now); err != nil {
		t.Fatal(err)
	}

	state, err := store.GetOnboardingState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !state.GitHubAppComplete {
		t.Fatal("existing GitHub App should satisfy onboarding even when the legacy flag is stale")
	}
	if err := store.CompleteOnboarding(ctx, "forge.example.com"); err != nil {
		t.Fatal(err)
	}
}
