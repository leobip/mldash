# mldash — design system

Decisions taken for the dashboard UI (`web/templates/index.html`). Apply these
instead of re-deciding: they are settled.

## Direction

Control-room terminal for scarce hardware. One dark canvas, tabular numbers
everywhere, color reserved for meaning (capacity state), infrastructure noise
pruned to the part a human reads. Not a SaaS dashboard: no white surfaces, no
decorative gradients, one accent only.

## Tokens (all AA on `--canvas`)

| Token | Value | Contrast | Use |
| --- | --- | --- | --- |
| `--canvas` | `#0f1117` | — | page background |
| `--surface` | `#1a1f2e` | — | cards, row hover, total row |
| `--line` | `#2a3040` | — | section rules, card borders |
| `--line-soft` | `rgba(255,255,255,0.06)` | ~1.3:1 | table row separators (quiet, neutral) |
| `--ink` | `#e6e9f0` | 15.5:1 | values, row names, `.fver` |
| `--ink-2` | `#a8b2c7` | 8.9:1 | supporting: `h2` labels, `.stopped`, `.val-used` |
| `--ink-3` | `#8892a6` | 6.0:1 | meta: `th`, `.legend`, `.footer`, `.none`, `.muted` |
| `--accent` | `#7cb8ff` | 9.1:1 | identity only: `h1`, `.card-type` |
| `--free` | `#6bcb77` | 9.4:1 | capacity available (`.val-free`, `.running`) |
| `--depleted` | `#ff6b6b` | 6.8:1 | **only** when `available == 0` (`.val-depleted`) |
| `--waiting` | `#ffd93d` | 13.7:1 | pending / warning |

Never reintroduce raw grays (`#444`/`#555`/`#666`): they failed AA on this
canvas (1.9:1–3.3:1). Hierarchy is carried by case, tracking, size and weight —
not by dimming text below 4.5:1.

## Depth

Borders + surface shifts only. **No shadows anywhere.** Elevation ladder: canvas
→ `--surface` (cards, hover). Do not add a third surface level.

## Hierarchy

- Focal point: the free-capacity number in each GPU card (28px bold). `total`
  and `used` stay neutral (`--ink`, `--ink-2`) — red means *depleted*, never
  *in use*; a healthy busy cluster must not look like an alarm.
- `h1` 22px accent · `h2` 11px uppercase +1px tracking + `--line` rule ·
  table body 13px · `th`/labels/footer 11–12px uppercase or sentence, `--ink-3`.
- Footnotes (`.legend`) are always **smaller and lighter than the data they
  describe**. The old `.legend-strong` (16px bold) inverted this — do not
  resurrect it.

## Numbers

`font-variant-numeric: tabular-nums` on `table` and `.stat .val`. Any figure
that refreshes every 30s must not change column width.

## Spacing

Base unit 4px. Table cells `7px 12px`, cards `16px 20px`, card gap 16px,
`h2` margin `24px 0 12px`, body padding 24px, `max-width: 1500px` centered
(ultrawide must not stretch a table row to unreadable measure).

## Data display rules

- **Requests, not usage.** Resource columns are labelled `GPU (req)` /
  `CPU (req)` / `Mem (req)`; the Nodes table says total/taken/free and its
  legend states it is the scheduler view.
- **Prune infrastructure noise** in tables, keep the full string reachable in
  `title=` for copy-paste into `kubectl`:
  - image → `ImageRepo` (registry host dropped) + `:tag` in `.muted`
  - pod → `PodBase` (ReplicaSet hash dropped) + `· suffix` in `.muted`
  - Helpers live in `internal/api/format.go`, exposed as template funcs
    `imageRepo`, `imageTag`, `podBase`, `podSuffix`. JSON API always keeps the
    full values.
- **No dead columns.** Namespace Quotas renders Total/Available only when some
  namespace actually has a ResourceQuota (`QuotasAnyConfigured`); otherwise the
  section is "Namespace Usage" with used-only columns and an explanatory line.
  44 `NA` cells used to fill 57% of that table.
- **Name the unit inside the column, not only in the section.** A bare
  `Mem Used` shown in `Gi` was read as *storage* by a user whose notebooks
  request 161 Gi of RAM while holding 1155 Gi of workspace volumes. Memory
  columns say `RAM`; compute columns say `(running)` when only live notebooks
  are counted (`CPU Used (running)`, `Mem Used (RAM, running)`). The legend
  states the definition of "Used" as its **first** line — caveats go first,
  not last.
- Section order = operator value: GPU Inventory → Nodes → Notebooks → Workloads
  → Namespace Usage.

## Constraints

Zero JavaScript, server-rendered `html/template`, meta-refresh 30s. Any
improvement must hold that line until the HTMX item is taken on deliberately.

## Open (not yet decided)

- Free-capacity number could dominate its card more (total/used demoted a tier).
- Empty states for sections that currently disappear when they have no rows
  (`{{if .Nodes}}`, `{{if .HasWorkloads}}`) — "0 workloads" looks like "watcher
  broken".
- `Ready` in the Nodes table is emoji-only (color-only signal); Status has text.
- meta-refresh loses scroll position on a ~3200px page (needs HTMX / open item #9).
