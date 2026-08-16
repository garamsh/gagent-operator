# Next.js (App Router) — Architecture & Style Conventions

How a Next.js project is laid out and where each kind of code
belongs: routes, the server/client boundary, mutations, env, and
tests.

Checked against Next.js 16. A claim below that names no version holds
for it.

## Contents
- 0. Folder & file naming
- 1. Project layout (App Router, `src/`)
- 2. App Router file conventions
- 3. Server Component vs Server Action vs Route Handler
- 4. Server Components — pages and layouts
- 5. Server Actions
- 6. Route Handlers — external use only
- 7. Env (single typed loader at project root)
- 8. Multi-layout via route groups
- 9. Parallel routes and intercepts (modal pattern)
- 10. `proxy.ts` (v16+) / `middleware.ts`
- 11. Testing — three layers, two locations
- 12. Import direction and file naming

## 0. Folder & file naming

Names describe what they own. The names the official tooling ships
are the canonical ones: if a name appears in the Next.js docs or in
a `create-next-app` default, adopt it; if it does not, name the file
or folder after what it owns.

- Official, and kept as shipped: `app/` (routes), `public/` (static
  assets), `src/` — optional in Next.js, required here (§1) — and
  the App Router file conventions
  (`layout.tsx`, `page.tsx`, `loading.tsx`, `error.tsx`, `route.ts`,
  `template.tsx`, `default.tsx`, `proxy.ts` / `middleware.ts`).
- `src/lib/utils.ts`, the shadcn-ui companion file, is allowed. It is
  the conventional home for `cn()` plus a few formatting and type
  helpers, and it stays small.
- Banned, because Next.js ships none of them: `utils/`, `helpers/`,
  `common/`, `misc/`, `shared/`, `stuff/`.
- Two helpers sharing an idea → name the idea:
  `src/lib/format-currency.ts`, `src/lib/email-validation.ts`.

## 1. Project layout (App Router, `src/`)

Use the `src/` layout. `src/app/` holds routes and nothing else;
every other folder under `src/` holds code by its responsibility,
named for what it does.

At the project root:

- `src/` — all application source. Its contents are listed below.
- `public/` — static assets, at the root, never inside `src/`.
- `e2e/` — Playwright E2E specs, at the root, never inside `src/`.
- `tsconfig.json` — maps the `@/*` path alias to `./src/*`.
- `next.config.ts`, `package.json`, `eslint.config.mjs`,
  `playwright.config.ts`, `vitest.config.ts` — project config.
- `.env.example` and `.env.local` (gitignored) — env files stay
  here, at the root (§7).

Under `src/`:

- `src/proxy.ts` — the v16 request proxy (§10).
- `src/app/` — routes and the App Router file conventions of §2,
  and nothing else.
- `src/app/api/` — Route Handlers, for external callers only (§6).
- `src/app/<segment>/_components/` — components belonging to one
  route, kept beside it; the `_` prefix opts the folder out of
  routing.
- `src/features/<name>/` — a vertical slice, owning its own
  `api/`, `components/`, `hooks/`, `types/` — only the ones the
  feature needs, not a fixed set per feature.
- `src/components/` — shared UI; `src/components/ui/` is the
  shadcn primitives home.
- `src/hooks/` — shared React hooks.
- `src/lib/<vendor>.ts` — pre-configured third-party clients and
  small shared helpers: `auth.ts`, `db.ts`, `api-client.ts`,
  `utils.ts` (the shadcn companion), `rate-limit.ts`, …. A
  vendor that has outgrown one file gets `src/lib/<vendor>/`.
- `src/env.ts` — the typed env loader (§7).
- `src/testing/` — test infrastructure: render helper, MSW
  handlers, MSW server (§11).
- `src/types/` — types used by two or more features.
- `src/styles/globals.css` — global styles.
- `src/instrumentation.ts` — optional OpenTelemetry hook.

A cross-cutting third-party client belongs at `src/lib/<vendor>.ts`,
or at `src/lib/<vendor>/` once the adapter outgrows one file.

## 2. App Router file conventions

A route is **not publicly accessible** until a `page.tsx` or
`route.ts` exists in the segment.

Never mix the App Router and the Pages Router in one project: no
`pages/` directory beside `app/`. A project runs one router.

| File | Role | Key constraint |
|---|---|---|
| `layout.tsx` | Shared UI for segment + descendants. State preserved. | Root **must** render `<html>` and `<body>`. |
| `page.tsx` | Route UI; leaf of subtree. Async by default. | Required for segment to be public. |
| `loading.tsx` | Suspense fallback. | Cannot show when layout reads uncached data — wrap in `<Suspense>` or move to `page`. |
| `error.tsx` | Error boundary. | **Must be a Client Component** (`'use client'`). |
| `global-error.tsx` | Replaces root layout on catastrophic error. | Must include own `<html>` and `<body>`. |
| `not-found.tsx` | UI for `notFound()` and unmatched URLs. | Streamed → 200, non-streamed → 404. |
| `route.ts` | HTTP request handler. Exports `GET`/`POST`/`PUT`/`PATCH`/`DELETE`/`HEAD`/`OPTIONS`. | Web `Request`/`Response` only. No JSX. |
| `template.tsx` | Like layout but re-mounts on navigation. | Use sparingly. |
| `default.tsx` | Fallback for unmatched parallel route slots. | Required when using parallel routes. |

### Folder conventions

| Syntax | URL effect | Use |
|---|---|---|
| `folder/` | adds URL segment | Standard route |
| `[param]/` | dynamic | `params.param` |
| `[...rest]/` | catch-all | `params.rest: string[]` |
| `[[...rest]]/` | optional catch-all | `params.rest?: string[]` |
| `(group)/` | **no** URL effect | Organization, multiple root layouts |
| `_folder/` | **no** URL effect | Private — opted out of routing |
| `@slot/` | **no** URL effect | Named slot for parallel routes |
| `(.)folder`, `(..)folder`, … | intercepts route | Modals |

Route group rules: routes in different groups at the same segment
**must not** resolve to the same URL. Navigating across **multiple
root layouts** causes a full reload. `/` must live in one group when
using multiple root layouts.

## 3. Server Component vs Server Action vs Route Handler

| Concern | Server Component | Server Action | Route Handler |
|---|---|---|---|
| Read data | yes, async render | — | yes, in handler |
| Mutations from forms | — | yes, **default** | no (§6) |
| Mutations from `onClick` | — | yes, via `useTransition` | no (§6) |
| Webhooks from third parties | — | no | **only choice** |
| Public REST/JSON API | — | no | **only choice** |
| OAuth callbacks | — | no | yes |
| Streaming (ReadableStream) | partial, via Suspense | — | yes |
| Works without JS | yes | yes | no |

### Decision rule

1. **External HTTP API / webhook** → `src/app/api/<path>/route.ts`
2. **Form submission / mutation from your own UI** → Server Action
3. **`onClick` mutation with optimistic UI** → Server Action via
   `useTransition` / `useActionState`
4. **Read data for a page** → Server Component (async, fetch inline)
5. **React to a mutation** → Server Action calls `revalidatePath` /
   `revalidateTag`, `redirect()`, or `router.refresh()`

### `'use client'` placement

Add to **specific interactive components**, not layouts or whole
pages. Once a file is `'use client'`, all its imports become
client-bundled. Use `import 'server-only'` for modules that must
never reach the client. Non-`NEXT_PUBLIC_` env vars are stripped to
`""` in client bundles.

## 4. Server Components — pages and layouts

- Pages are `async` Server Components that fetch their data inline:
  no `'use client'`, no `useEffect`.
- `params` and `searchParams` are **promises** in v15+ — `await`
  them.
- Layouts **do not re-render** on navigation, so a `searchParams`
  value read there is unreliable. Read live values in a Client
  Component.
- A layout reading uncached data (`cookies()`, `headers()`, an
  uncached `fetch`) **blocks** `loading.tsx`. Wrap the fetch in
  `<Suspense>` or move it to `page.tsx`.

## 5. Server Actions

A Server Action carries the `'use server'` directive at the top of
its module, and is the default for mutations driven by your own UI
(§3). Its `FormData` argument reaches it over the network, like any
other request body.

**Critical security:** Server Functions POST to the route where
they're used. A `proxy.ts` matcher that excludes that path silently
disables auth on its Server Actions. **Always re-verify the session
inside the Server Action.**

## 6. Route Handlers — external use only

- Route Handlers serve external callers: webhooks, OAuth callbacks,
  public REST/JSON endpoints, streamed responses. **Never** use one
  for a mutation your own UI submits — that is a Server Action (§3).
- A webhook handler verifies the provider's signature against the
  raw request body before acting on the event, and answers 400 when
  the signature is missing or fails. A body already parsed as JSON
  can no longer be verified.

## 7. Env (single typed loader at project root)

- One typed, validated loader, at `src/env.ts`. Everything else does
  `import { env } from '@/env'`. **Never** scatter `process.env.X`
  reads throughout the code.
- Env files stay at the project root. Load order, first match wins:
  `process.env` → `.env.$(NODE_ENV).local` → `.env.local` (not in
  test) → `.env.$(NODE_ENV)` → `.env`.

### Browser exposure

- Without `NEXT_PUBLIC_` prefix → server only.
- With `NEXT_PUBLIC_` prefix → inlined into the client bundle at
  build time; **frozen at build** — changing it after `next build`
  has no effect.
- Dynamic lookups (`process.env[varName]`) are NOT inlined.

## 8. Multi-layout via route groups

- Each route group owns its layout: `(marketing)/layout.tsx` for the
  public section, `(app)/layout.tsx` for the authenticated one,
  `(auth)/layout.tsx` for the login and registration flows. The root
  `src/app/layout.tsx` renders `<html>`/`<body>`, fonts, and
  providers.
- **Keep multiple root layouts rare** — navigation between them is a
  full reload.

## 9. Parallel routes and intercepts (modal pattern)

For modals that stay shareable, refresh-safe, and clean under
back/forward. The pieces, for a photo modal:

- `src/app/@modal/default.tsx` — fallback for the unmatched slot.
- `src/app/@modal/(..)photos/[id]/page.tsx` — the intercepted route,
  rendered as the modal.
- `src/app/photos/[id]/page.tsx` — the real page, rendered on direct
  navigation.
- `src/app/layout.tsx` — renders both `{children}` and `{modal}`.

## 10. `proxy.ts` (v16+) / `middleware.ts`

- The file lives at `src/proxy.ts` (§1). It exports a `proxy`
  function and a
  `config` object whose `matcher` selects the paths it runs on —
  conventionally everything except `/api`, `_next/static`,
  `_next/image`, and `favicon.ico`.
- v16 renamed `middleware.ts` → `proxy.ts`. Codemod:
  `npx @next/codemod@canary middleware-to-proxy .`.
- Use cases: auth gate (redirect), geolocation, A/B headers, CORS
  preflight for `/api/*`.
- **Anti-patterns:** don't use as a general middleware hub; don't
  rely on it for Server Action security — matchers can exclude
  paths silently (§5).

## 11. Testing — three layers, two locations

| Layer | Tool | Location |
|---|---|---|
| Unit | Vitest + Testing Library | `src/**/__tests__/*.test.ts` |
| Integration | Vitest + Testing Library + MSW | `src/features/<f>/__tests__/*.test.ts` or `src/app/<r>/__tests__/*.test.ts` |
| E2E | Playwright | `e2e/*.spec.ts` |

- Test names: `describe(...) > it(...)`.
- The in-process client is Testing Library's `render()` for a
  component and route segment fetching for a whole route.
- MSW handlers live at `src/testing/mocks/handlers.ts`.
- The render helper lives at `src/testing/render.tsx` and exports
  `renderWithProviders`, wrapping Testing Library's `render()` with
  the providers the app mounts.
- This stack's substitutes: MSW for outbound HTTP, a Prisma/Drizzle
  test double or SQLite for the database, a fake session for auth,
  `testcontainers-node` for real service dependencies.
- The built app is `next start`.
- Assert on rendered text, ARIA, emitted form payloads, and
  navigated URLs — **never** on the internal React tree shape or
  component identity.


## 12. Import direction and file naming

The three tiers, and what each may import:

- `components/`, `hooks/`, `lib/`, `types/`: cross-feature.
  Each may import from itself or each other.
- `features/*`: may import the cross-feature modules; **may not**
  import from `app/` or from another feature.
- `app/`: may import from both.

The ESLint rule `import/no-restricted-paths` enforces this.

### File naming

- Component: `<name>.tsx` (multi-word: `theme-toggle.tsx`).
- Hook: `use-<name>.ts`.
- Server Action: `<verb>-<noun>.ts` with `'use server'`.
- Test: `*.test.ts(x)` inside `__tests__/`.

