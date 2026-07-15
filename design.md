# Dashboard Design Reference

## Design Summary

The UI is a clean analytics dashboard with a soft light-gray canvas, white cards, thin neutral borders, rounded corners, and a single strong accent used for high-attention actions and data emphasis.

The visual style is:

- Minimal
- Professional
- SaaS admin dashboard
- High information density with generous whitespace
- Neutral-first, accent-second

## Layout

### Shell

- Left sidebar navigation
- Top utility bar with search, notifications, date controls, and export action
- Main content area with:
  - Welcome heading
  - KPI cards row
  - Primary analytics section
  - Secondary chart card
  - Recent transactions table

### Sidebar

- Width: approximately `240px`
- Background: warm white
- Sections separated by subtle dividers
- Nav items are text-first with light icon support
- Active state uses a soft filled background rather than a loud border

### Main Grid

- Outer page padding: `24px`
- Card gap: `16px`
- KPI cards: 4-up row
- Analytics area: 2-column split
  - Main sales chart: about `2fr`
  - Revenue breakdown card: about `1fr`

## Visual Tokens

### Core colors

```css
:root {
  --bg-canvas: #f4f2ee;
  --bg-surface: #ffffff;
  --bg-surface-muted: #f8f7f4;

  --border-subtle: #ece7df;
  --border-strong: #d8d1c7;

  --text-primary: #1f1f1c;
  --text-secondary: #6e6a63;
  --text-tertiary: #a29b91;

  --success: #1ea672;
  --warning: #e2b94b;
  --danger: #d85b52;

  /* Single swap point for branded emphasis */
  --accent: #1f1f1c;
  --accent-contrast: #ffffff;
  --accent-soft: color-mix(in srgb, var(--accent) 10%, white);
  --accent-muted: color-mix(in srgb, var(--accent) 35%, white);
}
```

### Accent-color rule

The screenshot uses near-black for:

- Primary action button
- Dark chart bars
- Strong data emphasis

Those should all be tied to one shared token:

```css
--accent
```

If you want to reskin the dashboard, changing only this value should update:

- Primary buttons
- Active chart series / dark bars
- High-emphasis stat marks
- Optional selected states and strong icons

Example theme swap:

```css
:root[data-theme="blue"] {
  --accent: #2563eb;
}

:root[data-theme="green"] {
  --accent: #15803d;
}

:root[data-theme="orange"] {
  --accent: #ea580c;
}
```

## Component Spec

### Page background

- Use `--bg-canvas`
- Keep the page slightly warmer than pure white

### Cards

- Background: `--bg-surface`
- Border: `1px solid var(--border-subtle)`
- Radius: `16px`
- Shadow: none or extremely subtle
- Padding:
  - KPI cards: `16px`
  - Larger cards: `20px` to `24px`

### Typography

- Font family: clean neo-grotesk or modern sans
  - Good choices: `Inter`, `Manrope`, `Plus Jakarta Sans`
- Heading weight: `600`
- Body weight: `400` to `500`
- Small labels: `500`

Suggested scale:

- Page heading: `40px`
- KPI value: `18px` to `20px`
- Card title: `12px` to `13px`
- Body text: `14px`
- Tiny labels: `11px` to `12px`

### Primary button

- Background: `var(--accent)`
- Text: `var(--accent-contrast)`
- Radius: `12px`
- Height: `40px`
- Horizontal padding: `16px`
- Border: `none`

```css
.btn-primary {
  background: var(--accent);
  color: var(--accent-contrast);
}
```

### Secondary controls

- Background: `--bg-surface`
- Border: `1px solid var(--border-subtle)`
- Text: `--text-primary`
- Radius: `12px`

### KPI cards

- Small uppercase or compact labels
- Large numeric value
- Tiny sparkline or vertical bar motif on the right
- Positive delta in `--success`

### Charts

#### Main sales chart

- Base bars: light neutral gray
- Highlighted bars or active series: `var(--accent)`
- Grid lines: very light neutral
- Tooltip card: white surface with subtle border

#### Revenue breakdown chart

- Secondary bars: very light neutral
- Emphasis bars: `var(--accent)`

Use this pattern:

```css
.chart-bar {
  background: var(--accent-muted);
}

.chart-bar.is-active,
.chart-bar.is-primary {
  background: var(--accent);
}
```

That keeps the primary dark bars and the button visually connected by one token.

### Table

- Header text: muted neutral
- Row separators: `var(--border-subtle)`
- Status pills:
  - Success: green tint
  - Pending: yellow tint
  - Refunded/inactive: neutral tint
- Row hover: `--bg-surface-muted`

## Spacing System

Use an 8px base scale:

- `4px`
- `8px`
- `12px`
- `16px`
- `20px`
- `24px`
- `32px`

Recommended usage:

- Icon gap: `8px`
- Button internal gap: `8px`
- Card padding: `16px` or `24px`
- Section spacing: `24px`

## Border Radius

Use one rounded system across the dashboard:

- Inputs: `12px`
- Buttons: `12px`
- Cards: `16px`
- Pills: `999px`

## Interaction Notes

- Hover states should be subtle, not glossy
- Avoid heavy shadows
- Keep motion fast and quiet
- Use accent color for intent, not for every decorative element

## Implementation Guidance

If this gets turned into code, use semantic tokens instead of hardcoding black in multiple places.

Recommended minimum token set:

```css
:root {
  --accent: #1f1f1c;
  --accent-contrast: #ffffff;
  --accent-soft: color-mix(in srgb, var(--accent) 10%, white);
  --chart-bar-primary: var(--accent);
  --chart-bar-secondary: var(--accent-soft);
  --button-primary-bg: var(--accent);
  --button-primary-text: var(--accent-contrast);
}
```

This is the key requirement from the screenshot:

- The black export button should use `--accent`
- The dark chart bars should use `--chart-bar-primary`
- `--chart-bar-primary` should resolve to `var(--accent)`

That gives you one clean variable to swap for the whole dashboard personality.

## Short Build Note

If you want, the next step can be mapping this `design.md` directly into your actual app tokens in:

- `web/src/theme.ts`
- `web/src/index.css`
- `web/src/v2/styles/v2.css`
