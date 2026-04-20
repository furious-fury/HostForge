package git

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"testing"

	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"
)

func TestAuthMethodForRepo_GitHubOnly(t *testing.T) {
	t.Parallel()
	m := authMethodForRepo("https://github.com/org/repo", AuthOptions{GitHubToken: "ghp_123"})
	basic, ok := m.(*githttp.BasicAuth)
	if !ok {
		t.Fatalf("expected BasicAuth for github repo, got %T", m)
	}
	if basic.Username != "x-access-token" || basic.Password != "ghp_123" {
		t.Fatalf("unexpected basic auth values: %+v", basic)
	}
}

func TestAuthMethodForRepo_NoTokenOrNonGithub(t *testing.T) {
	t.Parallel()
	if m := authMethodForRepo("https://github.com/org/repo", AuthOptions{}); m != nil {
		t.Fatalf("expected nil auth with empty token, got %T", m)
	}
	if m := authMethodForRepo("https://gitlab.com/org/repo", AuthOptions{GitHubToken: "ghp_123"}); m != nil {
		t.Fatalf("expected nil auth for non-github host, got %T", m)
	}
	if m := authMethodForRepo("not-a-url", AuthOptions{GitHubToken: "ghp_123"}); m != nil {
		t.Fatalf("expected nil auth for invalid url, got %T", m)
	}
}

func TestIsSSHURL(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"ssh://git@github.com/a/b.git":      true,
		"git+ssh://git@example.com/x.git":   true,
		"git@github.com:a/b.git":            true,
		"https://github.com/a/b":            false,
		"http://example.com/x":              false,
		"":                                  false,
		"github.com/a/b":                    false,
		"mailto:user@example.com":           false,
	}
	for in, want := range cases {
		if got := IsSSHURL(in); got != want {
			t.Errorf("IsSSHURL(%q)=%v want %v", in, got, want)
		}
	}
}

func TestAuthMethodForRepo_SSH(t *testing.T) {
	t.Parallel()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "hostforge-test")
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(block)

	m := authMethodForRepo("git@github.com:org/repo.git", AuthOptions{
		SSHPrivateKeyPEM:         keyPEM,
		SSHInsecureIgnoreHostKey: true,
	})
	if _, ok := m.(*gitssh.PublicKeys); !ok {
		t.Fatalf("expected *gitssh.PublicKeys for ssh remote, got %T", m)
	}

	if m := authMethodForRepo("git@github.com:org/repo.git", AuthOptions{}); m != nil {
		t.Fatalf("ssh remote without key should yield nil, got %T", m)
	}
}
