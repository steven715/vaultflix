# ADR-0013: One route table declares access; drop Casbin

- Status: Accepted
- Date: 2026-10-11

## Context

Three separate lists described the protected API, and they had already drifted apart:

- **Routes** — about 50 gin registrations in `cmd/server/main.go`.
- **Roles** — `casbin/policy.csv`. Admin had one `/api/*` wildcard per method; viewer had 17 explicit rows. The file was baked into the image, so nothing could change it at runtime.
- **Stream Token scope (Token Scope)** — `streamRoutePaths` in `internal/middleware/auth.go`, listing where a `scope=stream` token works.

Two kinds of drift were already in the code:

- The viewer Role had no rule for the two HLS routes, so viewers could not play remux Videos. Meanwhile `auth.go` explicitly allowed stream tokens on those same routes.
- `policy.csv` granted the viewer a `PUT /api/watch-history` route that did not exist.

No test loaded the real policy.

Casbin was used only to answer "may this Role call this route?". There were no role hierarchies, no per-resource rules, and no runtime policy edits. `middleware/rbac.go` just passed calls through to the enforcer.

## Decision

- **One route table** (`cmd/server/routes.go`) lists every protected route with its method, path, handler, `Viewer` and `StreamToken`.
  - `Viewer` says whether the viewer Role may call the route. Admin may call every route.
  - `StreamToken` says whether a `scope=stream` token may call the route, and then only for its own `:id`.
- **`middleware.RegisterRoutes`** registers each route behind its own guard. That guard enforces both the Role rule and the stream-token rule.
- **`JWTAuth` only authenticates.** It records the token's scope and bound Video in the context; the guard does the checking.
- **Casbin is removed:** the dependency, `casbin/model.conf`, `casbin/policy.csv` and `middleware/rbac.go`.
- **Tests pin the outcome.** `TestAPIRoutes_ViewerPermissions` and `TestAPIRoutes_StreamTokenRoutes` list exactly what viewers and stream tokens can reach, so any change to permissions shows up as a test diff.

## Alternatives rejected

- **Keep Casbin, generate its policy from the table.** It would keep a layer that adds nothing at this scale (ADR-0009).
- **Keep the three lists and add a cross-check test.** It catches drift, but every new route would still need edits in three places.

## Consequences

- Adding a route means adding one table row. Its permissions are visible right next to its handler.
- If per-resource or ownership rules are ever needed (for example, multiple users with private libraries), access control has to be designed again. A Role-per-route table will not be enough.
- A `403` now comes from the route guard, not from Casbin. CLAUDE.md and `docs/api.md` are updated.
