package services

import "testing"

func TestCanonicalRepoURL_stripsCredentials(t *testing.T) {
	out, err := CanonicalRepoURL("https://x:y@github.com/acme/app.git")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://github.com/acme/app"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}
