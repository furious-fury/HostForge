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
