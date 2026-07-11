package git

import (
	"net"
	"net/url"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"
)

// AuthOptions contains optional git transport credentials. At most one of the
// concrete credentials (GitHubToken, SSHPrivateKeyPEM) is used for a given
// remote; selection is based on the repository URL scheme/host.
type AuthOptions struct {
	// GitHubToken applies only to github.com remotes over HTTPS.
	// May be either a PAT or an App installation access token; both use the
	// "x-access-token" basic-auth username.
	GitHubToken string
	// SSHPrivateKeyPEM is a PEM-encoded ed25519/RSA private key in OpenSSH or PKCS#1
	// form. Used for ssh://, git@ or git+ssh:// remotes.
	SSHPrivateKeyPEM []byte
	// SSHUser overrides the default ssh user ("git").
	SSHUser string
	// SSHKeyPassphrase is the passphrase for SSHPrivateKeyPEM, if any.
	SSHKeyPassphrase string
	// SSHInsecureIgnoreHostKey skips host key verification. Defaults to true for
	// MVP when no known_hosts policy is wired.
	SSHInsecureIgnoreHostKey bool
}

// IsSSHURL reports whether repoURL is an ssh:// or scp-style (user@host:path) remote.
func IsSSHURL(repoURL string) bool {
	raw := strings.TrimSpace(repoURL)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(raw), "ssh://") || strings.HasPrefix(strings.ToLower(raw), "git+ssh://") {
		return true
	}
	// scp-style: user@host:path (no scheme, and has '@' before first ':')
	if strings.Contains(raw, "://") {
		return false
	}
	at := strings.Index(raw, "@")
	colon := strings.Index(raw, ":")
	return at > 0 && colon > at
}

func authMethodForRepo(repoURL string, auth AuthOptions) transport.AuthMethod {
	raw := strings.TrimSpace(repoURL)
	if raw == "" {
		return nil
	}
	if IsSSHURL(raw) {
		if len(auth.SSHPrivateKeyPEM) == 0 {
			return nil
		}
		user := strings.TrimSpace(auth.SSHUser)
		if user == "" {
			user = "git"
		}
		keys, err := gitssh.NewPublicKeys(user, auth.SSHPrivateKeyPEM, auth.SSHKeyPassphrase)
		if err != nil {
			return nil
		}
		if auth.SSHInsecureIgnoreHostKey {
			keys.HostKeyCallback = ssh.InsecureIgnoreHostKey()
		}
		return keys
	}
	token := strings.TrimSpace(auth.GitHubToken)
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
	// GitHub accepts Basic auth with username "x-access-token" and a PAT or
	// installation access token as the password.
	return &githttp.BasicAuth{
		Username: "x-access-token",
		Password: token,
	}
}
