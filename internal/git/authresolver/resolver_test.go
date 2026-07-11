package authresolver

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/hostforge/hostforge/internal/models"
)

type fakeSealer struct{}

func (fakeSealer) Open(b []byte) ([]byte, error) { return b, nil }

type fakeStore struct {
	pat *models.ProjectGitAuthSealed
	ssh *models.ProjectSSHKeySealed
}

func (f fakeStore) GetProjectGitAuthSealed(_ context.Context, _ string) (models.ProjectGitAuthSealed, error) {
	if f.pat == nil {
		return models.ProjectGitAuthSealed{}, sql.ErrNoRows
	}
	return *f.pat, nil
}
func (f fakeStore) GetProjectSSHKeySealed(_ context.Context, _ string) (models.ProjectSSHKeySealed, error) {
	if f.ssh == nil {
		return models.ProjectSSHKeySealed{}, sql.ErrNoRows
	}
	return *f.ssh, nil
}

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

func TestResolve_Priority_AppBeatsEverything(t *testing.T) {
	store := fakeStore{
		pat: &models.ProjectGitAuthSealed{ProjectID: "p", Provider: "github", TokenCT: []byte("pat-token")},
		ssh: &models.ProjectSSHKeySealed{ProjectID: "p", PublicKey: "pub", PrivateKeyCT: []byte("----KEY----")},
	}
	r := New(store, fakeSealer{}, fakeApp{token: "ghs_install"})
	res, err := r.Resolve(context.Background(), models.Project{
		ID:                   "p",
		GitSource:            models.GitSourceGitHubApp,
		GitHubInstallationID: 42,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceGitHubApp {
		t.Fatalf("source=%s", res.Source)
	}
	if res.Auth.GitHubToken != "ghs_install" {
		t.Fatalf("token=%q", res.Auth.GitHubToken)
	}
}

func TestResolve_PATFallback(t *testing.T) {
	store := fakeStore{
		pat: &models.ProjectGitAuthSealed{ProjectID: "p", Provider: "github", TokenCT: []byte("ghp_pat")},
	}
	r := New(store, fakeSealer{}, nil)
	res, err := r.Resolve(context.Background(), models.Project{ID: "p", GitSource: models.GitSourceURL})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourcePAT || res.Auth.GitHubToken != "ghp_pat" {
		t.Fatalf("unexpected: %+v", res)
	}
}

func TestResolve_SSHFallback(t *testing.T) {
	store := fakeStore{
		ssh: &models.ProjectSSHKeySealed{ProjectID: "p", PublicKey: "pub", PrivateKeyCT: []byte("---PRIV---")},
	}
	r := New(store, fakeSealer{}, nil)
	res, err := r.Resolve(context.Background(), models.Project{ID: "p", GitSource: models.GitSourceSSH})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceSSH || string(res.Auth.SSHPrivateKeyPEM) != "---PRIV---" {
		t.Fatalf("unexpected: %+v", res)
	}
	if !res.Auth.SSHInsecureIgnoreHostKey {
		t.Fatalf("default should ignore host key")
	}
}

func TestResolve_NoneWhenNoCreds(t *testing.T) {
	r := New(fakeStore{}, fakeSealer{}, nil)
	res, err := r.Resolve(context.Background(), models.Project{ID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceNone {
		t.Fatalf("source=%s", res.Source)
	}
}

func TestResolve_AppMintErrorFailsFast(t *testing.T) {
	r := New(fakeStore{}, fakeSealer{}, fakeApp{err: errors.New("boom")})
	_, err := r.Resolve(context.Background(), models.Project{
		ID:                   "p",
		GitSource:            models.GitSourceGitHubApp,
		GitHubInstallationID: 7,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolve_AppEmptyTokenFallsBackToPAT(t *testing.T) {
	store := fakeStore{pat: &models.ProjectGitAuthSealed{ProjectID: "p", Provider: "github", TokenCT: []byte("ghp")}}
	r := New(store, fakeSealer{}, fakeApp{token: ""})
	res, err := r.Resolve(context.Background(), models.Project{
		ID:                   "p",
		GitSource:            models.GitSourceGitHubApp,
		GitHubInstallationID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourcePAT {
		t.Fatalf("want pat fallback, got %s", res.Source)
	}
}

func TestWithHostKeyCheck_EnablesVerification(t *testing.T) {
	store := fakeStore{
		ssh: &models.ProjectSSHKeySealed{ProjectID: "p", PrivateKeyCT: []byte("k")},
	}
	r := New(store, fakeSealer{}, nil).WithHostKeyCheck(true)
	res, err := r.Resolve(context.Background(), models.Project{ID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Auth.SSHInsecureIgnoreHostKey {
		t.Fatal("expected host-key verification enabled")
	}
}

func TestResolveForRepoAccess_App(t *testing.T) {
	r := New(nil, nil, fakeApp{token: "tok"})
	res, err := r.ResolveForRepoAccess(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceGitHubApp || res.Auth.GitHubToken != "tok" {
		t.Fatalf("unexpected: %+v", res)
	}
	res, err = r.ResolveForRepoAccess(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceNone {
		t.Fatalf("want none, got %s", res.Source)
	}
}
