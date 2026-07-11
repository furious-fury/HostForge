# Hostforge Design Handbook

**Version:** 1.0 baseline  
**Status:** Canonical working handbook  
**Product:** Hostforge — open-source, self-hosted PaaS

Hostforge makes self-managed compute feel as polished as managed cloud platforms while preserving ownership, transparency, and control. This handbook is the source of truth for user-facing product decisions. It is deliberately operational: it explains what a user sees, why it exists, and how contributors implement it consistently.

<!-- BEGIN: handbook-charter.md -->
# 01. Handbook Charter

Use this handbook when designing, implementing, reviewing, or documenting Hostforge. It applies to the web application, empty states, settings, deployment output, in-product help, and product-owned documentation UI.

The handbook is not a gallery of screenshots. A screenshot can become stale; a principle, token, state definition, and component contract can be tested and reused. Follow the canonical chapter for every shared rule. A local screen must not invent its own status meaning, action hierarchy, terminology, or component variant.

Hostforge is for indie hackers, solo developers, self-hosting enthusiasts, small engineering teams, and DevOps-minded full-stack developers. It is not enterprise theatre. The product should be precise enough for experienced operators and approachable enough for someone deploying a first container.

<!-- END: handbook-charter.md -->

<!-- BEGIN: product-and-brand-position.md -->
# 02. Product and Brand Position

Hostforge is “the most polished self-hosted PaaS for people who want complete control over their infrastructure.” Its promise is not that infrastructure is effortless; its promise is that infrastructure is understandable, capable, and owned by the user.

Every screen must reinforce at least one pillar: **Confidence** through truthful state and dependable patterns; **Control** through visible settings and reversible actions; **Speed** through direct paths and responsive feedback; **Transparency** through logs, reasons, and explicit system behavior.

The visual character combines GitHub maturity, Fly.io engineering culture, Cloudflare operational confidence, and Linear polish. It is never cyberpunk, terminal cosplay, glassmorphism, oversized-gradient marketing, or black-and-white minimalism that hides hierarchy.

<!-- END: product-and-brand-position.md -->

<!-- BEGIN: design-philosophy.md -->
# 03. Design Philosophy

> Every interface is an operational interface.

Hostforge exists to help people manage infrastructure they own. Every visible element should answer one of three questions: what is happening, why is it happening, or what can I do next. If it answers none, remove it.

Prefer confidence over excitement. Infrastructure earns trust through restraint, accurate states, stable layout, and predictable recovery—not spectacle. The ideal response is: “I stopped thinking about the UI.”

## Progressive Power

Beginners need an obvious first task, observable progress, and a clear definition of success. Intermediate users need nearby controls for branches, replicas, health checks, and storage. Experts need fast access to logs, history, networking, configuration, and resource use. Progressive disclosure is information architecture, not feature removal.

## Nothing Magical

Never perform an unexplained action. Show why a deployment started, when a retry occurred, what failed, and the next recovery step. Use specific operational labels such as Building, Waiting for image, Running health checks, Deploying, Healthy, and Degraded. Avoid vague labels such as Working or Processing.

## Calm under pressure

Failure messaging must remain factual. Preserve layout, controls, and context; do not use alarming decoration. The interface should communicate: “This failed, and here is exactly how you recover.”

<!-- END: design-philosophy.md -->

<!-- BEGIN: design-governance.md -->
# 04. Design Governance

Conflicts resolve in this order: security, safety, legal, and accessibility requirements; approved product specification and decision records; Design Philosophy; foundations and tokens; component contracts; workflows; examples.

Every shared definition has one home. Color semantics belong to Color and Semantic Tokens; lifecycle labels belong to Status, Lifecycle, and Time; overlay behavior belongs to Overlays and Focused Tasks. Other chapters link to the owner rather than duplicate rules.

Use **must** for a requirement, **should** for the expected default with a documented exception, and **may** for an allowed option. A decision record is required for changes to shared tokens, lifecycle states, components, navigation, terminology, accessibility baselines, or repeatedly used workflows. Include the decision, rationale, owner, affected chapters, migration work, and deprecation note.

Do not create local semantics. If a shared pattern is insufficient, extend its canonical chapter. Before approval, ask: would this make Hostforge more predictable for someone operating infrastructure during a stressful moment?

<!-- END: design-governance.md -->

<!-- BEGIN: visual-language-foundations.md -->
# 05. Visual Language Foundations

Hostforge should look measured, warm, and capable. Use clear alignment, durable surfaces, restrained contrast, and dense-but-breathable information. The interface is not minimal for minimalism’s sake: empty space must separate decisions, not merely create an editorial pose.

Prefer a light primary workspace with Foundation-colored navigation and code surfaces. Cards are structural regions, not decorative containers; avoid cards inside cards unless the inner region is a distinct selectable or independently actionable object. Borders should do most of the separation work. Shadows are reserved for floating layers.

Use square-to-soft geometry: controls and surfaces should have a modest, consistent radius. Never use pill treatment for arbitrary containers. Rounded shapes communicate compact actions, tags, and statuses; they should not make operational UI feel toy-like.

<!-- END: visual-language-foundations.md -->

<!-- BEGIN: color-and-semantic-tokens.md -->
# 06. Color and Semantic Tokens

Color is a communication channel. **Forge `#A41623`** communicates Hostforge identity, precision, selected brand moments, and high-attention non-destructive emphasis. It must never communicate deletion, danger, or generic “primary” action by default. **Signal `#FFB320`** communicates deployment activity, build progress, active resource use, and selected operational emphasis. **Foundation `#262626`** communicates structure, navigation, primary text, and infrastructure surfaces. White is the primary light canvas.

Use semantic status tokens in addition to brand colors: success for completed healthy operation, caution for an attention-worthy but recoverable condition, danger for failure or destructive consequence, and neutral for inactive, unknown, or structural information. Do not encode meaning by color alone: pair every state with a text label and, where space permits, an icon.

Status colors must retain their meaning everywhere. Signal is activity, not warning. Forge is identity, not failure. Red is reserved for failure and destructive consequences. Meet WCAG AA contrast for text and essential controls; use a text label rather than relying on a low-contrast colored dot.

<!-- END: color-and-semantic-tokens.md -->

<!-- BEGIN: typography-and-writing-hierarchy.md -->
# 07. Typography and Writing Hierarchy

Use a highly legible sans-serif UI face with a reliable system fallback. Use a mono face only for logs, commands, identifiers, environment variables, and code-like values. Mono text is a semantic signal, not decoration.

Hierarchy comes from size, weight, spacing, and placement before color. Use one page title, short section titles, plain body text, and compact labels. Labels identify controls; they are not miniature headings. Avoid all-caps except short established acronyms. Keep line lengths readable and do not use tiny type to force dense configuration into one view.

Write with calm precision. Prefer “Deployment failed during health checks” to “Oops, something went wrong.” State the object, state, cause when known, and recovery action. Use sentence case. Use exact terms consistently: service, deployment, build, release, environment variable, secret, domain, volume, and machine are distinct concepts.

<!-- END: typography-and-writing-hierarchy.md -->

<!-- BEGIN: spacing-layout-responsive-grids.md -->
# 08. Spacing, Layout, and Responsive Grids

Use a consistent base spacing unit and a small scale of multiples. Repeated rhythm is more important than memorizing arbitrary pixel values. Maintain generous separation between page regions, moderate separation between related groups, and compact separation inside a control.

Desktop pages use a persistent global navigation rail, a bounded content column, and a page header containing context, title, state, and primary action. Wide data may use the available workspace; forms and prose must remain constrained for scanning. On narrow screens, preserve task order, collapse secondary navigation into an explicit control, and move secondary actions into a menu only when their labels remain discoverable.

Never make a critical operation horizontal-scroll-only. Tables may scroll horizontally when necessary, but must preserve the primary identifier and provide a usable compact alternative for common actions.

<!-- END: spacing-layout-responsive-grids.md -->

<!-- BEGIN: iconography-imagery-illustration.md -->
# 09. Iconography, Imagery, and Illustration

Use a single coherent line-icon family. Icons clarify familiar actions and resource types; they do not replace labels for consequential actions. Icon-only buttons must have accessible names and visible tooltips.

Use diagrams only when they make infrastructure relationships easier to understand: networking, build-to-release flow, or volume attachment are valid cases. Avoid generic stock imagery and decorative illustrations. Empty states may use a restrained, product-specific visual only when it supports orientation rather than entertainment.

<!-- END: iconography-imagery-illustration.md -->

<!-- BEGIN: motion-and-perceived-performance.md -->
# 10. Motion and Perceived Performance

Motion explains a change in state, location, or hierarchy. Use it for opening overlays, revealing configuration, sorting tables, expanding logs, and showing deployment progression. Do not use idle motion, bouncing cards, decorative page transitions, or movement that delays action.

Transitions should be brief, direct, and interruptible. Respect `prefers-reduced-motion`; reduce motion to opacity changes or immediate state updates while preserving status information. A long-running operation must show a truthful state, elapsed time where useful, and a path to logs or details. Never use a spinner as the only evidence that work is occurring.

<!-- END: motion-and-perceived-performance.md -->

<!-- BEGIN: information-architecture-resource-scope.md -->
# 11. Information Architecture and Resource Scope

Hostforge resources are scoped deliberately: organization, project, environment, service, deployment, and machine. Always show enough context that a user can answer “where will this action apply?” before a consequential change. Put the current project and environment near the top of the page; do not make users infer scope from a URL or a subtle color treatment.

Project-level views contain services, deployments, domains, variables, storage, and activity. Service-level views contain current state, deployments, configuration, logs, metrics, and settings. Environment context must be explicit whenever production-like and non-production resources can be confused.

<!-- END: information-architecture-resource-scope.md -->

<!-- BEGIN: navigation-command-access-search.md -->
# 12. Navigation, Command Access, and Search

Global navigation contains stable destinations, not every possible resource. Local navigation exposes the current project or service’s most frequent views. Settings belong where their scope is clear; do not create a generic settings dumping ground.

Provide command access for expert users but never require it for primary tasks. Search must reveal its scope and distinguish resources from actions. Keyboard shortcuts must be discoverable, non-destructive, and disabled while typing in text inputs unless explicitly intended.

The selected navigation state must be visible without relying only on color. Navigation labels use the product’s canonical terminology and must not change between pages for stylistic variety.

<!-- END: navigation-command-access-search.md -->

<!-- BEGIN: page-anatomy-and-density.md -->
# 13. Page Anatomy and Density

Every operational page starts with context, a clear title, current state where relevant, and the most important action. Put frequent or reversible actions in the visible action zone; place rare, advanced, or destructive actions in a clearly labelled overflow or settings region.

Use summary information before detail: state, last deployment, health, and key usage precede deep configuration or event history. Do not create dashboards full of undifferentiated cards. Each region must support a decision or an investigation.

Density should adapt to content, not personal taste. Logs and tables can be dense; onboarding and destructive confirmation should be spacious. Users should never need to scan a sea of equally weighted data.

<!-- END: page-anatomy-and-density.md -->

<!-- BEGIN: status-lifecycle-and-time.md -->
# 14. Status, Lifecycle, and Time

Status is a product contract. Use specific, observable labels: Queued, Building, Waiting for image, Deploying, Running health checks, Healthy, Degraded, Failed, Rolling back, Rolled back, Stopped, and Unknown. “Unknown” means Hostforge cannot currently verify the state; it is not a softer word for failure.

Show the object and the state together. A state badge is not enough: pair it with a timestamp, relevant reason, and a route to detail when the condition affects action. If multiple states apply, show the condition that currently requires the user’s attention, then reveal the full lifecycle in details.

Use relative time for recent events with an exact timestamp available on hover or focus. Use absolute time in audit history. State freshness explicitly when data is delayed. Retried operations must say that they were retried and whether the retry is automatic or user initiated.

<!-- END: status-lifecycle-and-time.md -->

<!-- BEGIN: actions-and-controls.md -->
# 15. Actions and Controls

Each region has one visually primary action at most. Primary actions move the current task forward; secondary actions support it; tertiary actions are links or low-emphasis controls. Forge may highlight a selected brand-level action but is not the destructive color.

Destructive actions use danger semantics and explicit verbs: Delete service, Remove domain, Roll back deployment. Never label them simply Delete when multiple objects are visible. Require confirmation only when the consequence is meaningful or irreversible; a confirmation must name the object and consequence, not merely ask “Are you sure?”

Disable a control only when its action is truly unavailable. Explain why and what unblocks it. Do not hide important actions because a prerequisite is unmet.

<!-- END: actions-and-controls.md -->

<!-- BEGIN: inputs-and-configuration-forms.md -->
# 16. Inputs and Configuration Forms

Forms are configuration workspaces, not interrogations. Group fields by the user’s mental model: source, build, runtime, networking, storage, and advanced settings. Show required fields first and disclose advanced options in a stable, labelled region.

Every input has a persistent label, helpful format guidance where needed, and an error attached to the field. Validate as early as useful without scolding users while they type. Preserve entered values after a failed submission. Do not use placeholder text as a label.

Secrets must be visually distinct from ordinary values and never exposed accidentally after saving. Make copy, rotate, reveal, and delete actions explicit. A save action must report whether it succeeded, what scope changed, and whether a redeploy or restart is required.

<!-- END: inputs-and-configuration-forms.md -->

<!-- BEGIN: data-display-and-tables.md -->
# 17. Data Display and Tables

Tables support scanning, comparison, and action. The first column identifies the resource. Numeric values align consistently; dates, status, and actions retain predictable positions. Sort order, filters, and active query state must be visible and persist when a user returns from a detail view when feasible.

Use empty states to explain why no data exists and offer the relevant next step. Do not use an empty table to imply a system failure. For loading, preserve column structure with a clear loading state; for errors, retain known data and explain what could not refresh.

Metrics must show unit, time range, and source freshness. Avoid charts without a question they answer. A chart should lead naturally to the relevant resource or event detail.

<!-- END: data-display-and-tables.md -->

<!-- BEGIN: feedback-and-system-messaging.md -->
# 18. Feedback and System Messaging

Use inline messages for field and section-specific guidance, banners for persistent page-level conditions, and toasts for short confirmations that do not require action. Do not use a toast for an error that needs investigation; keep that error visible in context.

Every failure message names the affected object, what failed, the known cause, and the next useful action. Never imply success before the system confirms it. On success, state the meaningful result: “Environment variable saved. Redeploy to apply it.”

Announcements for dynamic status must be concise and not overwhelm assistive technology. Avoid stacking notifications for events that belong in an activity feed or deployment timeline.

<!-- END: feedback-and-system-messaging.md -->

<!-- BEGIN: overlays-and-focused-tasks.md -->
# 19. Overlays and Focused Tasks

Use menus for compact choices, popovers for brief contextual information, drawers for a focused secondary task, and dialogs for decisions that must interrupt the current flow. Do not put a multi-step configuration workflow in a confirmation dialog.

An overlay must have a clear title, logical focus order, a visible close route when safe, focus restoration on dismissal, and no hidden critical information. Dialogs that confirm risk must show the object, scope, consequence, and recovery limits.

<!-- END: overlays-and-focused-tasks.md -->

<!-- BEGIN: logs-code-commands-technical-output.md -->
# 20. Logs, Code, Commands, and Technical Output

Logs are evidence. Preserve timestamps, source, level, ordering, and line wrapping controls. Offer search, copy, follow/pause, and a clear indication when output is truncated or disconnected. Do not restyle logs until they resemble a marketing terminal.

Commands and identifiers use monospace, are easy to copy, and distinguish placeholders from literal values. A copy action must confirm success accessibly. Long values should be revealable without destroying page layout. Never silently redact a value without explaining that it is redacted.

<!-- END: logs-code-commands-technical-output.md -->

<!-- BEGIN: onboarding-and-first-deployment.md -->
# 21. Onboarding and First Deployment

The first deployment flow must show the minimum required decisions, safe defaults, and the resulting infrastructure plan. Tell users what Hostforge will build, deploy, expose, and monitor. Advanced configuration remains available but does not block the first success.

Progress shows each real stage, its current status, logs, and recovery action. A successful first deployment ends with the service URL, health state, deployment reference, and a small set of meaningful next actions: view logs, configure a domain, add variables, or open service settings.

<!-- END: onboarding-and-first-deployment.md -->

<!-- BEGIN: services-builds-releases-rollbacks.md -->
# 22. Services, Builds, Releases, and Rollbacks

Make the delivery lifecycle visible: source change, build, artifact, release, health checks, and running service. A deployment detail page is the permanent explanation of one attempt: input, timeline, output, configuration snapshot, result, and recovery options.

Rollback is a deliberate operational action. Identify the target release, the currently running release, compatibility warnings, and whether data or configuration changes are outside the rollback’s scope. On failure, preserve the failed deployment’s evidence and present retry, edit configuration, inspect logs, and rollback only when each is actually available.

<!-- END: services-builds-releases-rollbacks.md -->

<!-- BEGIN: environment-secrets-and-configuration.md -->
# 23. Environment, Secrets, and Configuration

Separate ordinary environment variables from secrets without obscuring that both affect runtime behavior. Show scope, last changed time, and whether a value is applied to the current deployment. Do not reveal stored secret values by default.

Configuration changes should be reviewable before they take effect. If an action restarts or redeploys a service, say so before saving and record it afterward. Preserve a history of consequential changes when the product supports it; auditability reinforces ownership.

<!-- END: environment-secrets-and-configuration.md -->

<!-- BEGIN: domains-networking-certificates-storage.md -->
# 24. Domains, Networking, Certificates, and Storage

These resources are consequential because they connect Hostforge to real traffic and durable data. Always show ownership, current state, verification requirements, and an exact next action. For DNS, explain the expected record and verification result. For certificates, show issue, renewal, expiry, and failure state with exact dates.

For volumes, make attachment scope, mount path, persistence, capacity, and deletion consequence unmistakable. Do not make destructive storage actions visually equivalent to ordinary service configuration.

<!-- END: domains-networking-certificates-storage.md -->

<!-- BEGIN: monitoring-metrics-events-and-logs.md -->
# 25. Monitoring, Metrics, Events, and Logs

Observation starts with an answerable question: is the service healthy, when did this change, what resource is constrained, and what happened before failure? Summaries link to evidence rather than pretending to replace it.

Show time range, aggregation, units, and freshness on every metric. Mark missing data as missing; never draw a smooth continuation. Events are chronological facts with source and actor where known. Link events to the deployment, configuration change, or resource they describe.

<!-- END: monitoring-metrics-events-and-logs.md -->

<!-- BEGIN: incidents-degradation-and-recovery.md -->
# 26. Incidents, Degradation, and Recovery

During degradation, calm structure matters more than visual drama. Lead with affected scope, current state, user impact, observed cause, and recovery status. Preserve logs and prior state. Keep recovery controls visible and explain unsafe options.

Do not blame users, overstate certainty, or use vague “something went wrong” language. If Hostforge is uncertain, say what it can and cannot verify. After recovery, retain a readable event trail so users can understand what occurred.

<!-- END: incidents-degradation-and-recovery.md -->

<!-- BEGIN: accessibility-and-inclusive-operations.md -->
# 27. Accessibility and Inclusive Operations

Hostforge must meet WCAG AA for text and essential interface components. Keyboard users must reach, operate, and dismiss every control with visible focus. Dynamic state changes need appropriate screen-reader announcements without constant interruption. Color never carries status alone.

Support zoom, reflow, high-contrast conditions, reduced motion, and narrow screens. Use plain, technically accurate language; clarity aids users under time pressure and users working in a second language. Error messages must describe the problem and the recovery path, not merely announce invalidity.

<!-- END: accessibility-and-inclusive-operations.md -->

<!-- BEGIN: content-design-and-terminology.md -->
# 28. Content Design and Terminology

Write as a capable infrastructure teammate: direct, factual, concise, and never patronizing. Prefer verbs that describe real operations—Deploy, Restart, Attach, Verify, Roll back, Rotate, Remove. Avoid marketing language inside workflows.

Use consistent units and time: display a unit with every metric; avoid unexplained abbreviations; offer exact timestamps alongside recent relative time; specify timezone when ambiguity matters. State errors in this sequence when possible: object, outcome, cause, next step.

<!-- END: content-design-and-terminology.md -->

<!-- BEGIN: design-to-code-implementation.md -->
# 29. Design-to-Code Implementation

Implement foundations as semantic tokens rather than scattered literals. Components expose states and intent—not arbitrary style switches. For example, a status component accepts a documented lifecycle state; it does not accept an unconstrained color string.

Every reusable component must cover default, hover, focus, disabled, loading, error, empty, narrow-screen, and assistive-technology behavior where relevant. Test status precedence, keyboard operation, focus restoration, contrast, and destructive confirmation as product behavior, not as visual polish.

<!-- END: design-to-code-implementation.md -->

<!-- BEGIN: review-qa-and-evolution.md -->
# 30. Review, QA, and Evolution

Review UI changes at realistic data density and in success, loading, empty, degraded, and failure states. Validate desktop and narrow-screen layout, keyboard flow, focus visibility, screen-reader labels, reduced-motion behavior, and contrast. Inspect terminology and state labels as carefully as pixels.

When a pattern changes, update its canonical chapter, dependent workflows, examples, and implementation tests in the same change. Deprecate the old pattern with a named replacement and migration path. Hostforge should evolve, but never by making operators relearn the meaning of the interface.

<!-- END: review-qa-and-evolution.md -->

<!-- BEGIN: appendix-status-lexicon.md -->
# Appendix A. Status Lexicon

| Status | Meaning | User action |
| --- | --- | --- |
| Queued | Work is accepted but has not started. | View queue reason or wait. |
| Building | Source is being transformed into an artifact. | View build output. |
| Deploying | A release is being applied. | View deployment timeline. |
| Running health checks | A release is live but not yet confirmed healthy. | View health-check details. |
| Healthy | The latest known checks are passing. | No action required. |
| Degraded | The resource is operating with a known concern. | Investigate details. |
| Failed | The operation or resource did not reach its expected result. | Inspect cause and recover. |
| Rolling back | A prior release is being restored. | Observe progress. |
| Unknown | Hostforge cannot currently verify state. | Check connectivity and freshness. |

<!-- END: appendix-status-lexicon.md -->
