# design.md — Dashboard Visual Design

## Design philosophy
Developer-tool minimalism — closer to a terminal, GitHub's dark UI, or Linear's dashboard
than a marketing landing page. **No gradients. No glassmorphism. No purple/blue AI-SaaS
palette. No emoji in the UI.** Data density and legibility over decoration. If in doubt,
remove the element rather than style it.

## Color palette (monochrome + one signal color)
```
--bg:          #0d0d0d   (near-black, not pure black)
--surface:     #161616   (cards/table rows)
--border:      #2a2a2a   (hairline borders only, 1px)
--text-primary:   #e8e8e6
--text-secondary: #8a8a86
--accent:      #4ade80   (single green accent — used ONLY for "time saved" positive numbers)
--accent-dim:  #1a3324   (accent background tint, e.g. badge backgrounds)
--warn:        #e8a33d   (used only for cache-miss / full-run indicators, sparingly)
```
Light mode (optional stretch): invert to `#fafaf9` bg / `#111` text, same accent green,
same restraint.

## Typography
- Monospace throughout for data (`ui-monospace, "SF Mono", "JetBrains Mono", monospace`) —
  this is a developer tool, numbers and commit hashes should read as code.
- One sans-serif (`system-ui, -apple-system, sans-serif`) reserved only for the page title
  and section labels, to create a single clear hierarchy level.
- No more than 3 font sizes total: 13px (data/table), 15px (labels), 20px (page title).

## Layout
- Single column, max-width 960px, centered, generous 48px top margin. No sidebar, no nav —
  this is a single-purpose dashboard, not an app shell.
- Top: page title ("Diffr") + one-line subtitle in `--text-secondary`
  ("Test impact analysis — run history").
- Below: 3 stat cards in a row (Total Runs / Avg % Saved / Total Time Saved), plain
  bordered boxes, no icons, no shadows — just a 1px `--border` and padding.
- Below that: one simple line chart (cumulative ms saved over time) — hand-drawn with
  `<canvas>` or inline SVG, no charting library needed for ~20 data points.
- Below the chart: a plain `<table>` of recent runs — commit pair (short hash), changed
  function count, impacted/total tests, time saved %, cache hit/miss badge.
  Zebra-striped rows via `--surface` only, no hover glow effects.

## Components

### Stat card
```
┌─────────────────────────┐
│ TOTAL RUNS               │  ← 11px, uppercase, letter-spacing, --text-secondary
│ 14                        │  ← 28px, monospace, --text-primary
└─────────────────────────┘
```
Border: 1px solid `--border`. No border-radius beyond 4px (sharp, not bubbly).

### Cache badge (in table)
- Hit: small pill, `--accent-dim` background, `--accent` text, label "CACHED"
- Miss: small pill, transparent background, `--warn` 1px border, `--warn` text, label "FULL"

### Chart
- Single line, 1.5px stroke, `--accent` color, no fill/area gradient under the line.
- Axis labels in `--text-secondary`, 11px, minimal gridlines (2–3 horizontal only, very
  low opacity `--border`).

## Explicit anti-patterns to avoid
- No purple-to-blue or pink-to-orange gradient backgrounds anywhere.
- No large rounded "glass" cards with blur/shadow stacks.
- No hero section, no illustration, no "AI sparkle" iconography.
- No animated counters or confetti on load.
- No more than one accent color on screen at a time.

## Responsive behavior
Not a priority for v1 (this is a local dev-tool dashboard, desktop-only is acceptable) —
but stat cards should stack to a single column below 600px width as a minimum courtesy.
