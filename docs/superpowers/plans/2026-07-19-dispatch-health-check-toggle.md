# Dispatch Health Check Toggle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a default-enabled runtime switch that lets delivery rely only on persisted account state instead of calling upstream health endpoints.

**Architecture:** Extend the existing KV-backed `AppSettings` contract and expose one delivery-policy helper. `popAccount` and `popMultipleAccounts` take a database-only fast path when the switch is disabled; the existing upstream paths remain unchanged when it is enabled. The administrator UI reads and saves the same boolean through the existing settings API.

**Tech Stack:** Go 1.24, Gin, GORM, SQLite test database, vanilla JavaScript and HTML

---

### Task 1: Settings compatibility and persistence

**Files:**
- Modify: `handler/settings.go`
- Create: `handler/settings_dispatch_test.go`

- [ ] **Step 1: Write failing settings tests**

Add tests that construct `AppSettings{DispatchHealthCheckEnabled: true}`, merge an empty `storedRuntimeSettings`, and assert the value remains true; then merge `storedRuntimeSettings{DispatchHealthCheckEnabled: boolPtr(false)}` and assert it becomes false. Add a JSON marshal assertion that `dispatchHealthCheckEnabled` is present and false.

- [ ] **Step 2: Run tests and verify RED**

Run `go test ./handler -run 'TestDispatchHealthCheckSetting' -count=1` and confirm compilation fails because the new fields do not exist.

- [ ] **Step 3: Implement the settings field**

Add `DispatchHealthCheckEnabled bool` to `AppSettings`, `DispatchHealthCheckEnabled *bool \`json:"dispatchHealthCheckEnabled,omitempty"\`` to `storedRuntimeSettings`, default it with `envBool("DISPATCH_HEALTH_CHECK_ENABLED", true)`, merge the optional stored field, persist it, return it from `AdminSettings`, bind it in `UpdateAdminSettings`, and copy it into the in-memory settings value.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run `go test ./handler -run 'TestDispatchHealthCheckSetting' -count=1` and confirm all focused tests pass.

### Task 2: Delivery policy and database-only selection

**Files:**
- Modify: `handler/client.go`
- Create: `handler/dispatch_policy_test.go`

- [ ] **Step 1: Write failing delivery tests**

Create an in-memory SQLite database containing eligible accounts with stale or nil `LastCheckedAt`. Set `currentSettings.DispatchHealthCheckEnabled` to false and assert `popAccount` returns the oldest eligible account without upstream access. Assert `popMultipleAccounts(2, subscription)` returns two eligible matching accounts. Include suspended, used, nonzero-credit, and mismatched-subscription records and assert they are excluded.

- [ ] **Step 2: Run tests and verify RED**

Run `go test ./handler -run 'TestDeliveryWithoutHealthCheck' -count=1` and confirm it fails because existing selection attempts upstream health checks.

- [ ] **Step 3: Implement centralized delivery policy**

Add `dispatchHealthCheckEnabled() bool` backed by `GetCurrentSettings()`. Add database-only branches to `popAccount` and `popMultipleAccounts` immediately after loading eligible ordered candidates. The single-account branch returns the first candidate satisfying `isDispatchable`; the multi-account branch returns the first `n` matching candidates or `gorm.ErrRecordNotFound`. Leave import and administrator refresh functions unchanged.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run `go test ./handler -run 'TestDeliveryWithoutHealthCheck' -count=1` and confirm all focused tests pass.

### Task 3: Administrator settings control

**Files:**
- Modify: `static/index.html`
- Modify: `static/js/settings.js`

- [ ] **Step 1: Add the switch markup**

Add a `settings-toggle-card` near the upstream concurrency controls with checkbox id `settingDispatchHealthCheckEnabled`, label `发货前上游健康检查`, and help text explaining that disabling it relies on the current database status.

- [ ] **Step 2: Wire load and save behavior**

In `loadSettings`, set the checkbox with `d.dispatchHealthCheckEnabled !== false` so older servers/data default to enabled. In `saveSettings`, include `dispatchHealthCheckEnabled: document.getElementById('settingDispatchHealthCheckEnabled').checked`.

- [ ] **Step 3: Check static contracts**

Run `rg -n "settingDispatchHealthCheckEnabled|dispatchHealthCheckEnabled" static/index.html static/js/settings.js handler/settings.go` and verify markup, load, save, API response, request binding, default, merge, and persistence are all represented.

### Task 4: Full verification

**Files:**
- Modify: `.env.example`

- [ ] **Step 1: Document the optional environment default**

Add `DISPATCH_HEALTH_CHECK_ENABLED=true` with a concise comment explaining that it controls delivery-time upstream checks before KV settings override it.

- [ ] **Step 2: Format and test**

Run `gofmt -w handler/settings.go handler/settings_dispatch_test.go handler/client.go handler/dispatch_policy_test.go`, then `go test ./... -count=1`, and confirm zero failures.

- [ ] **Step 3: Build**

Run `go build ./...` and confirm exit code 0.

- [ ] **Step 4: Rendered verification**

Restart the local service, open the administrator settings page, and verify the new switch renders checked by default, toggles locally, and is included in the settings save contract without introducing console errors.

- [ ] **Step 5: Review final diff**

Run `git diff --check`, `git status --short`, and `git diff --stat`; confirm there is no whitespace damage or unrelated source churn.
