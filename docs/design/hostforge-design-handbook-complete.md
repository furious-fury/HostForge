# Hostforge Design Handbook â€” Expansion 01

**Status:** Planning material and merge-ready chapter.

**Canonical-source policy:** This is an additive expansion of `design-spec.md`. It does not replace, weaken, or reinterpret established decisions. If guidance conflicts, the canonical specification prevails until a formal handbook decision updates it.

---

# 1. Gap analysis

The current specification establishes Hostforgeâ€™s product position, audience, emotional goal, brand pillars, visual direction, core colors, and interaction philosophy. It is strong strategic direction. To become a long-lived handbook, it now needs the systems that let different contributors make the same decisions consistently.

## Brand expression needs operational rules

Forge, Signal, Foundation, white, and status colors have clear intent but not a complete semantic model. The handbook must define token names, surface use, dark-mode behavior, contrast requirements, and the precedence between brand color and operational status. Without this, Forge can become a generic primary-action color and Signal can be mistaken for warning or danger.

## The visual system is not yet implementable

Typography, spacing, grids, elevation, borders, radii, iconography, density, and responsive behavior are unspecified. These are not decorative details: they are the repeatable constraints that make an interface feel engineered instead of assembled.

## Information architecture is undefined

Contributors need a stable model for organization, project, environment, service, deployment, and machine scope; global and local navigation; settings; search; command access; breadcrumbs; and page anatomy. Progressive Power requires that advanced capability be discoverable, not merely buried.

## System state needs a shared language

The specification rightly treats transitional states as first-class information, but lifecycle labels, state precedence, freshness, retry messaging, timestamp rules, stale data, and partial failures remain undefined. This language must be shared across builds, deployments, services, domains, certificates, volumes, and machines.

## Components need behavioral contracts

Buttons, forms, tables, filters, empty states, dialogs, menus, feedback, loading, errors, logs, and confirmation patterns need defined anatomy, variants, states, content rules, keyboard behavior, accessibility requirements, and implementation notes.

## Workflows need end-to-end guidance

The handbook must cover first deployment, service creation, build and release observation, rollback, environment configuration, domain setup, storage, monitoring, log investigation, and recovery. Components alone cannot establish a coherent user journey.

## Accessibility, content, and governance are missing

The product needs explicit standards for contrast, focus, screen-reader announcements, reduced motion, dense data, terminology, units, dates, error language, and decision ownership. Without governance, the handbook will become a reference library rather than a source of truth.

---

# 2. Handbook architecture and chapter order

## Part I â€” Orientation

1. Handbook Charter
2. Product and Brand Position
3. Design Philosophy
4. Design Governance

## Part II â€” Foundations

5. Visual Language Foundations
6. Color and Semantic Tokens
7. Typography and Writing Hierarchy
8. Spacing, Layout, and Responsive Grids
9. Iconography, Imagery, and Illustration
10. Motion and Perceived Performance

## Part III â€” Product Structure

11. Information Architecture and Resource Scope
12. Navigation, Command Access, and Search
13. Page Anatomy and Density
14. Status, Lifecycle, and Time

## Part IV â€” Components

15. Actions and Controls
16. Inputs and Configuration Forms
17. Data Display and Tables
18. Feedback and System Messaging
19. Overlays and Focused Tasks
20. Logs, Code, Commands, and Technical Output

## Part V â€” Workflows

21. Onboarding and First Deployment
22. Services, Builds, Releases, and Rollbacks
23. Environment, Secrets, and Configuration
24. Domains, Networking, Certificates, and Storage
25. Monitoring, Metrics, Events, and Logs
26. Incidents, Degradation, and Recovery

## Part VI â€” Quality and Delivery

27. Accessibility and Inclusive Operations
28. Content Design and Terminology
29. Design-to-Code Implementation
30. Review, QA, and Evolution

## Appendices

- Token reference
- Status lexicon
- Action-risk matrix
- Copy glossary
- Keyboard shortcuts
- Design decision log

The next chapter is **Design Governance**. It follows Design Philosophy because it prevents later chapters from diluting the principles before component guidance begins.

---

# 3. Rules for preventing contradictions

## 3.1 Source precedence

Resolve conflicts in this order:

1. Approved security, safety, legal, and accessibility requirements.
2. Canonical `design-spec.md` and approved decision records.
3. Design Philosophy.
4. Foundation chapters and semantic tokens.
5. Component contracts.
6. Workflow chapters.
7. Examples, screenshots, and implementation snippets.

Examples illustrate a rule; they never override it.

## 3.2 One canonical home per definition

Define a shared concept once and link to it elsewhere. Color semantics belong in Color and Semantic Tokens; lifecycle labels belong in Status, Lifecycle, and Time; overlay behavior belongs in Overlays and Focused Tasks. Workflow chapters compose these rules but do not redefine them.

## 3.3 Use normative language deliberately

Use **must** for non-negotiable requirements, **should** for the expected default with documented exceptions, and **may** for permitted options. Avoid ambiguous terms such as â€œnormallyâ€ or â€œwhere possible.â€

## 3.4 Record consequential changes

Changes to token meaning, state labels, component behavior, navigation, terminology, or accessibility requirements require a dated decision record containing rationale, owner, affected chapters, migration work, and deprecation notes.

## 3.5 Update all dependents together

A change is incomplete until every affected component, workflow, example, glossary entry, and test expectation is updated. Search by the former token, label, component name, and terminology before merging.

## 3.6 No local semantics

Product areas may not invent their own success color, warning label, action hierarchy, confirmation pattern, or loading treatment for convenience. If the shared pattern is insufficient, extend the owning chapter instead.

## 3.7 Separate policy from implementation churn

The handbook defines intent and observable behavior. Framework-specific classes and volatile library APIs belong in linked engineering references unless they change the user experience.

## 3.8 Resolve disagreements publicly

Do not settle a handbook conflict through a local interpretation. Create a decision record, select a safe temporary behavior, and update the authoritative chapter first.

---

<!-- BEGIN: design-governance.md -->

# Design Governance

> A design system stays coherent only when decisions have owners, rules have a home, and exceptions are visible.

## Purpose

Hostforge will be maintained for years by people with different disciplines, preferences, and levels of context. Consistency will not survive on taste alone.

This chapter defines how design decisions become authoritative, how contributors resolve disagreement, and how the handbook evolves without becoming a collection of conflicting opinions.

Governance is not bureaucracy for its own sake. It protects operator confidence. A user should not have to relearn a status badge, destructive confirmation, or configuration form because a different contributor built the next screen.

## The handbook is a product contract

The handbook is the shared contract between product design, frontend engineering, documentation, and contributors. It explains both the intended experience and the constraints that make that experience repeatable.

The handbook does not replace judgment. It gives judgment a common starting point. When a contributor faces an uncovered decision, choose the option that best supports confidence, control, speed, and transparency; then document the decision if it may recur.

## Authority and precedence

Hostforge resolves conflicting guidance in this order:

1. Approved security, safety, legal, and accessibility requirements.
2. The canonical product specification and approved decision records.
3. Design Philosophy.
4. Foundations, including tokens and semantic state definitions.
5. Component contracts.
6. Workflow guidance.
7. Examples and implementation snippets.

The order matters. A workflow example cannot justify low-contrast status color. A convenient component API cannot redefine the meaning of a failed deployment. An old screenshot cannot override the current contract.

## Canonical locations

Every shared decision has one canonical home. That home contains the normative rule; every other mention links back to it.

| Decision | Canonical home |
| --- | --- |
| Product promise, audience, and pillars | Product and Brand Position |
| Interaction principles | Design Philosophy |
| Token values and semantic color use | Color and Semantic Tokens |
| Status labels and lifecycle meaning | Status, Lifecycle, and Time |
| Component anatomy and behavior | The applicable component chapter |
| Workflow sequence and task-specific content | The applicable workflow chapter |
| Accessibility requirements | Accessibility and Inclusive Operations |
| Terms, capitalization, units, and voice | Content Design and Terminology |

Do not copy a rule into another chapter merely to make that chapter feel complete. Summarize the relevant application and link to the owner. Duplication creates drift.

## Requirement language

Handbook rules use a small, deliberate vocabulary:

- **Must** means a non-negotiable requirement. A screen that violates it is incomplete.
- **Should** means the expected default. Departures require a clear, documented reason.
- **May** means an allowed option, not an expectation.

Avoid ambiguous words such as â€œusually,â€ â€œnormally,â€ and â€œwhere possibleâ€ in normative guidance. They leave contributors to infer the rule under pressure.

## Making a new decision

Before introducing a new pattern, contributors must answer:

1. Is there already a documented pattern that solves this problem?
2. Does the proposal preserve existing token, state, and action semantics?
3. Can a user understand what is happening, why, and what to do next?
4. Does it support a beginner without limiting an expert?
5. Is it accessible by keyboard, screen reader, and reduced-motion preferences?
6. Will another contributor know where and how to reuse it?

If a documented pattern solves the problem, reuse it. Consistency is usually more valuable than a locally optimized invention. If no pattern exists, document the new one in its owning chapter before treating it as reusable.

## Decision records

A decision record is required when a change affects a shared token, semantic state, component contract, navigation model, terminology, accessibility baseline, or a repeatedly used workflow.

Each record must include:

- a short, specific title;
- the decision and its effective date;
- the problem being solved;
- the rationale and, when consequential, alternatives considered;
- the owning and affected chapters;
- implementation and migration work;
- the approver or responsible maintainer; and
- a deprecation note when it replaces previous guidance.

Decision records should be concise. Their purpose is to preserve reasoning, not to recreate a meeting transcript.

## Exceptions are explicit and temporary

An exception is acceptable only when the default would harm clarity, safety, accessibility, or an important technical constraint.

Every exception must state the rule it departs from, why it is necessary, where it applies, who owns it, and when it will be reviewed. An exception must never silently establish a second standard.

For example, a warning may use a more prominent treatment for an irreversible certificate-expiry event. The justification is operational risk, not a desire for visual variety.

## Review responsibilities

Anyone changing the interface is responsible for checking their work against this handbook. Reviewers protect shared patterns, not merely visual polish.

Interface review must confirm:

- semantic state and color remain truthful;
- action hierarchy matches consequence and frequency;
- advanced controls are disclosed deliberately rather than concealed;
- failure states offer a factual recovery path;
- keyboard and assistive-technology behavior are included;
- loading, empty, and degraded states are designed, not deferred;
- terminology matches the glossary; and
- new reusable behavior has a canonical home.

## Deprecation and migration

Hostforge should not preserve weak patterns merely because they already exist. It should also not change familiar patterns casually.

When a pattern is replaced, the handbook must name the replacement, mark the old pattern deprecated, explain the migration path, and identify every affected surface. Product code should not contain both patterns indefinitely without a stated reason.

During migration, do not present two different meanings for the same status, color, or action. If coexistence is unavoidable, prioritize the safer and more explicit treatment.

## The final test

Before approving a change, ask:

> Would this make Hostforge more predictable for someone operating their own infrastructure at a stressful moment?

If the answer is no, the change needs stronger justification.

<!-- END: design-governance.md -->
# Hostforge Design Handbook â€” Expansion 02

This merge-ready addition deepens the visual-token and component chapters of the Hostforge Design Handbook. Append after the existing handbook content.

<!-- BEGIN: foundation-tokens.md -->
# Foundation Token Reference

## Token principles

Tokens describe purpose, not appearance. Use `color.status.danger` rather than a literal red; use `space.4` rather than a one-off margin. A token can change without requiring every screen to reinterpret its intent.

Do not introduce a token to solve a single local layout problem. First use an existing semantic or primitive token. A new token requires two or more legitimate uses, a stable name, and a documented relationship to the system.

## Color tokens

| Token | Value | Use |
| --- | --- | --- |
| `color.brand.forge` | `#A41623` | Brand identity, selected state, high-attention non-destructive emphasis. |
| `color.brand.signal` | `#FFB320` | Build/deployment activity and operational progress. |
| `color.foundation.900` | `#262626` | Primary text, navigation, code surfaces. |
| `color.canvas.default` | `#FFFFFF` | Primary workspace canvas. |
| `color.canvas.subtle` | `#F7F7F5` | Recessed or grouped light surfaces. |
| `color.border.default` | `#D9D9D5` | Standard structural separation. |
| `color.text.primary` | `#262626` | Primary readable text. |
| `color.text.secondary` | `#5C5C58` | Supporting text. |
| `color.text.muted` | `#767670` | Metadata only; never essential text on white without contrast validation. |
| `color.status.success` | `#18794E` | Healthy, completed, confirmed operation. |
| `color.status.warning` | `#A15C00` | Attention needed; operation can continue or recover. |
| `color.status.danger` | `#B42318` | Failure and destructive consequence. |
| `color.status.info` | `#2F5DA8` | Neutral operational information. |

Status text and essential icons must meet a 4.5:1 contrast ratio against their immediate background. Use a pale status background only as supporting context; the label itself must remain readable without it. Never substitute Forge for danger or Signal for warning.

## Typography tokens

The product uses a modern, highly legible sans-serif UI face with a system fallback. A mono stack is reserved for source branches, IDs, commands, logs, ports, paths, image names, and environment-variable values.

| Token | Size / line height | Intended use |
| --- | --- | --- |
| `type.display` | 32 / 40 | Rare top-level product or empty-state title. |
| `type.page-title` | 24 / 32 | Page title. |
| `type.section-title` | 18 / 26 | Page section heading. |
| `type.body` | 14 / 22 | Default interface prose. |
| `type.label` | 13 / 18 | Form labels, compact controls, table metadata. |
| `type.meta` | 12 / 16 | Secondary metadata; never essential instructions. |
| `type.code` | 13 / 20 | Monospace technical content. |

Use weight to differentiate hierarchy sparingly: 400 for body, 500 for labels and active navigation, 600 for headings and high-attention values. Avoid 700 for ordinary interface text. Uppercase labels are prohibited except for established technical acronyms.

## Spacing, form, and elevation

The base unit is 4 px. Approved spacing steps are 4, 8, 12, 16, 20, 24, 32, 40, 48, 64, and 80 px. Use 8 or 12 inside compact controls, 16 or 24 within grouped regions, and 32 or 48 between page regions.

| Token | Value | Use |
| --- | --- | --- |
| `radius.control` | 6 px | Inputs, buttons, compact menus. |
| `radius.surface` | 8 px | Panels and cards. |
| `radius.status` | 999 px | Status badges only. |
| `border.default` | 1 px | Standard divider and surface boundary. |
| `shadow.overlay` | `0 8px 24px rgba(38,38,38,.16)` | Menus, dialogs, drawers only. |

Surfaces use borders before shadows. Do not add elevation to a static page region merely to make it look important. A shadow means the object floats above its context.

## Breakpoints and density

Use layout behavior, not device names: compact below 640 px, medium from 640â€“1023 px, and wide at 1024 px and above. Pages must remain functional at 200% browser zoom. Do not gate essential information behind hover.

Compact density is appropriate for tables, logs, and service inventories. Comfortable density is appropriate for configuration, onboarding, and risky tasks. A user must be able to choose density only where it materially changes scanning efficiency; do not make density a global cosmetic preference.

<!-- END: foundation-tokens.md -->

<!-- BEGIN: component-actions.md -->
# Component Contract: Actions and Controls

## Buttons

A button initiates an action in the current interface. Links navigate. Do not use a button for navigation merely because it looks more prominent.

| Variant | Use | Rules |
| --- | --- | --- |
| Primary | The single main action for the current task. | One per action region. Must use an explicit verb. |
| Secondary | A meaningful supporting action. | May appear beside primary action. |
| Tertiary | Low-emphasis, reversible, or contextual action. | Must remain visibly interactive. |
| Danger | Irreversible or destructive action. | Use exact destructive verb; never visually primary by default. |
| Icon | Familiar compact action. | Requires accessible name and tooltip. |

Buttons have a minimum interactive target of 40 Ã— 40 px on touch-capable layouts. Loading replaces the action icon with progress while preserving the label and width. A button must not become disabled immediately after a click unless duplicate submission is genuinely unsafe; if it does, expose the resulting operation state nearby.

### Do

- Use â€œDeploy service,â€ â€œSave variables,â€ and â€œRoll back to release 42.â€
- Keep the primary action close to the information needed to decide.
- Explain disabled actions: â€œAdd a repository to deploy.â€

### Do not

- Use â€œSubmit,â€ â€œContinue,â€ or â€œClick hereâ€ without a task-specific meaning.
- Place multiple competing primary buttons in a header.
- Use Forge or Signal to make destructive actions feel brand-like or celebratory.

## Toggle, checkbox, and radio selection

A toggle changes one setting immediately and must state the resulting state in its label, for example â€œEnable automatic deploys.â€ Use a checkbox for a selection committed later with a form. Use radio options when one mutually exclusive selection must remain visible for comparison.

If a toggle causes a restart, deployment, cost, or exposure change, show the consequence before or immediately after the action. Never rely on an animated switch alone to convey an operationally meaningful change.

<!-- END: component-actions.md -->

<!-- BEGIN: component-status.md -->
# Component Contract: Status and Progress

## Status badge anatomy

A status badge contains a semantic icon or dot, a text label, and optional secondary detail outside the badge. It must not be the only presentation of a critical state. Pair it with timestamp, object, or reason at page and table level.

Badges are compact and scan-friendly. They use a subtle tinted background only when it improves grouping; status meaning is carried by the text and accessible label. Do not use a badge for arbitrary taxonomy tags.

## Progress

Use determinate progress only when Hostforge can calculate meaningful completion. Build and deployment stages commonly cannot promise a trustworthy percentage; show the current stage, elapsed time, and last event instead. Signal is reserved for active operational progress. A completed operation transitions to success; a stalled operation transitions to a named waiting or degraded state, never an eternal spinner.

## State precedence

When states conflict, surface the one requiring action: Failed overrides Building; Degraded overrides Healthy; Rolling back overrides the state of the release being replaced; Unknown overrides stale Healthy data. Details retain the full timeline.

<!-- END: component-status.md -->

<!-- BEGIN: component-forms.md -->
# Component Contract: Inputs and Configuration Forms

## Input anatomy

Every input must provide a persistent visible label, control, optional concise help text, and validation message when relevant. Mark required fields in the help or label using text; do not use an asterisk without explanation. Placeholder text is an example or format hint, never the sole label.

Text fields use an appropriate autocomplete attribute and input mode where available. Avoid auto-focusing fields on page load; it disrupts context and screen readers. For technical values, show the expected syntax and a valid example without making users translate prose into configuration.

## Validation and saving

Validate immediately only when failure is certain, such as malformed syntax. Validate remote constraints after a meaningful pause or submission, and distinguish â€œnot yet checkedâ€ from â€œinvalid.â€ Keep errors next to their field and summarize errors at the top for multi-field forms. Focus the summary after failed submission only when it improves recovery.

Save actions must reveal scope and effect. For configuration that needs redeployment, use language such as: â€œSave changes â€” deploy required.â€ After saving, state whether the running service changed, a deployment started, or further action is needed.

## Secret input

Secret values are masked after entry and never repopulated from the server. Reveal is a deliberate temporary action with an accessible state, not an eye icon with no explanation. Copying, rotating, and deleting secrets are explicit actions with audit-worthy confirmation where appropriate.

<!-- END: component-forms.md -->

<!-- BEGIN: component-tables.md -->
# Component Contract: Tables and Resource Lists

Use a table when users need to scan many comparable resources or values. Use a list when each item requires a richer, variable summary. A table must have a labelled caption or accessible name, stable column headers, and a predictable row-action location.

The first column identifies the resource and should remain visible during horizontal scrolling where technically feasible. State has a dedicated column; do not bury it inside metadata. Row click navigation must not interfere with text selection or a rowâ€™s explicit actions.

Filters appear above the data and show active values as removable, readable controls. Clear filters restores a predictable default. Empty state language distinguishes no resources, no matching results, lack of access, and unavailable data. Pagination must preserve filters and sort state.

### Table review checklist

- Can a user identify the resource, current state, and next investigation path without opening each row?
- Are units, time range, and data freshness visible where relevant?
- Does narrow-screen behavior preserve identity and consequential actions?
- Is sorting announced and keyboard operable?

<!-- END: component-tables.md -->

<!-- BEGIN: component-feedback-overlays.md -->
# Component Contract: Feedback and Overlays

## Feedback

Use an inline message when it belongs to a field or section, a banner for a persistent page-level condition, and a toast for a brief completion that does not require a decision. Toasts must not be the only record of a meaningful deployment or configuration result.

Feedback messages use this order: affected object, result, known reason, next step. For example: â€œAPI deployment failed during health checks. The `/health` endpoint returned 503. View deployment logs.â€ Do not dilute failure language with apology or vague reassurance.

## Dialogs and drawers

A dialog confirms a decision that needs focused attention. A drawer supports a focused secondary task while preserving page context. Menus list short actions; they are not forms.

On opening an overlay, move focus to its title or first relevant control. Trap focus only for modal dialogs. Escape closes safe overlays; a destructive confirmation may require an explicit choice but must still provide a clear cancel action. Return focus to the invoking element on dismissal whenever it remains available.

Never put long logs, multi-step setup, or core navigation inside a modal dialog. If a task needs a URL, persistence, or substantial context, it deserves a page.

<!-- END: component-feedback-overlays.md -->
# Hostforge Design Handbook â€” Expansion 03

This merge-ready addition deepens the operational workflow chapters. Append it after the existing handbook and component expansion.

<!-- BEGIN: workflow-first-deployment.md -->
# Workflow: First Deployment

## Goal

The first deployment proves that Hostforge can make self-managed compute understandable without pretending it is effortless. A user should reach a healthy service with a clear understanding of the source, configuration, deployment state, and URL they now own.

## Flow structure

The primary path is: choose source â†’ name service â†’ review runtime defaults â†’ configure required settings â†’ review plan â†’ deploy â†’ observe â†’ arrive at the service overview.

Present one decision group at a time, but retain a visible summary of completed choices. Do not make users complete an opaque wizard. A user must be able to go back, edit a choice, and understand the impact without losing entered configuration.

### 1. Choose source

Explain what Hostforge can deploy: a connected repository, container image, or supported template. Identify the selected repository, branch, and commit where available. If authorization is missing, say what Hostforge needs access to and provide a recoverable connection flow.

Do not show advanced build controls before the user has a valid source. Make them available immediately after source selection in a labelled **Advanced build settings** section.

### 2. Name and scope the service

Show organization, project, and environment scope before the name field. Use a readable service name and show how it affects the generated internal identifier or URL when relevant. Warn before creating a service in a production environment, but do not use alarmist styling.

### 3. Configure runtime

Start with detected or safe defaults: build method, exposed port, health-check path when known, region or machine selection when applicable, and public/private exposure. Explain the origin of inferred values: â€œDetected from Dockerfileâ€ is more trustworthy than simply pre-filling a field.

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

Show service name, project and environment context, current lifecycle state, public endpoint when applicable, running release, and the latest deployment outcome. Follow with a small operational summaryâ€”health, request or resource signal where available, and recent eventsâ€”then direct links to logs, deployments, configuration, metrics, and settings.

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

Before rollback, present the target release, the current release, the targetâ€™s age, source revision, configuration compatibility notice, and the consequence: configuration, data migrations, external services, and attached storage may not revert. The confirmation reads **Roll back to release [identifier]**.

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

The variables list shows key, type, source or scope, last changed time, and whether a change is pending deployment. Values are not normally displayed in a resource table. A variableâ€™s key should be easy to copy and use a mono type treatment.

Adding a variable requires a key, value, scope, and an explicit type when the product distinguishes ordinary values from secrets. Validate the key using clear format guidance. Prevent duplicate keys within the same scope and explain which inherited key is being overridden when applicable.

## Secrets

Secrets require the same configuration clarity as variables with stricter visibility. After storage, show the key, scope, last changed time, and actor where availableâ€”not the secret value. A user may copy a supplied value before saving, but Hostforge must not imply the stored value can be retrieved later.

Reveal, rotate, and delete are deliberate actions. Rotation must explain whether the old value remains in use until redeploy or restart. Deletion must identify dependent services and require a confirmation proportionate to the resulting runtime risk.

## Change and apply

Saving configuration must report its effect honestly:

- **Applied immediately** for settings that Hostforge can change without restarting.
- **Pending restart** when the running process needs a restart.
- **Pending deployment** when a new release is required.
- **Not applied** when validation or platform constraints prevent storage.

Do not show â€œSavedâ€ as the only result when a service is still running old configuration. Provide an appropriate next action, such as **Deploy latest configuration**.

## Audit and recovery

For consequential settings, retain an event record showing what changed, where, when, and by whom without exposing secret values. If the product supports versioning, clearly identify the version used by the currently running release and provide a reversible path for non-secret configuration.

<!-- END: workflow-configuration-and-secrets.md -->

<!-- BEGIN: workflow-domains-networking-storage.md -->
# Workflow: Domains, Networking, Certificates, and Storage

## Domains

Adding a domain is a verification workflow, not a simple text field. First validate the hostname and its target service/environment. Then display the exact DNS record Hostforge expects, including record type, name, value, and any propagation caveat. Provide copy controls for values and show the last verification attempt.

Domain states are explicit: Needs configuration, Verifying, Verified, Certificate provisioning, Active, Expiring soon, and Failed. â€œActiveâ€ means the domain is routed and its certificate is valid; it must not mean merely that the DNS record was submitted.

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

Logs are a primary investigation surface. Users can follow live output, pause follow, search, filter by source or severity when supported, copy selected content, and see whether output is truncated or disconnected. New lines must not move a user who is reading older output; show a â€œnew logsâ€ control instead.

Log entries preserve timestamp, source, level, and raw message. Wrapping is configurable. Errors that reference a deployment, release, request, or machine should link to the relevant evidence without obscuring the original text.

## Events

Events are durable facts: deployment started, configuration changed, certificate renewed, volume attached, machine restarted, and health check failed. Each event includes time, object, state, source, actor when known, and a detail path. Events are not a substitute for logs; they orient the user before detailed investigation.

## Incident presentation

When Hostforge detects or receives a failure, lead with affected scope, observed state, impact, first known time, and recovery status. Then provide evidence, causal information only when known, and the safest next actions. The interface must distinguish confirmed facts from inference.

Use a steady structure during incidents. Do not add panic-inducing animation, dramatic color wash, or ambiguous alarm copy. A red failure state is enough when paired with factual recovery guidance.

## Recovery completion

After recovery, show what changed, when service returned to a healthy state, and any unresolved follow-up. Retain the incident timeline so users can learn from it. Never silently remove evidence because the current state is healthy again.

<!-- END: workflow-monitoring-logs-incidents.md -->
# Hostforge Design Handbook â€” Expansion 04

This merge-ready addition defines the quality and delivery layer for the Hostforge Design Handbook. Append it after the existing handbook and prior expansions.

<!-- BEGIN: accessibility-and-inclusive-operations-detailed.md -->
# Accessibility and Inclusive Operations

## Standard

Hostforge must meet WCAG 2.2 AA for the product interface. Accessibility is not a compliance pass after visual design. In infrastructure software it is operational reliability: users need to perceive status, recover from errors, and execute actions under stress, at zoom, with a keyboard, and with assistive technology.

When an accessibility requirement conflicts with a stylistic preference, accessibility wins. When an accessibility requirement conflicts with an established component pattern, change the component pattern rather than introducing an undocumented local exception.

## Keyboard interaction

Every interactive element must be reachable in a predictable reading order, visibly focused, and operable without a pointer. Use native HTML controls wherever possible. Do not simulate buttons, links, checkboxes, or inputs with generic elements unless there is a documented technical reason and equivalent semantics, keyboard handling, and accessible naming are supplied.

Focus moves only when it helps the user continue a task. Examples: opening a dialog moves focus into it; failed form submission may move focus to an error summary; dismissing an overlay returns focus to its trigger. New logs, polling updates, and successful background refreshes must never steal focus.

Provide keyboard support for menus, dialogs, tabs, table sorting, comboboxes, command access, and list selection. Escape closes non-destructive overlays. Do not make keyboard shortcuts fire while a user is typing in an input, editor, or terminal-like control unless the shortcut is explicitly designed for that context.

## Focus visibility

Every focusable element needs a visible, high-contrast focus indicator that is not removed by custom styling. A 2 px or greater focus ring with sufficient contrast against adjacent surfaces is the default. Do not rely on a subtle color fill, browser default outline suppression, or hover styling to indicate focus.

## Color and contrast

Text and essential controls meet a 4.5:1 contrast ratio; large text may meet 3:1 only when it meets the WCAG definition of large text. Focus indicators and non-text controls meet a 3:1 contrast ratio against adjacent colors. Disabled controls may use lower contrast only when they are truly unavailable and their disabled meaning is not required to understand current system state.

Never express success, warning, failure, selected state, or required-field state with color alone. Pair color with a label, icon, pattern, or explicit textual state. A green dot without â€œHealthyâ€ is not an accessible status indicator.

## Dynamic content and announcements

Use live regions sparingly. Announce a material transitionâ€”â€œDeployment failed during health checksâ€â€”once. Do not announce each new log line, every chart refresh, or the movement of a progress indicator. Important errors must remain present in the page after announcement.

For long-running operations, expose status, stage, elapsed time, and details in the DOM; a spinner alone is not sufficient. If data becomes stale or unavailable, communicate that condition in text near the affected content.

## Motion, zoom, and reflow

Respect `prefers-reduced-motion`. Remove nonessential movement, reduce animated transitions to an immediate state change or opacity transition, and never make progress understandable only through motion.

Support text resizing and 200% browser zoom without loss of content or function. At narrow widths, content may reflow but must not require two-dimensional scrolling except for inherently wide data such as code and tables. Provide an understandable compact treatment for primary table columns and actions.

## Forms and errors

Labels remain visible while users type. Required fields, expected formats, and error messages are programmatically associated with their controls. Do not announce validation failure before the user has had a meaningful chance to complete a field. Preserve submitted values whenever safe after an error.

Errors identify the affected field or resource, the problem, and a recoverable next step. â€œInvalid valueâ€ is insufficient. Prefer â€œPort must be a number from 1 to 65535.â€

## Accessibility acceptance checklist

- All functionality is keyboard-operable without a timing trap.
- Focus order follows visual and task order.
- Focus remains visible in every state.
- Icons and controls have accessible names.
- Status does not rely on color, shape, or motion alone.
- Dynamic updates are announced only when material and do not interrupt reading.
- Zoom, narrow screens, and reduced motion preserve function.
- Errors are associated, understandable, and recoverable.

<!-- END: accessibility-and-inclusive-operations-detailed.md -->

<!-- BEGIN: content-design-and-terminology-detailed.md -->
# Content Design and Terminology

## Voice

Hostforge writes like a calm, technically capable teammate. The voice is direct, factual, and respectful. It avoids cheerleading, blame, filler, and faux-human apology. The product should explain infrastructure without treating users as novices or requiring them to already know every concept.

Prefer concrete verbs and nouns: Deploy, Restart, Roll back, Verify, Attach, Rotate, Remove; service, deployment, release, environment, domain, certificate, volume, machine. Avoid vague verbs such as Process, Handle, Complete, and Manage when a precise operation exists.

## Sentence patterns

For normal guidance, lead with the action or result: â€œConnect a repository to create your first service.â€ For errors, use this order:

1. affected object;
2. outcome;
3. known cause; and
4. next useful action.

Example: â€œAPI deployment failed during health checks. The `/health` endpoint returned 503. Check logs or update the health-check path.â€

Do not claim certainty that the system does not have. â€œHostforge could not verify the DNS record yetâ€ is more trustworthy than â€œDNS verification failedâ€ when propagation is still possible.

## Labels and actions

Labels name the thing; actions start with a verb. Use **Deployment history**, not **View deployments** as a navigation label. Use **View deployment logs**, not **Logs** when action context is required. Use **Delete volume**, not **Delete**.

Avoid â€œclick,â€ â€œtap,â€ â€œsimply,â€ â€œjust,â€ â€œeasy,â€ and â€œobviously.â€ They add no instruction and can sound dismissive. Use sentence case for labels, controls, and messages.

## Numbers, dates, and units

Every metric displays a unit. Use familiar binary or decimal storage units consistently and document the choice. Use a narrow no-break space or normal space between value and unit according to implementation conventions, but never concatenate them ambiguously. Preserve meaningful precision; do not show six decimals for a percentage a user acts on as a whole number.

Use relative time for recent eventsâ€”â€œ3 minutes agoâ€â€”and make the exact timestamp available on hover, focus, or detail. Use absolute time for audit history and expiry. State the timezone when users could reasonably interpret time differently; account-level timezone handling must be consistent across the product.

## Terminology contract

| Term | Definition | Do not use for |
| --- | --- | --- |
| Service | A running application workload managed by Hostforge. | A deployment attempt or machine. |
| Build | The transformation of source into a runnable artifact. | The running release. |
| Deployment | One attempt to deliver a release to runtime. | A service itself. |
| Release | The versioned runnable result selected for runtime. | A source branch. |
| Environment | A scoped configuration and runtime context, such as production. | A generic variable list. |
| Secret | A protected configuration value whose content is not displayed after storage. | Any ordinary environment variable. |
| Domain | A hostname routed to a Hostforge service. | A URL path or internal service name. |
| Volume | Persistent storage attached to a workload. | Ephemeral filesystem content. |

## Content review checklist

- Is the object named before or with the state?
- Is the action specific and consequence-aware?
- Does error text explain a recovery path?
- Are terms consistent with the glossary?
- Are units, time ranges, freshness, and timezones clear where relevant?
- Is the language factual without marketing or blame?

<!-- END: content-design-and-terminology-detailed.md -->

<!-- BEGIN: design-to-code-detailed.md -->
# Design-to-Code Implementation

## Tokens are the interface contract

Implement visual foundations as named tokens exposed to components. Product code must not scatter raw hex values, arbitrary spacing values, or ad hoc shadows. A semantic component consumes intentâ€”for example, `status="degraded"`â€”and resolves it to the approved visual and accessible treatment.

Separate primitive tokens from semantic tokens. Primitive tokens contain scales such as `red.700`, `space.16`, and `radius.6`. Semantic tokens express product meaning: `color.status.danger`, `color.surface.default`, `color.action.primary`. Application code should use semantic tokens except inside the foundation implementation itself.

## Component API rules

Component props express product state and behavior, not a bag of visual overrides. Prefer `Button variant="danger" loading` over arbitrary `backgroundColor`, `radius`, and `textColor` props. Prefer `Status state="healthy"` over `color="green"` and `label="Good"`.

Every reusable component documents its supported variants, states, keyboard behavior, ARIA semantics, content limits, and responsive behavior. Unsupported combinations must fail visibly in development or be prevented by the type system where available.

## Required states

Each component must define applicable default, hover, focus-visible, active, disabled, loading, error, empty, and narrow-layout behavior before it is considered complete. A visual snapshot of the default state is not adequate coverage.

Long-running action controls preserve their label while loading. A list component distinguishes initial load, refresh with cached data, empty results, filtered empty results, permission limitation, and retrieval failure. These states have different user meaning and must not collapse into one generic skeleton.

## Accessibility in implementation

Semantic HTML is the default implementation. ARIA supplements native semantics; it does not replace them. All icon-only controls need accessible names. Programmatically associated labels, descriptions, and errors are required for inputs. Overlays restore focus; modals manage focus; dynamic announcements are intentionally scoped.

Do not use title attributes as the only accessible name or explanation. Do not make a clickable `div` when a button or link is appropriate. Do not rely on visual order when DOM order can be made logical.

## Tests and fixtures

Every shared component has representative fixtures for all supported states. Automated tests cover keyboard operation, accessible names, focus management, semantic status labels, and high-risk state transitions. Visual tests cover density, long text, empty data, failure, and narrow layouts.

Use realistic infrastructure content in fixtures: long image names, failed health checks, inactive domains, dense logs, missing metrics, and destructive confirmations. Happy-path placeholder text hides the conditions that most often break operational interfaces.

<!-- END: design-to-code-detailed.md -->

<!-- BEGIN: review-qa-and-evolution-detailed.md -->
# Review, QA, and Evolution

## Required review states

Every user-facing change is reviewed in its relevant states: initial load, loaded data, empty data, filtered empty data, validation failure, server failure, degraded or stale data, success, long text, and compact layout. A deployment feature must be reviewed while queued, active, healthy, and failedâ€”not only in a polished healthy screenshot.

Review at realistic density. Use long service names, multiple environments, older timestamps, dense tables, clipped log output, and slow or incomplete data. Infrastructure software fails first at the edges; the review system must look there before users do.

## Interface review checklist

### Clarity and hierarchy

- Can a user identify current scope, state, and next action in the first viewport?
- Does each visual region support a decision, action, or investigation?
- Is there one clear primary action per task region?
- Is advanced capability discoverable without interrupting the primary path?

### Operational truth

- Are status labels specific and supported by known system state?
- Are retries, staleness, uncertainty, and failure represented honestly?
- Does the screen distinguish a saved configuration change from an applied runtime change?
- Are destructive consequences and rollback limits explicit?

### System consistency

- Does the screen reuse the canonical token, component, status, and terminology rules?
- Has it introduced a local color, icon, state label, or action hierarchy?
- Are loading, empty, and error treatments consistent with the shared patterns?

### Accessibility and resilience

- Is the flow usable with keyboard, focus visibility, zoom, and reduced motion?
- Are controls named and dynamic state changes announced appropriately?
- Does narrow layout preserve task completion and critical context?

## Change management

When a shared pattern changes, update its canonical chapter, implementation, automated checks, examples, and dependent workflow guidance in the same change. Mark superseded guidance as deprecated and name its replacement. Do not leave two competing patterns in production indefinitely without a decision record.

New patterns enter the system only after a demonstrated repeated need, a documented owner, a meaningful API, a state model, and accessibility behavior. A component is not ready because it looks reusable; it is ready when another contributor can apply it correctly without reverse-engineering its intent.

## Definition of done

A user-facing feature is complete when its happy path, operational edge states, recovery path, accessible behavior, content, and implementation contract are all reviewed. â€œWe will handle errors laterâ€ is not an acceptable completion state for infrastructure software.

<!-- END: review-qa-and-evolution-detailed.md -->
