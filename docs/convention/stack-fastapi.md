# FastAPI — Architecture & Style Conventions

> How a FastAPI project is laid out and where each kind of code
> belongs: bootstrap, configuration, database, schemas, services,
> routers, and tests.
>
> Checked against FastAPI 0.138, Pydantic 2, SQLAlchemy 2. A claim
> below that names no version holds for these.

## Contents
- 0. Folder & file naming
- 1. Project layout — domain-by-package (production)
- 2. Application bootstrap
- 3. Configuration
- 4. Database
- 5. Schemas (Pydantic v2)
- 6. Service layer (with optional repository)
- 7. Routers
- 8. Streaming (SSE / JSON Lines / bytes)
- 9. Dependencies
- 10. Tests
- 11. Migrations & errors

## 0. Folder & file naming

Names describe what they own. Adopt the names the canonical
zhanymkanov production layout uses:

- **Per-domain `utils.py`** is fine (single file at the root of
  a domain — non-business logic helpers, response normalization,
  data enrichment). Cross-cutting `utils.py` at the project root
  is also fine.
- **`config.py`** (domain-local config, `BaseSettings` subclass
  with `env_prefix="<DOMAIN>_"`), **`constants.py`**,
  **`exceptions.py`**, **`dependencies.py`**, **`router.py`**,
  **`schemas.py`**, **`models.py`**, **`service.py`** are the
  conventional file names in each domain folder. Adopt them.

Names that are still banned (vague, never canonical for FastAPI):

- `helpers.py` / `helpers/` (folder)
- `common.py` / `common/`
- `misc.py` / `misc/`
- `shared.py` / `shared/`

These were never the FastAPI canonical layout and are still
wrong for it. Two helpers sharing an idea → name the idea:
`src/auth/password_hashing.py`, `src/billing/format_currency.py`.

## 1. Project layout — domain-by-package (production)

URL versioning is a **URL prefix** on the `APIRouter` (§7) — never
a directory name like `api/v1/`. This is the zhanymkanov production
layout.

Paths outside the domain folders:

- `src/main.py` — the `FastAPI()` application: lifespan, CORS,
  and the `include_router` loop (§2).
- `src/config.py` — the global `Settings` (§3).
- `src/database.py` — async engine, sessionmaker, `get_db`,
  `Base` (§4).
- `src/dependencies.py` — cross-cutting dependencies, promoted
  from a domain when ≥ 3 resources share them (§9).
- `src/exceptions.py` — cross-cutting exceptions, mapped to HTTP
  by the handlers in `src/main.py` (§11).
- `src/pagination.py` — shared pagination helper (optional).
- `src/<domain>/` — one folder per bounded context; the table
  below lists the files it owns.
- `src/<external_service>/client.py` — one folder per external
  service (`src/s3/client.py`,
  `src/payment_provider/client.py`), holding its client.
- `alembic/` — the async migration environment (§11).
- Root files: `pyproject.toml` and `uv.lock` (§3),
  `alembic.ini`, `logging.ini`, `.env`, `.gitignore`,
  `docker-compose.yml` for local dev services (db, redis, etc.),
  `Dockerfile` for the production image.

Test paths are in §10.

For **small projects (≤3 domains, ≤5 tables)** where the
domain-by-package shape is overkill, the layer-based layout
(`src/api/`, `src/services/`, `src/repositories/`, `src/models/`,
`src/schemas/`, `src/core/`) is acceptable. Switch to
domain-by-package the moment either threshold is crossed.

For **microservices / very small services** the official
`fastapi/full-stack-fastapi-template` flat layout
(`backend/app/{main,models,crud,utils}.py` +
`backend/app/{api,core,alembic}/`) is the canonical starter. The
template uses `app/utils.py` for cross-cutting helpers — that
name is fine for that layout. Adopt the official template for
its intended use case (small / single-service projects), not for
multi-domain production code.

### File responsibilities inside `src/<domain>/`

| File | Owns |
|------|------|
| `router.py` | `APIRouter(...)`. Routes stay thin: parse via Pydantic, call `service.<method>`, return what it hands back under the `schemas.<Domain>Public` type (§7). |
| `schemas.py` | Pydantic v2 input/output. **Never** merged with ORM models. |
| `models.py` | SQLAlchemy 2.x ORM tables. One file per aggregate. |
| `service.py` | Business logic, transactions, cross-aggregate calls. |
| `repository.py` | (Optional) DB-only access. Add when `service.py` starts mixing I/O orchestration with raw query building. |
| `dependencies.py` | Domain-local FastAPI dependencies (`valid_<x>_id`, etc.). |
| `config.py` | `BaseSettings` subclass with `env_prefix="<DOMAIN>_"` — auth-specific env, billing-specific env, etc. |
| `constants.py` | `class <Domain>ErrorCode(StrEnum)`. Replaces magic strings. |
| `exceptions.py` | Domain exceptions, mapped to HTTP by global handlers in `main.py`. |
| `utils.py` | Non-business logic helpers (response normalization, data enrichment, etc.). |

Cross-domain imports use explicit aliases:
`from src.billing import service as billing_service`. **Never**
`from src.<domain> import *`.

## 2. Application bootstrap

`src/main.py` constructs the application and nothing else:

- Startup and shutdown work — opening the connection pool and
  closing it — runs in a `lifespan` `asynccontextmanager` handed
  to `FastAPI(...)`. `@app.on_event("startup")` /
  `@app.on_event("shutdown")` are deprecated as of 0.93.
- CORS is added with `CORSMiddleware`, its origins read from
  `Settings`.
- Routers are included in a loop, with no arguments at the call
  site — each `APIRouter` already carries its own `prefix` (§7),
  built from `settings.API_V1_STR`.

`API_V1_STR` lives in `Settings`; a v1 → v2 migration is a
one-line constant change.

### Serving a built SPA (optional)

For monorepos with a built frontend (Vite / Astro / Angular /
Svelte / Vue / etc.), serve the build directory with
`app.frontend("/", directory="dist")` — or `router.frontend(...)`
when it belongs behind a router prefix. These are low-priority
routes, so regular API routes match first and client-side
routing fallbacks fill the rest. Avoid `StaticFiles` for an SPA
mount when the frontend needs client-side routing.

## 3. Configuration

- The global `Settings(BaseSettings)` lives in `src/config.py`
  and reads `.env`.
- Reach it through an `@lru_cache`-decorated `get_settings()`,
  which lets tests use
  `app.dependency_overrides[get_settings] = ...`.
- **Avoid one mega-Settings class** — split into a global
  `Settings` and small per-domain `<Domain>Config(BaseSettings)`
  classes when warranted (see §1 for the per-domain
  `config.py`).

**Package manager:** `uv` (Astral) is the production standard.
`pyproject.toml` is the manifest; `uv.lock` is the lockfile — not
`requirements.txt`.

### Running the app

Use the official `fastapi` CLI rather than `uvicorn` /
`gunicorn` directly — it handles the import path resolution,
reload, and production mode for you. `fastapi dev` serves
localhost with reload; `fastapi run` is the
production-recommended path and defaults to `0.0.0.0:8000`.
Declare the entrypoint under `[tool.fastapi]` in
`pyproject.toml` so neither command needs an explicit path
argument.

`fastapi run` is single-process by default; front it with a
process manager (or `gunicorn` with
`uvicorn.workers.UvicornWorker`) only when horizontal scaling is
needed.

## 4. Database

- `src/database.py` owns the `DeclarativeBase` subclass, the
  async engine, the `async_sessionmaker`, and the `get_db`
  dependency that yields one `AsyncSession` per request.
- Per-domain models in `src/<domain>/models.py` with
  SQLAlchemy 2.x typed `Mapped[T]` annotations.
- **Async-first.** Sync DB calls inside `async def` are a
  classic deadlock source — pick the async API. The Postgres
  driver is `asyncpg` (used by the `postgresql+asyncpg://` URL).
- Query with `select()` / `insert()` / `update()` / `delete()`
  executed through `Session.execute()` — never `Session.query()`.
- Naming: `lower_case_snake`, **singular** tables. Group with
  prefix (`payment_account`). `_at` for datetimes, `_date` for
  dates.
- A reproducible constraint / index naming scheme is set via
  `MetaData(naming_convention={...})` on `Base`, with a dict
  you define yourself; SQLAlchemy ships no `POSTGRES_*` constant.
  Any consistent scheme works — predictable names matter more for
  Alembic autogenerate than any particular convention.

## 5. Schemas (Pydantic v2)

Per domain: `Base`, `Create`, `Update`, `Public`. Add
`<Resource>Public` (list response) when pagination exists.

- `ConfigDict(from_attributes=True)` on the public schema
  enables `<Resource>Public.model_validate(orm_row)`.
- **Never** `ConfigDict(json_encoders={...})` — deprecated in
  v2. Serialize with `@field_serializer` instead.
- **Never** `Field(ge=18, default=None)` — constraint contradicts
  default.
- **Never** merge schema with ORM model in one file.
- **Never use `...` (Ellipsis) for required parameters or model
  fields.** It's not needed and is not recommended in modern
  FastAPI / Pydantic v2. For required inputs, declare the type
  and the constraint; for required body fields, declare the
  field without a default. FastAPI and Pydantic v2 infer
  requirement from the absence of a default.
- **Never use Pydantic `RootModel`** — declare the parameter as
  `Annotated[<type>, Body()]` instead and let FastAPI build a
  `TypeAdapter` for you. That works with all FastAPI features
  (validation, OpenAPI, dependency injection).

## 6. Service layer (with optional repository)

Plain `async def` functions. Class form acceptable only when
service is genuinely stateful.

Transactions live here. `async with session.begin():` for
multi-statement work. Cross-aggregate calls go through other
services. Joins / aggregations are SQL. Services never import
from `src.<domain>.router` (back-edge). Services may raise domain
exceptions from `src.<domain>.exceptions`.

**Layer split (production standard):**

- `service.py` orchestrates business logic — accepts a session,
  calls the repository, raises domain exceptions, returns
  schemas.
- `repository.py` is DB-only — accepts a session, runs queries,
  returns ORM rows or domain types. **SQL-first**: do joins,
  filters, and aggregations in SQL, not in Python loops.
- `router.py` calls `service.py`. Routers never touch the
  session directly; they ask the service to do the work.

For small domains the repository can be inlined into
`service.py`. Pull it out when the same query needs to be
called from more than one service, or when the service file
starts mixing orchestration with raw query building.

**Mixing async and blocking code.** When a service is `async def`
but part of its work calls a sync library (e.g. `requests`,
synchronous ORM, file I/O, or a third-party SDK that doesn't
support `async`), wrap that call with `asyncify` from
**[Asyncer](https://asyncer.tiangolo.com/)** (also from the
FastAPI / Tiangolo team) — it runs the blocking call in a
threadpool without manually managing `run_in_threadpool` or
wrapping the whole service in `def`. Asyncer is the canonical
answer for "I have an async endpoint but I need to call a sync
library cleanly."

## 7. Routers

**Always declare router-level `prefix`, `tags`, and shared
`dependencies=` on the `APIRouter` itself** — not at the
`include_router` call site. That keeps the router
self-describing and makes `include_router(app)` a one-liner.

**One HTTP operation per function.** Don't mix `@router.get("/")
+ @router.post("/")` in the same function — separation
keeps OpenAPI / docs / tests / cache invalidation sane.

**Return type or `response_model`.** When the function's
return type already matches the public schema, write the
return type (`-> User`) and skip `response_model` —
Pydantic v2 serializes on the Rust side for performance.
Use `response_model=...` only when the public schema differs
from the internal return value (filtering fields, computed
properties, etc.).

**Never `ORJSONResponse` or `UJSONResponse`.** Both are
deprecated as of 0.131 — declaring a return type (or
`response_model`) lets
Pydantic v2 handle JSON serialization on the Rust side, which
is faster than either and avoids the
`jsonable_encoder` round-trip in the route.

Thin: parse, call `service`, return. Always
`Annotated[T, Depends(...)]` — **never** `def foo(x: T =
Depends(...))`. `async def` for I/O deps; `def` for pure deps.

Return what the endpoint already holds — the ORM row, or the
value `service` handed back — and let the declared return type
or `response_model` validate and serialize it. Constructing the
public schema inside the endpoint validates the same data twice.

## 8. Streaming (SSE / JSON Lines / bytes)

For Server-Sent Events, declare
`response_class=EventSourceResponse` (from `fastapi.sse`) and
`yield` items from the endpoint. Plain objects are
auto-serialized as JSON `data:` fields; yield `ServerSentEvent`
when you need explicit `event` / `id` / `retry` / `comment`
fields.

For JSON Lines or byte streaming, use `StreamingResponse` (from
`starlette.responses`) directly.

## 9. Dependencies

- Domain-local dependencies live in
  `src/<domain>/dependencies.py`. A `valid_<x>_id` dependency
  resolves a path parameter to its object and raises
  `HTTPException(status_code=404)` when there is none.
- Export an `Annotated` alias beside each dependency
  (`ValidUser = Annotated[User, Depends(valid_user_id)]`) so a
  route declares it in one token.
- **Prefer small, decoupled dependencies.** FastAPI caches
  dependency results within a request's scope — the same
  dependency asked for five times in one request runs once — so
  splitting token parsing (`parse_jwt_data`) from an ownership
  check (`valid_owned_post`) costs nothing and makes both
  reusable.
- **`PyJWT` is the JWT library** (`parse_jwt_data`). Never
  `from jose import jwt`.
- Cross-cutting deps live in `src/dependencies.py`. Promote a
  domain dep to `src/dependencies.py` only when ≥ 3 resources
  share it.

## 10. Tests

Placement:

- `tests/conftest.py` — shared async client and db fixtures.
- `tests/<domain>/test_router.py`, `test_service.py`,
  `test_dependencies.py` — one file per domain file exercised.
- `tests/<external_service>/` — one folder per external service
  client.

Tooling and substitutes:

- `pytest` + `pytest-asyncio`. The in-process client is
  **always** `httpx.AsyncClient(transport=ASGITransport(app=app))`.
  Never `TestClient` once the project uses `AsyncSession`.
  **Never** `from async_asgi_testclient import TestClient`.
- **Never** mock the DB in integration tests. Substitutes are
  in-process: a SQLite session for the DB, `httpx_mock` for
  downstream HTTP, a fake server for SMTP.
- Override deps (`app.dependency_overrides[parse_jwt_data] =
  fake_user`), don't monkeypatch internals.
- Test names: `test_<unit>_<scenario>` — pytest functions named
  after the unit and the scenario.
- E2E runs the served app against real services started by
  `testcontainers-python` (Postgres, Redis, etc.).


## 11. Migrations & errors

- `alembic init -t async`. Import every `<domain>.models` in
  `alembic/env.py` so autogenerate sees them. Set a
  human-readable file template:
  `file_template = %%(year)d-%%(month).2d-%%(day).2d_%%(slug)s`
  (e.g. `2022-08-24_post_content_idx.py`).
- **Migrations must be static and reversible.** If a migration
  depends on dynamic data, only the data is dynamic — never
  the schema.
- Generate migrations with descriptive names and slugs.
- Review each migration before merge. Schema changes touch
  every environment; they deserve a second pair of eyes.
- `HTTPException` for HTTP errors in routes/deps.
- Cross-cutting domain exceptions live in
  `src/<domain>/exceptions.py` and are mapped to HTTP in
  `src/main.py`'s exception handlers
  (`@app.exception_handler(MyDomainError)`).
- **Never** `except Exception:` in routes. Catch the narrowest
  class.
- **Never** `BackgroundTasks` for anything you'd page on. If
  the task is short (< 1s) and failure can be silently dropped,
  `BackgroundTasks` is fine. Otherwise use Celery + Redis (or
  arq / Taskiq for async-native).

