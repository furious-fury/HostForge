package git

import (
	"net"
	"net/url"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// AuthOptions contains a short-lived GitHub App installation credential.
type AuthOptions struct {
	GitHubInstallationToken string
}

func authMethodForRepo(repoURL string, auth AuthOptions) transport.AuthMethod {
	raw := strings.TrimSpace(repoURL)
	if raw == "" {
		return nil
	}
	token := strings.TrimSpace(auth.GitHubInstallationToken)
	if token == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		host = strings.ToLower(strings.TrimSpace(u.Host))
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
	}
	if host != "github.com" && host != "www.github.com" {
		return nil
	}
	// GitHub App installation tokens use the x-access-token basic-auth username.
	return &githttp.BasicAuth{
		Username: "x-access-token",
		Password: token,
	}
}
