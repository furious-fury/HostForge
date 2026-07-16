# Service types roadmap

HostForge currently provisions repository-backed application services. The Add Service flow presents the longer-term service categories so the product can grow without mixing unrelated setup fields into one form.

## Application services

Available now. This category covers frontend applications, backend APIs, full-stack applications, and repository-backed workers. Railpack remains responsible for framework detection, build planning, runtime commands, and container creation.

## Database services

Planned, but not provisioned by the current shell. A future database wizard should cover:

- engine and version selection for PostgreSQL, MySQL, Redis, and other supported engines;
- persistent volume creation, backup policy, restore workflows, and storage limits;
- generated credentials, connection strings, environment injection, and secret rotation;
- private networking by default, with explicit controls for public access;
- health, metrics, logs, upgrades, and safe deletion requirements.

Database services must not be represented as ordinary Railpack builds because their lifecycle and durability requirements are different from application containers.

## Cron jobs

Planned, but not scheduled by the current shell. A future cron wizard should cover:

- repository, branch, root directory, runtime, and command selection;
- validated cron expressions, timezone selection, concurrency policy, and timeouts;
- environment variables and secret access;
- execution history, logs, retry behavior, manual runs, and cancellation;
- retention and alerting for failed or missed executions.

Cron jobs may reuse Railpack for building repository code, but scheduling and execution history require a separate runtime workflow.

## Current UI contract

Database and cron cards are intentionally marked `Planned`. They do not create records, start containers, or imply that provisioning is available. When implementation begins, each card should open its own setup wizard and persist an explicit service type rather than overloading the application-service model.
