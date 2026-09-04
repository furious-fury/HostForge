-- Domain publication state (ADR-0002 §6, §19), separate from ssl_status.
--
-- ssl_status and "is this route live in Caddy" have been the same field
-- since domains existed: SyncCaddyRoutes set ssl_status=ACTIVE on a
-- successful reload and ssl_status=ERROR both when a route had no upstream
-- yet and when Caddy actually failed to apply it -- three different
-- situations collapsed into one status with one meaning assigned to it by
-- whichever writer ran last.
--
-- publish_state is the new source of truth for whether Caddy is currently
-- serving this domain: published, unpublished (no upstream yet, or the
-- reconciler has not caught up), or invalid (this domain's own config could
-- not be validated and it is quarantined out of the fleet render, ADR
-- section 19). ssl_status keeps its existing values and gains a real
-- owner in this phase's cert-poll change: it comes to mean "does a
-- certificate exist", independent of routing.
ALTER TABLE domains ADD COLUMN publish_state TEXT NOT NULL DEFAULT 'unpublished'
    CHECK(publish_state IN ('published','unpublished','invalid'));
ALTER TABLE domains ADD COLUMN publish_error TEXT NOT NULL DEFAULT '';
