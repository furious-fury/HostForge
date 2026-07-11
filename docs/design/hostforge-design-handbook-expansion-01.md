# Hostforge Design Handbook — Expansion 01

**Status:** Planning material and merge-ready chapter.

**Canonical-source policy:** This is an additive expansion of `design-spec.md`. It does not replace, weaken, or reinterpret established decisions. If guidance conflicts, the canonical specification prevails until a formal handbook decision updates it.

---

# 1. Gap analysis

The current specification establishes Hostforge’s product position, audience, emotional goal, brand pillars, visual direction, core colors, and interaction philosophy. It is strong strategic direction. To become a long-lived handbook, it now needs the systems that let different contributors make the same decisions consistently.

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

## Part I — Orientation

1. Handbook Charter
2. Product and Brand Position
3. Design Philosophy
4. Design Governance

## Part II — Foundations

5. Visual Language Foundations
6. Color and Semantic Tokens
7. Typography and Writing Hierarchy
8. Spacing, Layout, and Responsive Grids
9. Iconography, Imagery, and Illustration
10. Motion and Perceived Performance

## Part III — Product Structure

11. Information Architecture and Resource Scope
12. Navigation, Command Access, and Search
13. Page Anatomy and Density
14. Status, Lifecycle, and Time

## Part IV — Components

15. Actions and Controls
16. Inputs and Configuration Forms
17. Data Display and Tables
18. Feedback and System Messaging
19. Overlays and Focused Tasks
20. Logs, Code, Commands, and Technical Output

## Part V — Workflows

21. Onboarding and First Deployment
22. Services, Builds, Releases, and Rollbacks
23. Environment, Secrets, and Configuration
24. Domains, Networking, Certificates, and Storage
25. Monitoring, Metrics, Events, and Logs
26. Incidents, Degradation, and Recovery

## Part VI — Quality and Delivery

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

Use **must** for non-negotiable requirements, **should** for the expected default with documented exceptions, and **may** for permitted options. Avoid ambiguous terms such as “normally” or “where possible.”

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

Avoid ambiguous words such as “usually,” “normally,” and “where possible” in normative guidance. They leave contributors to infer the rule under pressure.

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
