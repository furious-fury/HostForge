package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/furious-fury/HostForge/internal/docker"
	"github.com/furious-fury/HostForge/internal/repository"
	mobyclient "github.com/moby/moby/client"
)

// deployImageRepoPrefix is the repository namespace every deploy image is
// tagged under (internal/services/deploy_v2.go builds "hostforge/<slug>:<id>").
// The sweep only ever considers tags under this prefix, so a database, gateway,
// or base image is never a candidate.
const deployImageRepoPrefix = "hostforge/"

// StartImageGarbageCollectionLoop runs image GC once shortly after startup and
// then on interval, in the reconciler style of
// StartDatabaseInstanceReconciliationLoop. A non-positive interval disables it.
//
// It is deliberately standalone rather than hooked into the deploy path: Phase 3
// is removing coupling from that path, and reclaiming disk is not something a
// deploy should have to wait on or be blamed for.
func StartImageGarbageCollectionLoop(ctx context.Context, log *slog.Logger, store *repository.Store, dockerClient *mobyclient.Client, retain int, interval time.Duration) {
	if interval <= 0 {
		log.Info("image garbage collection disabled", "interval", interval)
		return
	}
	go func() {
		// A short initial delay lets startup recovery and the orphan sweep
		// settle container state first, so the keep-set reflects reality rather
		// than a half-recovered moment.
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			if removed, err := SweepUnreferencedImages(ctx, log, store, dockerClient, retain); err != nil {
				log.Warn("image garbage collection failed", "error", err)
			} else if removed > 0 {
				log.Info("image garbage collection removed images", "removed", removed)
			}
			timer.Reset(interval)
		}
	}()
}

// SweepUnreferencedImages removes deploy image tags no deployment still needs.
// It lists every hostforge/* image, keeps those in the store's retained set,
// and removes the rest without forcing -- an image a container still uses comes
// back as a skip, not an error, so the daemon's reference counting is the final
// guard behind the database keep-set.
//
// Best-effort throughout: a failure on one tag is logged and the sweep
// continues; a failure to build the keep-set or list images is returned but is
// never fatal to the process, because the caller runs it in a background loop.
func SweepUnreferencedImages(ctx context.Context, log *slog.Logger, store *repository.Store, dockerClient *mobyclient.Client, retain int) (removed int, err error) {
	keep, err := store.ListRetainedImageRefs(ctx, retain)
	if err != nil {
		return 0, err
	}
	images, err := docker.ListImagesByRepoPrefix(ctx, dockerClient, deployImageRepoPrefix)
	if err != nil {
		return 0, err
	}
	for _, image := range images {
		for _, tag := range image.RepoTags {
			if _, kept := keep[tag]; kept {
				continue
			}
			gone, removeErr := docker.RemoveImageIfUnused(ctx, dockerClient, tag)
			if removeErr != nil {
				log.Warn("failed to remove unreferenced image", "image", tag, "error", removeErr)
				continue
			}
			if gone {
				log.Info("garbage collected deploy image", "image", tag)
				removed++
			}
		}
	}
	return removed, nil
}
