package git

import (
	"testing"

	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

func TestAuthMethodForRepo_GitHubOnly(t *testing.T) {
	t.Parallel()
	m := authMethodForRepo("https://github.com/org/repo", AuthOptions{GitHubInstallationToken: "ghs_install"})
	basic, ok := m.(*githttp.BasicAuth)
	if !ok {
		t.Fatalf("expected BasicAuth for github repo, got %T", m)
	}
	if basic.Username != "x-access-token" || basic.Password != "ghs_install" {
		t.Fatalf("unexpected basic auth values: %+v", basic)
	}
}

func TestAuthMethodForRepo_NoTokenOrNonGithub(t *testing.T) {
	t.Parallel()
	if m := authMethodForRepo("https://github.com/org/repo", AuthOptions{}); m != nil {
		t.Fatalf("expected nil auth with empty token, got %T", m)
	}
	if m := authMethodForRepo("https://gitlab.com/org/repo", AuthOptions{GitHubInstallationToken: "ghs_install"}); m != nil {
		t.Fatalf("expected nil auth for non-github host, got %T", m)
	}
	if m := authMethodForRepo("not-a-url", AuthOptions{GitHubInstallationToken: "ghs_install"}); m != nil {
		t.Fatalf("expected nil auth for invalid url, got %T", m)
	}
}
