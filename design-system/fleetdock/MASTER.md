# Fleetdock dashboard: design system

This is the single source of truth for how the dashboard (`frontend/`) looks
and behaves. It was derived from the ui-ux-pro-max guidelines (accessibility,
interaction, layout, typography and colour rules), using Fleetdock's own teal
brand instead of the generic palette. Pages may add a
`pages/<page>.md` override; when one exists, its rules win over this file.

## Principles

1. **Calm, dense, legible.** This is a tool people use all day: neutral
   surfaces, one accent colour, information density over decoration.
2. **One way to do each thing.** Use the shared primitives in
   `src/components/ui`; don't hand-write class names or inline styles for
   something a primitive already does.
3. **Every state is designed.** Every view handles loading (skeleton), empty
   (an explanation plus the next step), error (what went wrong plus retry) and
   success.
4. **Keyboard first.** Every control can be reached with Tab, has a visible
   focus ring and an accessible name. Tabs and menus use arrow keys.

## Tokens (`src/styles/tokens.css`)

| Role | CSS variable | Tailwind utility |
| --- | --- | --- |
| Page background | `--bg` | `bg-bg` |
| Card / panel | `--panel` | `bg-panel` |
| Subtle fill (hover, wells) | `--panel-2`, `--panel-3` | `bg-panel-2` |
| Text | `--fg` | `text-fg` |
| Secondary text (≥4.5:1) | `--muted` | `text-muted` |
| Decorative text only | `--fg-subtle` | `text-subtle` |
| Lines | `--border`, `--border-strong` | `border-border`, `border-border-strong` |
| Brand (buttons) | `--primary`, `--primary-fg` | `bg-primary`, `text-primary-fg` |
| Accent (links, active, focus) | `--accent`, `--accent-soft` | `text-accent` |
| Status | `--success`, `--warning`, `--danger`, `--info` (+ `-soft`) | `text-success` … |
| Charts | `--chart-1` … `--chart-5` (validated light and dark) | — |
| Fonts | Geist / Geist Mono | `font-sans`, `font-mono`, class `.mono` |
| Radius | `--radius-sm` 6, `--radius-md` 8, `--radius` 10, `--radius-xl` 14 | `rounded-sm/md/lg/xl` |
| Elevation | `--shadow-xs/sm/md/lg` | `shadow-xs/sm/md/lg` |

Type scale: 11 (`--fs-2xs`) · 12 (`text-xs`) · 13 (`--fs-sm`) · 14 (`text-sm`,
body) · 16 (section title) · 20 (page title) · 24 (stat value). Don't introduce
other sizes.

**Never** put a hex colour or `rgba()` in a component. If a colour is missing,
add a token.

Dark mode is the `.dark` class on `<html>`. Use `dark:` utilities, which follow
the class, or better, tokens, which switch automatically.

## Primitives (`import { … } from "@/components/ui"`)

| Need | Use |
| --- | --- |
| Action button | `<Button variant="primary\|secondary\|ghost\|danger\|danger-solid" size="sm\|md\|lg" icon={<Plus size={15}/>} loading={busy}>` (defaults to `type="button"`; pass `type="submit"` in forms) |
| Link that looks like a button | `<ButtonLink href variant …>` |
| Icon-only button | `<IconButton label="Delete" onClick>` (the label is required; it also becomes the tooltip) |
| Text field / select / textarea | `<Input>`, `<Select>`, `<Textarea>` (with `size="sm"` for toolbars), always inside `<Field label hint error>` |
| Search box | `<SearchInput icon={<Search size={15}/>} placeholder aria-label>` |
| Checkbox / toggle | `<Checkbox label hint>`, `<Switch label checked onCheckedChange>` |
| Card | `<Card>`, `<CardHeader title icon actions description>`, `<CardBody>`, `<CardFooter>`; or `className="card panel"` for a padded block |
| Status pill | `<StatusBadge status>` for entity states; `<Badge tone="success\|warning\|danger\|info\|accent\|neutral" dot>` for anything else |
| Inline message | `<Callout tone="info\|warning\|danger\|success" title actions>` |
| Page top | `<PageHeader title description actions breadcrumbs badges>`, once per page |
| Section top | `<SectionHeader title icon description actions>` |
| Label/value pairs | `<KeyValueList items>`, or `<Detail label value mono>` for a single row |
| Summary numbers | `<div className="stat-strip">` of `<StatTile label value hint hintTone href icon>` |
| View switcher in a page | `<Tabs tabs value onChange label panelId>`, or `<QueryTabs>` to keep it in the URL; `<TabNav>` for route tabs |
| One-of-n filter / range | `<SegmentedControl options value onChange label>` |
| Table | `<DataTable>` (`components/data-table.tsx`) for lists with search and pagination; otherwise `<Table>` (card + scroll wrapper) with `className="num"` on numeric cells |
| Filters above a list | `<Toolbar>` |
| Loading | `<PageSkeleton>`, `<DetailSkeleton>`, `<TableSkeleton>`, `<StatStripSkeleton>`, `<Skeleton>`; `<Spinner>` only inside buttons or tiny inline spots |
| Failed query | `<QueryError error onRetry what backHref>` or `<QueryState query loading what>` (it tells "not found" apart from outages) |
| Empty | `<EmptyState icon title hint action>` (`plain` inside a card) |
| Dialog | `<Modal title wide>` with actions in `<div className="modal-actions">` (Cancel first, primary last) |
| Confirm destructive | `useConfirm()` / `<ConfirmModal danger confirmText>` |
| Tooltip / help | `<Tooltip content>`, `<HelpText>` |
| Copy | `<CopyButton value>` |
| Charts | `<MetricChart title points unit max color="var(--chart-n)">` in `<div className="chart-grid-layout">` |

Layout helpers (CSS classes): `stack` (column, 1rem gap), `stack-sm`, `row`
(wrapping row, .5rem gap), `toolbar`, `toolbar-spacer`, `section`,
`detail-grid` / `detail-main` / `detail-aside`, `form-grid`, `form-actions`,
`grid-cards`. For one-off spacing use Tailwind utilities (`mt-4`, `gap-2`,
`p-4`) and not `style={{…}}`.

## Rules

- **No inline `style={{}}`** except for genuinely dynamic values: computed
  widths, positions, progress percentages, and grid column templates that
  come from data.
- **No hand-written button class strings** (`className="btn btn-primary"`)
  in pages. Use `Button`, `ButtonLink` or `IconButton`. `Menu` and the existing
  primitives handle their own internals.
- **Headings:** one `h1` per page (through `PageHeader`), `h2` for sections,
  `h3` inside cards. Use sentence case.
- **Numbers** in tables and stats are tabular (`num` class) and right-aligned
  in tables.
- **Colour is never the only signal:** status always has a text label, and
  charts and badges carry labels or icons too.
- **Touch targets** are at least 44px on coarse pointers (handled by the CSS
  for `.btn` and `.input`).
- **Motion:** 120–180ms with `--ease`; `prefers-reduced-motion` is respected
  globally.
- **Copy:** buttons are verb-first ("Add server", "Run query"). Empty states
  say why the view is empty and offer the next step. Errors say what happened
  and what to do; use `friendlyError(err)`.
- **Forms:** visible labels (no placeholder-only fields), the error next to
  the field (`Field error={fieldError(err, "name")}`), the primary action last
  in `form-actions`.

## Pre-delivery checklist (per page)

- [ ] No `style={{` left, except dynamic values
- [ ] No `className="btn` strings in the page
- [ ] Loading state is a skeleton shaped like the content
- [ ] Empty state has a next step
- [ ] Errors use `QueryError` or `Callout` and offer Retry where it makes sense
- [ ] Works at 375px (no horizontal page scroll) and 1440px
- [ ] Light and dark both checked
- [ ] Keyboard: Tab order is logical, focus is visible, Esc closes overlays
