# Hostforge Design Handbook — Expansion 04

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

Never express success, warning, failure, selected state, or required-field state with color alone. Pair color with a label, icon, pattern, or explicit textual state. A green dot without “Healthy” is not an accessible status indicator.

## Dynamic content and announcements

Use live regions sparingly. Announce a material transition—“Deployment failed during health checks”—once. Do not announce each new log line, every chart refresh, or the movement of a progress indicator. Important errors must remain present in the page after announcement.

For long-running operations, expose status, stage, elapsed time, and details in the DOM; a spinner alone is not sufficient. If data becomes stale or unavailable, communicate that condition in text near the affected content.

## Motion, zoom, and reflow

Respect `prefers-reduced-motion`. Remove nonessential movement, reduce animated transitions to an immediate state change or opacity transition, and never make progress understandable only through motion.

Support text resizing and 200% browser zoom without loss of content or function. At narrow widths, content may reflow but must not require two-dimensional scrolling except for inherently wide data such as code and tables. Provide an understandable compact treatment for primary table columns and actions.

## Forms and errors

Labels remain visible while users type. Required fields, expected formats, and error messages are programmatically associated with their controls. Do not announce validation failure before the user has had a meaningful chance to complete a field. Preserve submitted values whenever safe after an error.

Errors identify the affected field or resource, the problem, and a recoverable next step. “Invalid value” is insufficient. Prefer “Port must be a number from 1 to 65535.”

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

For normal guidance, lead with the action or result: “Connect a repository to create your first service.” For errors, use this order:

1. affected object;
2. outcome;
3. known cause; and
4. next useful action.

Example: “API deployment failed during health checks. The `/health` endpoint returned 503. Check logs or update the health-check path.”

Do not claim certainty that the system does not have. “Hostforge could not verify the DNS record yet” is more trustworthy than “DNS verification failed” when propagation is still possible.

## Labels and actions

Labels name the thing; actions start with a verb. Use **Deployment history**, not **View deployments** as a navigation label. Use **View deployment logs**, not **Logs** when action context is required. Use **Delete volume**, not **Delete**.

Avoid “click,” “tap,” “simply,” “just,” “easy,” and “obviously.” They add no instruction and can sound dismissive. Use sentence case for labels, controls, and messages.

## Numbers, dates, and units

Every metric displays a unit. Use familiar binary or decimal storage units consistently and document the choice. Use a narrow no-break space or normal space between value and unit according to implementation conventions, but never concatenate them ambiguously. Preserve meaningful precision; do not show six decimals for a percentage a user acts on as a whole number.

Use relative time for recent events—“3 minutes ago”—and make the exact timestamp available on hover, focus, or detail. Use absolute time for audit history and expiry. State the timezone when users could reasonably interpret time differently; account-level timezone handling must be consistent across the product.

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

Implement visual foundations as named tokens exposed to components. Product code must not scatter raw hex values, arbitrary spacing values, or ad hoc shadows. A semantic component consumes intent—for example, `status="degraded"`—and resolves it to the approved visual and accessible treatment.

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

Every user-facing change is reviewed in its relevant states: initial load, loaded data, empty data, filtered empty data, validation failure, server failure, degraded or stale data, success, long text, and compact layout. A deployment feature must be reviewed while queued, active, healthy, and failed—not only in a polished healthy screenshot.

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

A user-facing feature is complete when its happy path, operational edge states, recovery path, accessible behavior, content, and implementation contract are all reviewed. “We will handle errors later” is not an acceptable completion state for infrastructure software.

<!-- END: review-qa-and-evolution-detailed.md -->
