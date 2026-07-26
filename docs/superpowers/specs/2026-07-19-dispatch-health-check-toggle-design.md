# Dispatch Health Check Toggle Design

## Goal

Add a runtime system setting that controls whether account delivery performs an upstream health check. The setting defaults to enabled so existing installations retain their current behavior.

## Scope

- Apply the setting to card activation and token delivery, including JSON and SSE delivery paths.
- When enabled, preserve the current upstream validation and health persistence behavior.
- When disabled, select and deliver accounts using only persisted database state and existing eligibility filters, including active status, unused state, and subscription match.
- Keep account import health checks and administrator-triggered account refreshes unchanged.

## Settings Model

Add `DispatchHealthCheckEnabled` to `AppSettings` and an optional `dispatchHealthCheckEnabled` pointer to the stored runtime settings payload. The environment/default value is `true`.

The optional stored field provides the upgrade migration behavior: an existing KV JSON document without the field leaves the default value unchanged. No SQL schema migration is needed for either SQLite or MySQL. The next settings save writes the new field into the existing runtime-settings KV document.

The administrator settings API returns and accepts `dispatchHealthCheckEnabled`. Saving updates the in-memory settings and persists the value through the existing KV mechanism, so the change takes effect immediately without restarting the service.

## Delivery Behavior

Centralize the decision behind a small delivery-specific helper rather than scattering raw setting reads throughout handlers. All delivery paths use the helper before calling upstream validation or full account health checks.

When the setting is disabled:

- `popAccount` returns a database-eligible account without calling the upstream models endpoint.
- Multi-account and SSE delivery skip delivery-time `checkAccountHealth` calls.
- Existing bound-card retrieval returns its stored accounts without an upstream call.
- Database eligibility rules remain unchanged; suspended or already assigned accounts are not newly selected.

When enabled, behavior remains identical to the current implementation.

## User Interface

Add a switch to the existing system settings area labeled as the delivery upstream health check. Its supporting text should state that disabling it makes delivery rely on the current database status. The control loads from and saves to the administrator settings API with a fallback of enabled when the server field is absent.

## Error Handling

The new boolean requires no additional validation. Existing save errors continue to use the current settings error response and toast behavior. Disabling the check intentionally accepts the risk that persisted account state may be stale.

## Testing

- Verify missing stored configuration preserves the enabled default.
- Verify explicit stored `false` overrides the default and survives persistence serialization.
- Verify delivery-check policy invokes upstream validation when enabled and bypasses it when disabled.
- Run the complete Go test suite and build.
- Start the application and verify the settings control renders, loads, and participates in the save payload.
