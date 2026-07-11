# HostForge Design System

## Direction

HostForge is a self-hosted deploy control plane — the audience is developers running their own infrastructure. The target feel: **as precise as Vercel, but warm instead of monochrome** — a workshop, not a boardroom. Confident, not clinical.

The name already gives us a real material to design from: a forge. Not literally (no flame icons, no "blacksmith" cosplay), but as a restrained source of color and motion — molten orange for the accent, tempered steel for informational states, patina green for success, a slow ember glow reserved for things that are genuinely *live*.

**Non-negotiables from this brief:**
- Both light and dark are first-class — build every token in pairs, ship both polished.
- One accent (orange), refined, not swapped.
- Warmth over starkness. Soft edges over sharp. Sentence case over SHOUTING CAPS.
- Motion is rare and meaningful, never decorative.

---

## 1. Color

### Neutrals

| Token | Light | Dark | Use |
|---|---|---|---|
| `canvas` | `#FAF8F4` | `#17130F` | Page background |
| `surface` | `#FFFFFF` | `#221D17` | Cards, panels, sidebar |
| `surface-raised` | `#FFFFFF` + 1px border | `#2A241D` | Hover/elevated cards |
| `border` | `#E8E2D8` | `#362F26` | Hairlines, card edges |
| `ink` | `#211C16` | `#F5F0E8` | Primary text |
| `ink-muted` | `#726B5E` | `#A89C8B` | Secondary text, labels |
| `ink-faint` | `#A39C8F` | `#6B6255` | Placeholder, disabled |

Both are warm neutrals (not blue-grays) — this is what keeps it from reading as another Vercel clone. Dark mode is a warm graphite, not near-black.

### Accent — Ember

| Token | Value | Use |
|---|---|---|
| `ember-500` | `#E85D2C` | Primary buttons, active nav, links |
| `ember-400` | `#F2703D` | Same role on dark surfaces (needs the lift for contrast) |
| `ember-100` | `#FBE4D8` (light) / `#3A2418` (dark) | Tinted backgrounds — active nav item, subtle highlights |

This is one notch more saturated and red-shifted than the current orange — closer to hot metal than clay. Refined, not replaced.

### Semantic status

| State | Light | Dark | Metaphor |
|---|---|---|---|
| Success | `#3E9467` | `#4FB37D` | Patina — cooled, settled |
| Failed | `#D7402A` | `#F0543A` | Scorch |
| Info / running | `#3E77A6` | `#5C93C4` | Tempered steel |
| Warning | `#C77F17` | `#E8A23A` | Spark |

**Rule:** status colors always pair a soft tint background (10–12% opacity) with the full color for text/dot — never a bold outlined pill on white like today's UI. Softer = calmer, still scannable.

**Rule:** Ember is the *only* saturated color used for UI chrome (buttons, active states). Status colors are reserved strictly for status. This keeps the palette feeling considered instead of a rainbow of badges.

---

## 2. Typography

| Role | Family | Weight | Where |
|---|---|---|---|
| Display | Space Grotesk | Medium/Semibold | Page titles, KPI numbers |
| Body / UI | Inter | Regular/Medium | Everything else — nav, buttons, tables, copy |
| Data / mono | JetBrains Mono | Medium | Metrics, commit hashes, log lines, small data labels |

Space Grotesk over Inter-for-everything is the one deliberate swap — it has just enough personality in the numerals and geometry to make headers feel considered, while staying technical. Inter carries the actual density of a dashboard because it disappears at small sizes. Mono is used **sparingly** now — reserved for genuinely tabular/technical data (commit SHAs, byte counts, durations), not sprayed across every label like the current UI does.

### Scale

| Style | Size/Line | Weight | Case |
|---|---|---|---|
| KPI number | 32/38 Space Grotesk | Medium | — |
| Page title | 22/28 Space Grotesk | Medium | Sentence case |
| Section header | 15/22 Space Grotesk | Medium | Sentence case |
| Body | 14/20 Inter | Regular | Sentence case |
| Small / caption | 13/18 Inter | Regular | Sentence case |
| Data label (eyebrow) | 11/16 JetBrains Mono | Medium, tracked +0.04em | UPPERCASE — the *only* place caps survive |

Today almost every label is uppercase mono ("ACTIVE PROJECTS," "OVERVIEW," "HOST"). Dial that back to the rare small eyebrow only — sentence case everywhere else immediately makes the UI feel less like a spreadsheet and more like a product.

---

## 3. Layout & Spacing

- **Grid unit:** 8px base (4px allowed for tight icon/text pairing only).
- **Sidebar:** 240px fixed, collapsible to 64px icon rail.
- **Card radius:** `10px`. Sharp 0-radius (current) reads cold/hard-to-deal-with; full pill rounding reads playful/consumer. 10px is the "welcoming but precise" middle.
- **Card border:** 1px `border` token, no drop shadows at rest. Shadow only appears on genuine elevation (dropdowns, modals, hover-lift on interactive cards).
- **KPI row:** `auto-fit, minmax(220px, 1fr)` — same responsive behavior as today, just re-skinned.
- **Density:** keep the current information density (it's a good instinct for an ops dashboard) but add ~4px more internal card padding and ~8px more row height in tables — the current layout feels slightly clenched.

---

## 4. Components

**Buttons**
- Primary: `ember-500` fill, white text, `10px` radius, no shadow. Hover: `ember-400`, no scale/bounce.
- Secondary: `surface` fill, 1px `border`, `ink` text.
- Ghost: text-only, `ink-muted`, hover → `ink`.

**Status pills**
- Tint background (status color at 10%) + dot + label, sentence case, `6px` radius (smaller than cards — pills stay compact). Drop the bold 1px outline style currently used.

**KPI tiles**
- Eyebrow (mono, uppercase, `ink-muted`) → big Space Grotesk number → one-line caption. Identical structure to today, just retyped. The "live" tiles (containers running, active deploys > 0) get the ember-glow treatment below.

**Sidebar nav**
- Active item: `ember-100` tint background + `2px` `ember-500` left border, not a flat gray block like now. Icons 1.5px stroke, no fills except the active dot/badge.

**Tables (Recent Activity, etc.)**
- Hairline row dividers only, no zebra striping. Row hover = subtle `surface-raised` tint. Status column uses the pill above.

**Empty states**
- Direct, plain-verb copy that tells the user what to do next, not a mood. E.g. *"No deployments yet. Connect a repo to trigger your first build."* — not "It's quiet here 👀."

**Charts / sparklines**
- Replace flat single-color strokes with a gradient stroke (full color → transparent) matching CPU=ember, Memory=info, Disk=warning, Network=success — same semantic mapping as today's colors, just with the gradient treatment for a bit of life without adding chart junk.

---

## 5. Motion — the one signature move

Everything above is disciplined and quiet on purpose. The one place HostForge gets to have a pulse: **live state gets a literal, restrained glow.**

- A KPI tile or status dot representing something *actually running right now* (containers running, an in-progress deploy) gets a slow (2.5s, ease-in-out) breathing radial glow in `ember-100`/`ember-500` behind it — like metal still warm from the forge.
- This is reserved **only** for genuine live/in-progress state. A static "0 containers running" or a completed deploy never gets it. If it's not alive, it doesn't glow — that's what keeps it meaningful instead of decorative.
- All other motion: 150ms ease-out on hover/press, nothing else. No page-load choreography, no bouncing.
- Respect `prefers-reduced-motion`: glow becomes a static tint, no animated pulse.

---

## 6. Voice & copy

- Active voice, name what the user controls: "Deploy" not "Trigger deployment pipeline."
- A button's label matches its resulting toast: "Deploy" → *"Deployed."*
- Errors state what happened and how to fix it, no apology, no vagueness: *"Build failed — missing `DATABASE_URL`. Add it in Environment Variables and redeploy."*
- Empty states are an invitation to act, not a mood — see example above.

---

## 7. Before → after sketch (Fleet Status)

```
BEFORE (current)                       AFTER
┌─────────────────────────┐            ┌─────────────────────────┐
│ OVERVIEW                │            │ Overview                │
│ Fleet status             │            │ Fleet status             │
│                          │            │                          │
│ [ACTIVE PROJECTS]  [DEPLOYS]  ...     │ [Active projects] [Deploys] ... │
│  2 (flat)          6 (flat)           │  2          6 (ember glow, live)│
│                                        │                          │
│ HOST ─────────────────── SYSTEM →     │ Host ─────────────────── System →│
│ flat single-color sparklines          │ gradient sparklines     │
│                                        │                          │
│ RECENT ACTIVITY                       │ Recent activity          │
│ outlined bold pills SUCCESS/FAILED    │ soft tinted pills        │
└─────────────────────────┘            └─────────────────────────┘
```

Structurally almost identical — this isn't a rebuild, it's a retype: sentence case, softer status pills, gradient charts, and glow reserved for what's actually live.

---

## 8. Open questions for next pass

- Icon set: Lucide vs Phosphor (1.5px stroke either way) — any existing preference?
- Should the ember glow also apply to the sidebar's live deploy indicator, or stay confined to the Overview KPIs for now?
- Logo/wordmark — worth a quick pass once this system is locked, since "HostForge" currently just renders as text + a generic spark icon.
