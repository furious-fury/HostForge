# Hostforge Revamp Implementation Plan

## Goal

Complete the remaining frontend revamp in focused, independently verifiable increments without changing frameworks, routing, APIs, data contracts, or working behavior.

## 1. Audit route state signals

- Map existing data and query signals for every priority route to the handbook lifecycle, freshness, failure, empty, and recovery rules.
- Confirm which operational states can be represented from current frontend data.
- Record any state that requires a new backend signal rather than guessing in the UI.

## 2. Add shared operational feedback primitives

- Create reusable skeleton loading, stale-data notice, inline error/retry, and degraded/recovery patterns.
- Keep semantic tokens, ARIA announcements, keyboard behavior, and reduced-motion handling centralized.
- Ensure each reusable component defines default, loading, error, empty, narrow-layout, and focus-visible states.

## 3. Revamp project overview and service overview

- Improve first-viewport hierarchy, scope, status summaries, and next actions.
- Upgrade project and deployment tables for long content, density, empty results, loading, stale data, and retrieval failures.
- Add factual recovery actions for degraded and failed services.

## 4. Revamp create-service and deployment detail/logs

- Strengthen form validation, error association, and deployment-progress messaging.
- Represent queued, build, deploy, health-check, success, failure, and rollback states with the canonical vocabulary.
- Improve log-stream connection, reconnecting, paused, empty, and failure states with direct recovery paths.

## 5. Revamp environment variables and secrets

- Distinguish saved configuration from runtime-applied configuration and redeploy requirements.
- Improve validation, encryption-key availability, secret-safe rendering, empty states, and server failures.
- Keep destructive secret actions explicit and keyboard accessible.

## 6. Test and validate

- Add focused component and state tests for lifecycle labels, accessible names, focus behavior, and high-risk transitions.
- Verify each priority route at wide and compact layouts, keyboard-only operation, 200% zoom, and reduced-motion preference.
- Run the production build and review state coverage with realistic long values, dense tables, stale data, and failures.

## Completion criteria

The remaining revamp is complete only when every priority route has a deliberate initial load, loaded, empty, validation failure, server failure, stale/degraded, and narrow-layout experience, while preserving all existing integrations and data contracts.