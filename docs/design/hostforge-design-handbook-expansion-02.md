# Hostforge Design Handbook — Expansion 02

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

Use layout behavior, not device names: compact below 640 px, medium from 640–1023 px, and wide at 1024 px and above. Pages must remain functional at 200% browser zoom. Do not gate essential information behind hover.

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

Buttons have a minimum interactive target of 40 × 40 px on touch-capable layouts. Loading replaces the action icon with progress while preserving the label and width. A button must not become disabled immediately after a click unless duplicate submission is genuinely unsafe; if it does, expose the resulting operation state nearby.

### Do

- Use “Deploy service,” “Save variables,” and “Roll back to release 42.”
- Keep the primary action close to the information needed to decide.
- Explain disabled actions: “Add a repository to deploy.”

### Do not

- Use “Submit,” “Continue,” or “Click here” without a task-specific meaning.
- Place multiple competing primary buttons in a header.
- Use Forge or Signal to make destructive actions feel brand-like or celebratory.

## Toggle, checkbox, and radio selection

A toggle changes one setting immediately and must state the resulting state in its label, for example “Enable automatic deploys.” Use a checkbox for a selection committed later with a form. Use radio options when one mutually exclusive selection must remain visible for comparison.

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

Validate immediately only when failure is certain, such as malformed syntax. Validate remote constraints after a meaningful pause or submission, and distinguish “not yet checked” from “invalid.” Keep errors next to their field and summarize errors at the top for multi-field forms. Focus the summary after failed submission only when it improves recovery.

Save actions must reveal scope and effect. For configuration that needs redeployment, use language such as: “Save changes — deploy required.” After saving, state whether the running service changed, a deployment started, or further action is needed.

## Secret input

Secret values are masked after entry and never repopulated from the server. Reveal is a deliberate temporary action with an accessible state, not an eye icon with no explanation. Copying, rotating, and deleting secrets are explicit actions with audit-worthy confirmation where appropriate.

<!-- END: component-forms.md -->

<!-- BEGIN: component-tables.md -->
# Component Contract: Tables and Resource Lists

Use a table when users need to scan many comparable resources or values. Use a list when each item requires a richer, variable summary. A table must have a labelled caption or accessible name, stable column headers, and a predictable row-action location.

The first column identifies the resource and should remain visible during horizontal scrolling where technically feasible. State has a dedicated column; do not bury it inside metadata. Row click navigation must not interfere with text selection or a row’s explicit actions.

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

Feedback messages use this order: affected object, result, known reason, next step. For example: “API deployment failed during health checks. The `/health` endpoint returned 503. View deployment logs.” Do not dilute failure language with apology or vague reassurance.

## Dialogs and drawers

A dialog confirms a decision that needs focused attention. A drawer supports a focused secondary task while preserving page context. Menus list short actions; they are not forms.

On opening an overlay, move focus to its title or first relevant control. Trap focus only for modal dialogs. Escape closes safe overlays; a destructive confirmation may require an explicit choice but must still provide a clear cancel action. Return focus to the invoking element on dismissal whenever it remains available.

Never put long logs, multi-step setup, or core navigation inside a modal dialog. If a task needs a URL, persistence, or substantial context, it deserves a page.

<!-- END: component-feedback-overlays.md -->
