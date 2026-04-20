package git

import (
	"context"
	"fmt"
	"sort"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
)

// ListRemoteBranches returns sorted branch names available on repoURL and a
// best-effort inferred default branch.
func ListRemoteBranches(ctx context.Context, repoURL string, auth AuthOptions) ([]string, string, error) {
	remote := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{strings.TrimSpace(repoURL)},
	})
	refs, err := remote.ListContext(ctx, &gogit.ListOptions{Auth: authMethodForRepo(repoURL, auth)})
	if err != nil {
		return nil, "", fmt.Errorf("list remote refs: %w", err)
	}

	branchSet := map[string]struct{}{}
	byHash := map[string][]string{}
	var headHash plumbing.Hash
	defaultBranch := ""

	for _, ref := range refs {
		name := ref.Name()
		switch {
		case name.IsBranch():
			branch := name.Short()
			branchSet[branch] = struct{}{}
			hash := ref.Hash().String()
			byHash[hash] = append(byHash[hash], branch)
		case name == plumbing.HEAD && !ref.Hash().IsZero():
			headHash = ref.Hash()
			if ref.Type() == plumbing.SymbolicReference && ref.Target().IsBranch() {
				defaultBranch = ref.Target().Short()
			}
		}
	}

	branches := make([]string, 0, len(branchSet))
	for branch := range branchSet {
		branches = append(branches, branch)
	}
	sort.Strings(branches)
	if len(branches) == 0 {
		return nil, "", nil
	}

	if defaultBranch == "" && !headHash.IsZero() {
		defaultBranch = choosePreferredBranch(byHash[headHash.String()])
	}
	if defaultBranch == "" {
		defaultBranch = choosePreferredBranch(branches)
	}
	return branches, defaultBranch, nil
}

// ResolveBranch returns requested when set; otherwise tries to infer a remote
// default branch and falls back to "main".
func ResolveBranch(ctx context.Context, repoURL, requested string, auth AuthOptions) string {
	branch := strings.TrimSpace(requested)
	if branch != "" {
		return branch
	}
	_, inferred, err := ListRemoteBranches(ctx, repoURL, auth)
	if err == nil && strings.TrimSpace(inferred) != "" {
		return inferred
	}
	return "main"
}

func choosePreferredBranch(branches []string) string {
	if len(branches) == 0 {
		return ""
	}
	originalByLower := map[string]string{}
	for _, branch := range branches {
		lower := strings.ToLower(strings.TrimSpace(branch))
		if _, exists := originalByLower[lower]; !exists {
			originalByLower[lower] = strings.TrimSpace(branch)
		}
	}
	for _, preferred := range []string{"main", "master", "trunk", "develop", "dev"} {
		if branch, ok := originalByLower[preferred]; ok {
			return branch
		}
	}
	out := make([]string, 0, len(branches))
	out = append(out, branches...)
	sort.Strings(out)
	return out[0]
}
