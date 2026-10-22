# quiz-v2 — Documentation Index

This directory holds all project documentation: the design system, the
product spec and plans, the UI prototypes, and the official-library
reference.

## Layout

```
docs/
├── README.md                      # this file
├── DESIGN.md                      # THE design system — colour, type, shape, states, a11y, motion
├── template/                      # the three working UI prototypes
│   ├── DASHBOARD.html             # teacher workspace
│   ├── LOGIN.html                 # auth / join / register
│   └── QUIZ.html                  # student attempt flow
├── superpowers/
│   ├── specs/2026-10-20-quiz-system-design.md     # the product spec
│   └── plans/
│       ├── 2026-10-21-initial-framework.md        # Go/Fiber scaffold, config, store
│       └── 2026-10-21-frontend-ui.md              # templates, design contract, screens
├── GO-FIBER-DOCS.md               # URL catalog (sitemap) of docs.gofiber.io
├── JQUERY-DOCS.md                 # URL catalog (sitemap) of api.jquery.com
└── BOOTSTRAP-V6-DOCS.md           # URL catalog (sitemap) of getbootstrap.com 6.0
```

## Authority

| Area | Authority |
| :--- | :--- |
| Colour, typography, spacing, radius, elevation, layer order, component states, responsive behaviour, accessibility floors, motion | [`DESIGN.md`](./DESIGN.md) |
| Product behaviour, data model, and the UI/UX build contract | [`superpowers/specs/2026-10-20-quiz-system-design.md`](./superpowers/specs/2026-10-20-quiz-system-design.md) §15 |
| Screen structure and copy intent | [`template/`](./template/) — structure only, not markup or CSS |
| Execution order | [`plans/2026-10-21-initial-framework.md`](./superpowers/plans/2026-10-21-initial-framework.md), then [`plans/2026-10-21-frontend-ui.md`](./superpowers/plans/2026-10-21-frontend-ui.md) |

`DESIGN.md` is the single design-token contract. Where the spec and
`DESIGN.md` disagree, `DESIGN.md` wins.

## Quick links

- Design system: [`DESIGN.md`](./DESIGN.md)
- Product spec: [`superpowers/specs/2026-10-20-quiz-system-design.md`](./superpowers/specs/2026-10-20-quiz-system-design.md)
- Scaffold plan: [`superpowers/plans/2026-10-21-initial-framework.md`](./superpowers/plans/2026-10-21-initial-framework.md)
- Frontend plan: [`superpowers/plans/2026-10-21-frontend-ui.md`](./superpowers/plans/2026-10-21-frontend-ui.md)
- Bootstrap 6 docs index: [`BOOTSTRAP-V6-DOCS.md`](./BOOTSTRAP-V6-DOCS.md)
- Fiber docs index: [`GO-FIBER-DOCS.md`](./GO-FIBER-DOCS.md)
- jQuery docs index: [`JQUERY-DOCS.md`](./JQUERY-DOCS.md)

## Stack

Go monolith · Fiber v3 · SSR (`html/template`) · MariaDB 12.3 · Docker
Compose · realtime SSE + POST · Bootstrap 6.0.0-alpha.1 (vendored) · jQuery
4.0.0 (vendored) · Gotham Rounded (self-hosted).
