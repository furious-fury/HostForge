package repository

import (
	"context"
	"testing"
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
