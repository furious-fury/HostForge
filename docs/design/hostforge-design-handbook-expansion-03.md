# Hostforge Design Handbook — Expansion 03

This merge-ready addition deepens the operational workflow chapters. Append it after the existing handbook and component expansion.

<!-- BEGIN: workflow-first-deployment.md -->
# Workflow: First Deployment

## Goal

The first deployment proves that Hostforge can make self-managed compute understandable without pretending it is effortless. A user should reach a healthy service with a clear understanding of the source, configuration, deployment state, and URL they now own.

## Flow structure

The primary path is: choose source → name service → review runtime defaults → configure required settings → review plan → deploy → observe → arrive at the service overview.

Present one decision group at a time, but retain a visible summary of completed choices. Do not make users complete an opaque wizard. A user must be able to go back, edit a choice, and understand the impact without losing entered configuration.

### 1. Choose source

Explain what Hostforge can deploy: a connected repository, container image, or supported template. Identify the selected repository, branch, and commit where available. If authorization is missing, say what Hostforge needs access to and provide a recoverable connection flow.

Do not show advanced build controls before the user has a valid source. Make them available immediately after source selection in a labelled **Advanced build settings** section.

### 2. Name and scope the service

Show organization, project, and environment scope before the name field. Use a readable service name and show how it affects the generated internal identifier or URL when relevant. Warn before creating a service in a production environment, but do not use alarmist styling.

### 3. Configure runtime

Start with detected or safe defaults: build method, exposed port, health-check path when known, region or machine selection when applicable, and public/private exposure. Explain the origin of inferred values: “Detected from Dockerfile” is more trustworthy than simply pre-filling a field.

Unknown values remain explicit. Never present a guessed port or command as confirmed. If a default is unsafe or cannot be verified, require an informed choice.

### 4. Review plan

Before deployment, show a compact factual plan:

- source revision and build method;
- target project and environment;
- runtime settings and exposed endpoint;
- persistent storage or domains to be created or attached;
- actions that start now, including build and deployment; and
- settings that will require a later redeploy to change.

The primary action reads **Deploy service**, never merely **Create**. The page must make it clear that this action starts real infrastructure work.

### 5. Observe deployment

After submission, take the user to a persistent deployment detail page. The page shows source, stage timeline, elapsed time, live logs, health checks, current status, and one relevant recovery path. Do not trap deployment observation in a modal or transient toast.

Use named stages: Queued, Building, Pushing image, Creating release, Deploying, Running health checks, and Healthy. Show only stages the platform can honestly observe. If a stage waits on external work, state the reason and current elapsed time.

### 6. Complete

On success, show the service URL, Healthy status, release identifier, and last deployment time. Offer a restrained next-action set: **View service**, **View logs**, **Add domain**, and **Configure variables**. Avoid celebratory confetti or oversized success graphics. Reliable operation is the reward.

## Failure and recovery

If source connection fails, preserve the selected repository and provide reconnect instructions. If build fails, show the failing stage, relevant log excerpt, full logs, and a retry action. If health checks fail, identify the checked path, observed result, configured port, and options to inspect logs, edit configuration, or roll back when a previous healthy release exists.

Never automatically retry a user-initiated failed deployment without saying so. If Hostforge retries due to a documented platform condition, show the retry count and reason.

## Accessibility and implementation notes

Stage changes are announced concisely to assistive technology. Logs do not steal focus when new content arrives. The primary action is keyboard reachable, review content follows logical reading order, and mobile layouts preserve all plan details without requiring horizontal scrolling.

<!-- END: workflow-first-deployment.md -->

<!-- BEGIN: workflow-service-lifecycle.md -->
# Workflow: Service, Build, Release, and Rollback

## Service overview

The service overview answers four questions in its first viewport: Is this service healthy? What version is running? What happened most recently? What can I do next?

Show service name, project and environment context, current lifecycle state, public endpoint when applicable, running release, and the latest deployment outcome. Follow with a small operational summary—health, request or resource signal where available, and recent events—then direct links to logs, deployments, configuration, metrics, and settings.

Do not turn the overview into a dashboard of decorative cards. Each summary must lead to evidence or an action.

## Deployment history

Deployment history is an ordered operational record, not a changelog. Each row shows release or deployment identifier, source revision, trigger, start time, duration, final state, and a direct route to detail. Mark the currently running release plainly.

Do not erase failed attempts. They are evidence. If history is filtered, make the filter visible and preserve it when returning from a detail page.

## Deployment detail

A deployment detail page contains:

- immutable identification: service, environment, release, source revision, actor or trigger;
- lifecycle timeline with timestamps and duration;
- build and runtime output;
- configuration snapshot or a link to the relevant version;
- health-check result;
- resulting status; and
- recovery actions that are actually available.

Status changes should be represented in chronological order. Do not collapse a failed health check into a generic final failure. Users must be able to identify whether failure occurred during build, artifact publish, scheduling, startup, routing, or health verification.

## Restart, redeploy, and rollback

**Restart** restarts the currently configured running service. **Redeploy** creates or reapplies a release from defined source and configuration. **Roll back** returns traffic or runtime to a previous release. These terms must never be used interchangeably.

Before rollback, present the target release, the current release, the target’s age, source revision, configuration compatibility notice, and the consequence: configuration, data migrations, external services, and attached storage may not revert. The confirmation reads **Roll back to release [identifier]**.

After rollback begins, surface it as an operation with a timeline. A successful rollback ends in Healthy only after health checks pass. If it fails, the page must explain whether the previous release remains active and what state traffic is in.

## Recovery hierarchy

When a release fails, prioritize actions by safety: inspect details and logs; retry if inputs are still valid; correct configuration; roll back to a known compatible healthy release. Do not make destructive deletion prominent as a recovery path.

<!-- END: workflow-service-lifecycle.md -->

<!-- BEGIN: workflow-configuration-and-secrets.md -->
# Workflow: Environment, Secrets, and Configuration

## Configuration model

Users must always know the scope of a configuration value: organization, project, environment, or service. Display scope in the page context and beside any inherited value. An inherited value is not the same as a local value; users must be able to tell which applies at runtime and where it can be changed.

Group configuration by runtime relevance: environment variables, secrets, build settings, runtime settings, health checks, networking, storage, and deployment behavior. Do not put all settings in one long form.

## Environment variables

The variables list shows key, type, source or scope, last changed time, and whether a change is pending deployment. Values are not normally displayed in a resource table. A variable’s key should be easy to copy and use a mono type treatment.

Adding a variable requires a key, value, scope, and an explicit type when the product distinguishes ordinary values from secrets. Validate the key using clear format guidance. Prevent duplicate keys within the same scope and explain which inherited key is being overridden when applicable.

## Secrets

Secrets require the same configuration clarity as variables with stricter visibility. After storage, show the key, scope, last changed time, and actor where available—not the secret value. A user may copy a supplied value before saving, but Hostforge must not imply the stored value can be retrieved later.

Reveal, rotate, and delete are deliberate actions. Rotation must explain whether the old value remains in use until redeploy or restart. Deletion must identify dependent services and require a confirmation proportionate to the resulting runtime risk.

## Change and apply

Saving configuration must report its effect honestly:

- **Applied immediately** for settings that Hostforge can change without restarting.
- **Pending restart** when the running process needs a restart.
- **Pending deployment** when a new release is required.
- **Not applied** when validation or platform constraints prevent storage.

Do not show “Saved” as the only result when a service is still running old configuration. Provide an appropriate next action, such as **Deploy latest configuration**.

## Audit and recovery

For consequential settings, retain an event record showing what changed, where, when, and by whom without exposing secret values. If the product supports versioning, clearly identify the version used by the currently running release and provide a reversible path for non-secret configuration.

<!-- END: workflow-configuration-and-secrets.md -->

<!-- BEGIN: workflow-domains-networking-storage.md -->
# Workflow: Domains, Networking, Certificates, and Storage

## Domains

Adding a domain is a verification workflow, not a simple text field. First validate the hostname and its target service/environment. Then display the exact DNS record Hostforge expects, including record type, name, value, and any propagation caveat. Provide copy controls for values and show the last verification attempt.

Domain states are explicit: Needs configuration, Verifying, Verified, Certificate provisioning, Active, Expiring soon, and Failed. “Active” means the domain is routed and its certificate is valid; it must not mean merely that the DNS record was submitted.

## Certificates

Show certificate issuer when relevant, issue date, expiry date, renewal state, and failure reason. Renewal is an operational state, not background magic: if it retries or cannot verify a domain, show why and what the user must change. Escalate expiry risk with factual warning language and a direct recovery path.

## Networking

Networking settings must name the direction and scope of exposure: public endpoint, private service, inbound port, outbound access, or internal route. Do not present a bare port number without explaining what listens on it and who can reach it. Any setting that increases public exposure must include a concise consequence statement before confirmation.

## Storage

Creating or attaching storage requires service scope, mount path, capacity or policy, persistence behavior, and availability implication. If storage cannot move with a deployment or region change, say so before the action. Do not imply that a rollback restores data.

Destructive volume actions require a typed or otherwise high-intent confirmation when data loss is irreversible. The confirmation names the volume, affected service, and permanence of deletion. A stopped service is not sufficient reason to visually downplay a destructive storage action.

<!-- END: workflow-domains-networking-storage.md -->

<!-- BEGIN: workflow-monitoring-logs-incidents.md -->
# Workflow: Monitoring, Logs, Incidents, and Recovery

## Monitoring overview

Monitoring is for answering operational questions, not displaying decoration. The service page should make it easy to determine health, resource pressure, recent deployment impact, and whether a condition needs investigation. Every metric shows unit, time range, aggregation, and freshness.

Provide sensible default windows for recent investigation, with visible control over time range. Do not interpolate missing telemetry as a smooth chart. Mark a gap, stale source, or unavailable metric explicitly.

## Logs

Logs are a primary investigation surface. Users can follow live output, pause follow, search, filter by source or severity when supported, copy selected content, and see whether output is truncated or disconnected. New lines must not move a user who is reading older output; show a “new logs” control instead.

Log entries preserve timestamp, source, level, and raw message. Wrapping is configurable. Errors that reference a deployment, release, request, or machine should link to the relevant evidence without obscuring the original text.

## Events

Events are durable facts: deployment started, configuration changed, certificate renewed, volume attached, machine restarted, and health check failed. Each event includes time, object, state, source, actor when known, and a detail path. Events are not a substitute for logs; they orient the user before detailed investigation.

## Incident presentation

When Hostforge detects or receives a failure, lead with affected scope, observed state, impact, first known time, and recovery status. Then provide evidence, causal information only when known, and the safest next actions. The interface must distinguish confirmed facts from inference.

Use a steady structure during incidents. Do not add panic-inducing animation, dramatic color wash, or ambiguous alarm copy. A red failure state is enough when paired with factual recovery guidance.

## Recovery completion

After recovery, show what changed, when service returned to a healthy state, and any unresolved follow-up. Retain the incident timeline so users can learn from it. Never silently remove evidence because the current state is healthy again.

<!-- END: workflow-monitoring-logs-incidents.md -->
