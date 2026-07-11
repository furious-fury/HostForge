package redact

import "testing"

func TestRepoURLForLog_stripsUserinfo(t *testing.T) {
	got := RepoURLForLog("https://user:secret@github.com/org/repo.git")
	want := "https://github.com/org/repo.git"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestHTTPURLForLog_stripsUserinfo(t *testing.T) {
	got := HTTPURLForLog("http://admin:tok@127.0.0.1:2019/foo")
	want := "http://127.0.0.1:2019/foo"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// Installation access tokens are injected into the URL as the password of an
// x-access-token@ userinfo pair. Make sure neither the user nor the token
// survives RepoURLForLog, even in pathological cases.
func TestRepoURLForLog_stripsInstallationToken(t *testing.T) {
	cases := []string{
		"https://x-access-token:ghs_installtokenvalue@github.com/org/repo.git",
		"https://x-access-token:v1.ghs_1234567890abcdef@github.com/org/repo.git",
	}
	want := "https://github.com/org/repo.git"
	for _, in := range cases {
		if got := RepoURLForLog(in); got != want {
			t.Errorf("RepoURLForLog(%q)=%q want %q", in, got, want)
		}
	}
}
