package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	githubapp "github.com/hostforge/hostforge/internal/github/app"
	"github.com/hostforge/hostforge/internal/git"
	"github.com/hostforge/hostforge/internal/git/authresolver"
	"github.com/hostforge/hostforge/internal/models"
)

// appClientHolder lazily loads the per-App RSA key from the sealed DB row and
// caches the resulting *app.Client. It is invalidated when the App config is
// upserted or deleted.
type appClientHolder struct {
	mu      sync.Mutex
	client  atomic.Pointer[githubapp.Client]
	loadErr error
	appID   int64
}

// loadAppClient returns a *githubapp.Client derived from the singleton github_app
// row, or (nil, nil) when the app has not been configured yet. Callers may
// treat (nil, nil) as "no app auth available".
func (s *server) loadAppClient(ctx context.Context) (*githubapp.Client, error) {
	if s.appCache == nil {
		s.appCache = &appClientHolder{}
	}
	if c := s.appCache.client.Load(); c != nil {
		return c, nil
	}
	s.appCache.mu.Lock()
	defer s.appCache.mu.Unlock()
	if c := s.appCache.client.Load(); c != nil {
		return c, nil
	}
	sealed, err := s.store.GetGitHubAppSealed(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load github app: %w", err)
	}
	if s.envSealer == nil {
		return nil, fmt.Errorf("github app configured but env encryption key is not set")
	}
	pemBytes, err := s.envSealer.Open(sealed.PrivateKeyCT)
	if err != nil {
		return nil, fmt.Errorf("decrypt app private key: %w", err)
	}
	key, err := githubapp.ParsePrivateKeyPEM(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("parse app private key: %w", err)
	}
	cli, err := githubapp.New(githubapp.Config{
		AppID:      sealed.AppID,
		PrivateKey: key,
	})
	if err != nil {
		return nil, err
	}
	s.appCache.client.Store(cli)
	s.appCache.appID = sealed.AppID
	return cli, nil
}

// invalidateAppClient forgets the cached *githubapp.Client so the next loader
// call picks up new sealed material or clears itself when the row is gone.
func (s *server) invalidateAppClient() {
	if s.appCache == nil {
		return
	}
	s.appCache.client.Store(nil)
}

// appTokenProviderFor returns an AppTokenProvider usable by the authresolver,
// built from the configured app client, or nil if no app is configured.
func (s *server) appTokenProviderFor(ctx context.Context) authresolver.AppTokenProvider {
	cli, err := s.loadAppClient(ctx)
	if err != nil || cli == nil {
		return nil
	}
	return appTokenAdapter{cli: cli}
}

// appTokenAdapter narrows the github/app.Client method set to the authresolver
// interface.
type appTokenAdapter struct{ cli *githubapp.Client }

func (a appTokenAdapter) MintInstallationToken(ctx context.Context, installationID int64) (authresolver.InstallationToken, error) {
	tok, err := a.cli.MintInstallationToken(ctx, installationID)
	if err != nil {
		return authresolver.InstallationToken{}, err
	}
	return authresolver.InstallationToken{Token: tok.Token}, nil
}

// newGitAuthResolver builds a resolver wired with the current App + sealer
// state. It implements services.GitAuthResolver.
func (s *server) newGitAuthResolver(ctx context.Context) *authresolver.Resolver {
	var sealer authresolver.Sealer
	if s.envSealer != nil {
		sealer = s.envSealer
	}
	return authresolver.New(s.store, sealer, s.appTokenProviderFor(ctx))
}

// resolveGitAuthForProject returns the best credentials for project. This is
// used by endpoints that need git.AuthOptions before calling ExecuteDeploy or
// listing remote branches.
func (s *server) resolveGitAuthForProject(ctx context.Context, project models.Project) (git.AuthOptions, error) {
	return s.newGitAuthResolver(ctx).ResolveAuthOptions(ctx, project)
}

// resolveGitAuthForInstallation mints credentials for a raw installation id,
// useful before a project row exists (wizard branch listing).
func (s *server) resolveGitAuthForInstallation(ctx context.Context, installationID int64) (git.AuthOptions, error) {
	res, err := s.newGitAuthResolver(ctx).ResolveForRepoAccess(ctx, installationID)
	if err != nil {
		return git.AuthOptions{}, err
	}
	return res.Auth, nil
}

// redactToken masks an oauth-like token value for logs.
func redactToken(t string) string {
	t = strings.TrimSpace(t)
	if len(t) <= 8 {
		return "***"
	}
	return t[:4] + "…" + t[len(t)-4:]
}
