# ADR-0004: Soft-delete users via `disabled_at`

- Status: Accepted
- Date: 2026-04-06

## Context

Admin needs to revoke a user's access while preserving their related data (favorites, watch history) — the delete modal promises exactly that.

## Decision

"Deleting" a user is a soft delete: set `disabled_at TIMESTAMPTZ` (nil = active) instead of removing the row. `AuthService.Login` rejects disabled accounts with a dedicated `ErrAccountDisabled` sentinel → **403** (distinct from 401 invalid-credentials). A disabled user can be re-enabled (`EnableUser`). Admin accounts are protected from being disabled (`ErrCannotDisableAdmin`). `DELETE /api/users/:id` is semantically a disable (returns 204, removes no data).

## Alternatives rejected

- **Hard `DELETE FROM users`** — cascades/orphans favorites + watch history, loses the audit trail, and is irreversible (no re-enable).

## Consequences

- Username stays taken after disabling.
- Every login path and user query must account for the disabled state.
- The DELETE endpoint's name no longer matches its behavior — documented to avoid surprise.
