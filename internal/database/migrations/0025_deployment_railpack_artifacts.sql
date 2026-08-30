-- Raw Railpack build plan and info, captured per deployment (ADR-0002 §15.6,
-- §15.7). Previously regenerated and discarded on every build — only
-- detectedProviders survived, folded into stack_kind/stack_label. Persisting
-- the plan itself is what makes hostforge.toml generation and post-mortem
-- debugging of a build possible; extracted fields (start command, resolved
-- language versions) are a deliberate follow-up, not this migration.
--
-- Empty for a Dockerfile build, or when the source file exceeded the
-- persistence size cap (internal/railpack.maxProvenanceJSONBytes) — never
-- truncated, since truncated JSON cannot be parsed by anything reading this
-- column later.
ALTER TABLE deployments ADD COLUMN railpack_plan_json TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN railpack_info_json TEXT NOT NULL DEFAULT '';
