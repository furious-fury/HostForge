package main

import (
	"context"
	"database/sql"
	"errors"

	"github.com/hostforge/hostforge/internal/crypto/envcrypt"
	"github.com/hostforge/hostforge/internal/git/authresolver"
	githubapp "github.com/hostforge/hostforge/internal/github/app"
	"github.com/hostforge/hostforge/internal/repository"
)

// cliGitAuthResolver returns a services.GitAuthResolver wired to local DB
// credentials: GitHub App installation tokens (if an App is configured on this
// host), per-project PAT, per-project SSH deploy key, then public.
//
// The CLI talks directly to the local sqlite (see cmd/cli/main.go), so it
// mirrors the server's resolver construction rather than going through the
// HTTP API.
func cliGitAuthResolver(ctx context.Context, store *repository.Store, sealer *envcrypt.Sealer) *authresolver.Resolver {
	var app authresolver.AppTokenProvider
	if cli, err := loadAppClientForCLI(ctx, store, sealer); err == nil && cli != nil {
		app = cliAppAdapter{cli: cli}
	}
	var s authresolver.Sealer
	if sealer != nil {
		s = sealer
	}
	return authresolver.New(store, s, app)
}

// loadAppClientForCLI mirrors cmd/server.loadAppClient but has no cache: the
// CLI process is short-lived so a per-run construction is fine.
func loadAppClientForCLI(ctx context.Context, store *repository.Store, sealer *envcrypt.Sealer) (*githubapp.Client, error) {
	sealed, err := store.GetGitHubAppSealed(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if sealer == nil {
		return nil, errors.New("github app configured but env encryption key is not set")
	}
	pemBytes, err := sealer.Open(sealed.PrivateKeyCT)
	if err != nil {
		return nil, err
	}
	key, err := githubapp.ParsePrivateKeyPEM(pemBytes)
	if err != nil {
		return nil, err
	}
	return githubapp.New(githubapp.Config{AppID: sealed.AppID, PrivateKey: key})
}

type cliAppAdapter struct{ cli *githubapp.Client }

func (a cliAppAdapter) MintInstallationToken(ctx context.Context, installationID int64) (authresolver.InstallationToken, error) {
	tok, err := a.cli.MintInstallationToken(ctx, installationID)
	if err != nil {
		return authresolver.InstallationToken{}, err
	}
	return authresolver.InstallationToken{Token: tok.Token}, nil
}
