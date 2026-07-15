package authresolver

import (
	"context"
	"errors"
	"testing"
)

type fakeApp struct {
	token string
	err   error
}

func (f fakeApp) MintInstallationToken(_ context.Context, _ int64) (InstallationToken, error) {
	if f.err != nil {
		return InstallationToken{}, f.err
	}
	return InstallationToken{Token: f.token}, nil
}

func TestResolveInstallationAuth(t *testing.T) {
	resolver := New(fakeApp{token: "ghs_install"})
	result, err := resolver.ResolveForRepoAccess(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != SourceGitHubApp || result.Auth.GitHubInstallationToken != "ghs_install" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestResolveForRepoAccessWithoutInstallationIsPublic(t *testing.T) {
	result, err := New(fakeApp{token: "unused"}).ResolveForRepoAccess(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != SourceNone || result.Auth.GitHubInstallationToken != "" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestResolveForRepoAccessPropagatesMintError(t *testing.T) {
	_, err := New(fakeApp{err: errors.New("boom")}).ResolveForRepoAccess(context.Background(), 7)
	if err == nil {
		t.Fatal("expected mint error")
	}
}
