# Commerce Card Inventory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let administrators select an existing subscription and manage eligible generated cards directly from each product.

**Architecture:** Keep `CommerceProductCard` as the internal product-card relation. Replace the ID/code form with product-scoped inventory APIs and an existing-style modal; validate card subscription, account count, usage, and assignment on the server.

**Tech Stack:** Go, Gin, GORM, SQLite, existing vanilla JavaScript and `main.css` components.

---

### Task 1: Product-scoped inventory API

**Files:**
- Modify: `handler/commerce.go`
- Modify: `main.go`
- Test: `handler/commerce_inventory_test.go`

- [ ] Add tests proving inventory listing only returns unused cards matching the product subscription and account count, while exposing the current assignment state.
- [ ] Run `go test ./handler -run CommerceInventory -count=1` and verify the new tests fail.
- [ ] Add a GET route that returns eligible and currently assigned generated cards for one product.
- [ ] Replace code-based insertion with card-ID add/remove actions; reject mismatched, used, cross-product, reserved, and sold cards.
- [ ] Run `go test ./handler -run CommerceInventory -count=1` and verify it passes.

### Task 2: Existing-style product controls

**Files:**
- Modify: `static/commerce-admin-fragment.html`
- Modify: `static/js/commerce-admin.js`

- [ ] Replace the subscription text input with the existing `k-dropdown` structure populated by `/admin/accounts/subscription-stats`.
- [ ] Remove the product-ID and pasted-code inventory form.
- [ ] Add an inventory button per product and an existing-style modal containing eligible cards, selection checkboxes, assignment state, and add/remove commands.
- [ ] Confirm no new stylesheet or inline component design is introduced.

### Task 3: Verification

**Files:**
- Verify all modified files.

- [ ] Run `gofmt -w handler/commerce.go handler/commerce_inventory_test.go main.go`.
- [ ] Run `go test ./... -count=1` and verify all packages pass.
- [ ] Run `go build ./...` and verify exit code 0.
- [ ] Run `git diff --check` and verify no whitespace errors.
- [ ] Restart the local server and verify the inventory API responds on port `9527`.
