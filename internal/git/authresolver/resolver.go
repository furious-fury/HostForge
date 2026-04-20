// Package authresolver picks the best per-project git transport credentials
// from the configured sources, in priority order: GitHub App installation
// token > per-project PAT > per-project SSH deploy key > empty (public).
//
// Store, Sealer and AppProvider are small interfaces so CLI and server code
// can both depend on this package without importing the concrete services.
package authresolver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hostforge/hostforge/internal/git"
	"github.com/hostforge/hostforge/internal/models"
)

// Sealer decrypts blobs sealed by internal/crypto/envcrypt.
type Sealer interface {
	Open(sealed []byte) ([]byte, error)
}

// Store is the subset of repository.Store needed by the resolver.
type Store interface {
	GetProjectGitAuthSealed(ctx context.Context, projectID string) (models.ProjectGitAuthSealed, error)
	GetProjectSSHKeySealed(ctx context.Context, projectID string) (models.ProjectSSHKeySealed, error)
}

// AppTokenProvider mints installation access tokens for a given installation id.
// The resolver treats a nil provider or a zero installation id as "no app auth available".
type AppTokenProvider interface {
	MintInstallationToken(ctx context.Context, installationID int64) (InstallationToken, error)
}

// InstallationToken is the minimal subset of the GitHub App installation token
// the resolver needs. This mirrors internal/github/app.InstallationToken so we
// don't import that package directly here.
type InstallationToken struct {
	Token string
}

// Source identifies which credential mode the resolver ended up using. It is
// useful for logs and UI indicators.
type Source string

const (
	// SourceNone means no authentication material was applied (public repo).
	SourceNone Source = "none"
	// SourceGitHubApp means an installation access token was minted.
	SourceGitHubApp Source = "github_app"
	// SourcePAT means a stored PAT was used.
	SourcePAT Source = "pat"
	// SourceSSH means a per-project SSH deploy key was used.
	SourceSSH Source = "ssh"
)

// Options controls resolution behavior.
type Options struct {
	// DisableSSHHostKeyCheck, when true, passes AuthOptions.SSHInsecureIgnoreHostKey.
	// Defaults to true for MVP; callers can explicitly set false to disable the flag.
	DisableSSHHostKeyCheck bool
}

// Result pairs the derived git.AuthOptions with the winning Source.
type Result struct {
	Auth   git.AuthOptions
	Source Source
}

// Resolver glues the three credential backends together.
type Resolver struct {
	store    Store
	sealer   Sealer
	app      AppTokenProvider
	insecure bool
}

// New builds a Resolver. Any of store/sealer/app may be nil, in which case the
// corresponding credential source is skipped.
func New(store Store, sealer Sealer, app AppTokenProvider) *Resolver {
	return &Resolver{
		store:    store,
		sealer:   sealer,
		app:      app,
		insecure: true,
	}
}

// WithHostKeyCheck toggles ssh host-key verification. The default is insecure
// (no verification) so operators don't need to pre-populate known_hosts on a
// fresh machine; callers that know what they're doing may enable verification.
func (r *Resolver) WithHostKeyCheck(enable bool) *Resolver {
	if r != nil {
		r.insecure = !enable
	}
	return r
}

// Resolve returns the credentials to use for cloning/listing project's repo.
//
// Priority:
//  1. If project.GitSource == github_app and an installation id + AppProvider
//     are available, mint an installation token.
//  2. Otherwise try PAT (any GitSource value).
//  3. Otherwise try SSH.
//  4. Otherwise return empty options (public).
func (r *Resolver) Resolve(ctx context.Context, project models.Project) (Result, error) {
	if r == nil {
		return Result{Source: SourceNone}, nil
	}
	gs := strings.TrimSpace(project.GitSource)
	if gs == "" {
		gs = models.GitSourceURL
	}

	if gs == models.GitSourceGitHubApp && r.app != nil && project.GitHubInstallationID > 0 {
		tok, err := r.app.MintInstallationToken(ctx, project.GitHubInstallationID)
		if err != nil {
			return Result{}, fmt.Errorf("mint installation token: %w", err)
		}
		if strings.TrimSpace(tok.Token) != "" {
			return Result{
				Auth:   git.AuthOptions{GitHubToken: tok.Token},
				Source: SourceGitHubApp,
			}, nil
		}
	}

	if r.store != nil && r.sealer != nil {
		patRow, err := r.store.GetProjectGitAuthSealed(ctx, project.ID)
		switch {
		case err == nil:
			if strings.ToLower(strings.TrimSpace(patRow.Provider)) != "github" {
				return Result{}, fmt.Errorf("unsupported git auth provider %q", patRow.Provider)
			}
			pt, err := r.sealer.Open(patRow.TokenCT)
			if err != nil {
				return Result{}, fmt.Errorf("decrypt git pat: %w", err)
			}
			if tok := strings.TrimSpace(string(pt)); tok != "" {
				return Result{
					Auth:   git.AuthOptions{GitHubToken: tok},
					Source: SourcePAT,
				}, nil
			}
		case errors.Is(err, sql.ErrNoRows):
			// fall through
		default:
			return Result{}, fmt.Errorf("lookup project git auth: %w", err)
		}

		sshRow, err := r.store.GetProjectSSHKeySealed(ctx, project.ID)
		switch {
		case err == nil:
			pt, err := r.sealer.Open(sshRow.PrivateKeyCT)
			if err != nil {
				return Result{}, fmt.Errorf("decrypt project ssh key: %w", err)
			}
			if len(pt) > 0 {
				return Result{
					Auth: git.AuthOptions{
						SSHPrivateKeyPEM:         pt,
						SSHUser:                  "git",
						SSHInsecureIgnoreHostKey: r.insecure,
					},
					Source: SourceSSH,
				}, nil
			}
		case errors.Is(err, sql.ErrNoRows):
			// fall through
		default:
			return Result{}, fmt.Errorf("lookup project ssh key: %w", err)
		}
	}

	return Result{Source: SourceNone}, nil
}

// ResolveAuthOptions is a convenience shim that returns only the git.AuthOptions,
// suitable for adapting the Resolver to a `func(ctx, project) (git.AuthOptions, error)`
// signature without callers importing this package's Result type.
func (r *Resolver) ResolveAuthOptions(ctx context.Context, project models.Project) (git.AuthOptions, error) {
	res, err := r.Resolve(ctx, project)
	if err != nil {
		return git.AuthOptions{}, err
	}
	return res.Auth, nil
}

// ResolveForRepoAccess is a convenience for callers that only have a
// repository URL (e.g. wizard branch-listing before a project is persisted).
// It picks app auth when installationID is > 0 and app provider is configured;
// otherwise returns empty options (public).
func (r *Resolver) ResolveForRepoAccess(ctx context.Context, installationID int64) (Result, error) {
	if r == nil || r.app == nil || installationID <= 0 {
		return Result{Source: SourceNone}, nil
	}
	tok, err := r.app.MintInstallationToken(ctx, installationID)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(tok.Token) == "" {
		return Result{Source: SourceNone}, nil
	}
	return Result{
		Auth:   git.AuthOptions{GitHubToken: tok.Token},
		Source: SourceGitHubApp,
	}, nil
}
