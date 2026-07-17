# Service types roadmap

HostForge currently provisions repository-backed application services. The Add Service flow presents the longer-term service categories so the product can grow without mixing unrelated setup fields into one form.

## Application services

Available now. This category covers frontend applications, backend APIs, full-stack applications, and repository-backed workers. Railpack remains responsible for framework detection, build planning, runtime commands, and container creation.

## Database services

Implemented pending the mandatory six-engine VPS acceptance matrix. The database wizard covers digest-pinned PostgreSQL, MySQL, MariaDB, MongoDB, Redis, and Valkey releases as environment-isolated, privately networked services with persistent Docker volumes. It supports engine/version, environments, resource presets, editable application connection bindings, encrypted R2/S3 backup policies, restore-as-copy, guarded replace-current restore, retained deletion, and same-version patch-image upgrades. Database detail shows durable operation progress, independent environment state, logs, metrics, storage use, lifecycle controls, credential rotation, backup history, and rollback outcomes.

The remaining release gate covers:

- complete lifecycle acceptance runs for every engine on the target VPS;
- recording measured disk use, restore verification, restart reconciliation, isolation, and zero published database ports.

Secure, audited public database access remains a separate future phase and must not be implemented by publishing a raw Docker port.

Database services must not be represented as ordinary Railpack builds because their lifecycle and durability requirements are different from application containers.

The agreed architecture, delivery phases, supported engine catalog, private networking model, backup design, and acceptance requirements are defined in [database-services-implementation-plan.md](database-services-implementation-plan.md).

## Cron jobs

Planned, but not scheduled by the current shell. A future cron wizard should cover:

- repository, branch, root directory, runtime, and command selection;
- validated cron expressions, timezone selection, concurrency policy, and timeouts;
- environment variables and secret access;
- execution history, logs, retry behavior, manual runs, and cancellation;
- retention and alerting for failed or missed executions.

Cron jobs may reuse Railpack for building repository code, but scheduling and execution history require a separate runtime workflow.

## Current UI contract

The Database card opens its dedicated wizard and persists an explicit `database` service type. Engines without a completed, digest-pinned adapter remain visible but disabled. The Cron card remains marked `Planned` and does not create records or start containers.
