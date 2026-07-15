// Package git wraps go-git for cloning and updating deploy worktrees.
package git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// WorktreeDir returns a deterministic directory name segment for repoURL and branch.
// branch may be empty (remote default).
func WorktreeDir(repoURL, branch string) string {
	h := sha256.Sum256([]byte(repoURL + "\n" + branch))
	return hex.EncodeToString(h[:16])
}

// CloneOrUpdate clones repoURL into destDir, or opens and pulls if dest is already a repo.
func CloneOrUpdate(ctx context.Context, repoURL, branch, destDir string, auth AuthOptions) error {
	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil {
		return fmt.Errorf("mkdir parent: %w", err)
	}

	gitDir := filepath.Join(destDir, ".git")
	if st, err := os.Stat(gitDir); err == nil && st.IsDir() {
		return pull(ctx, repoURL, destDir, branch, auth)
	}
	if _, err := os.Stat(destDir); err == nil {
		if err := os.RemoveAll(destDir); err != nil {
			return fmt.Errorf("remove incomplete path %s: %w", destDir, err)
		}
	}

	opts := &gogit.CloneOptions{
		URL:      repoURL,
		Progress: os.Stderr,
		Auth:     authMethodForRepo(repoURL, auth),
	}
	if branch != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(branch)
		opts.SingleBranch = true
	}

	_, err := gogit.PlainCloneContext(ctx, destDir, false, opts)
	if err != nil {
		return fmt.Errorf("git clone: %w", err)
	}
	return nil
}

func pull(ctx context.Context, repoURL, repoPath, branch string, auth AuthOptions) error {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("git open: %w", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}
	pullOpts := &gogit.PullOptions{
		RemoteName: "origin",
		Auth:       authMethodForRepo(repoURL, auth),
	}
	if branch != "" {
		pullOpts.ReferenceName = plumbing.NewBranchReferenceName(branch)
		pullOpts.SingleBranch = true
	}
	if err := wt.PullContext(ctx, pullOpts); err != nil && err != gogit.NoErrAlreadyUpToDate {
		return fmt.Errorf("git pull: %w", err)
	}
	return nil
}

// HeadCommit returns the checked-out commit hash for a cloned worktree.
func HeadCommit(repoPath string) (string, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("git open: %w", err)
	}
	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("git head: %w", err)
	}
	return head.Hash().String(), nil
}

func CheckoutCommit(repoPath, commitHash string) error {
	hash := plumbing.NewHash(commitHash)
	if hash.IsZero() {
		return fmt.Errorf("invalid empty commit hash")
	}
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("git open: %w", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}
	if err := worktree.Checkout(&gogit.CheckoutOptions{Hash: hash, Force: true}); err != nil {
		return fmt.Errorf("checkout commit %s: %w", commitHash, err)
	}
	return nil
}
