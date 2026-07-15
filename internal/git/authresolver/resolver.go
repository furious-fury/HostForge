// Package authresolver mints GitHub App installation credentials for repository access.
package authresolver

import (
	"context"
	"strings"

	"github.com/hostforge/hostforge/internal/git"
)

type AppTokenProvider interface {
	MintInstallationToken(ctx context.Context, installationID int64) (InstallationToken, error)
}

type InstallationToken struct {
	Token string
}

type Source string

const (
	SourceNone      Source = "none"
	SourceGitHubApp Source = "github_app"
)

type Result struct {
	Auth   git.AuthOptions
	Source Source
}

type Resolver struct {
	app AppTokenProvider
}

func New(app AppTokenProvider) *Resolver {
	return &Resolver{app: app}
}

func (r *Resolver) ResolveForRepoAccess(ctx context.Context, installationID int64) (Result, error) {
	if r == nil || r.app == nil || installationID <= 0 {
		return Result{Source: SourceNone}, nil
	}
	token, err := r.app.MintInstallationToken(ctx, installationID)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(token.Token) == "" {
		return Result{Source: SourceNone}, nil
	}
	return Result{Auth: git.AuthOptions{GitHubInstallationToken: token.Token}, Source: SourceGitHubApp}, nil
}

func (r *Resolver) ResolveInstallationAuth(ctx context.Context, installationID int64) (git.AuthOptions, error) {
	result, err := r.ResolveForRepoAccess(ctx, installationID)
	if err != nil {
		return git.AuthOptions{}, err
	}
	return result.Auth, nil
}
