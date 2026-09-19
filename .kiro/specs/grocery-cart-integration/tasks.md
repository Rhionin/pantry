# Implementation Plan

## Overview

Replace the no-op `shopping.CartExporter` with a provider-agnostic provisioning core plus one
Kroger adapter, a per-provider fulfillment ledger, and the shopping-list controls that make a
provisioning outcome visible.

The order below is the design's order, and the reasons are load-bearing rather than stylistic:

1. **Subtraction first.** `cart_integrations` is dead and wrongly shaped, and the
   `CartExporter` / `ExportItem` / `ExportError` trio encodes the assumption this feature
   invalidates — that a provisioning result is either `nil` or an `error`. Deleting them before
   building means no new code is written to coexist with a shape it replaces.
2. **Scaffold second: the contract suite and the fake provider, before the engine.** They are
   what makes the capability abstraction more than an assertion. Written after the engine, a
   contract suite degenerates into a description of whatever Kroger happens to do.
3. **Then** the data-access types and credential lifecycle, the quantity computation, the engine,
   the Kroger adapter, the HTTP surface, and the frontend — each mostly assembly once the shapes
   above are settled.

Every task ends in a state where `go build ./...` and `go test ./...` pass. Per AGENTS.md, run
`./scripts/test-coverage.sh` at the end of each parent task; it ratchets its own threshold upward,
so commit the updated script with the code.

Verification follows AGENTS.md. API tests through `handlerTestCase` / `runHandlerTests` with
`afterRequest: exchanges(...)` are the default — a ledger advance is verified by a subsequent
`GET /api/providers/{id}/ledger` exchange, not by querying the table. Direct database queries
appear in exactly two places, both because no endpoint exposes the value: the `ledger_boundary`
precision check and the migration tests. Property tests use `pgregory.net/rapid`, already a direct
dependency and already used in `internal/scan/scan_properties_test.go`. Unit tests are reserved for
pure algorithms: the quantity formula, barcode normalization, and payload validation.

Two things in this plan deliberately change existing behavior, and both are called out where they
land: task 1.4 changes an existing test's expected `$.exported` value from `1` to `0`, and task 6.1
implements a barcode rule that the first draft of this spec got wrong in a way that would have
silently added the wrong products.

Language: Go for the backend, TypeScript/React for the frontend.

## Tasks

- [ ] 1. Subtraction and the core types

  - [ ] 1.1 Migration `006_replace_cart_integrations_with_provider_ledger.sql`
    - `DROP TABLE cart_integrations` — no Go file references it, and its `UNIQUE(user_id)` permits
      one row per user, so it can hold at most one provider's connection state while
      `service_type` shows multi-provider was the intent. SQLite cannot drop the constraint in
      place, so "altering" it means a rebuild either way; dropping a table with zero rows and zero
      readers is strictly better than rebuilding it into a shape that still would not fit
      Requirement 2
    - Create `provider_connections` with `UNIQUE (user_id, provider_id)` — the pair, not `user_id`
      alone, is the single change that makes multiple providers possible
    - Create `fulfillment_ledger` with `PRIMARY KEY (provider_id, item_id)` in that order, because
      the dominant read is every entry for one provider and the provider prefix makes it a range
      scan
    - Create `shopping_list_entry_adjustments` with `PRIMARY KEY (entry_id, provider_id)` — this
      cannot be a column on `shopping_list_items`, because one entry carries a different adjustment
      per provider and setting one must leave the others untouched
    - `ALTER TABLE items ADD COLUMN replenishment_mode TEXT NOT NULL DEFAULT 'target'` — the
      `NOT NULL DEFAULT` gives every existing row `'target'` with no backfill `UPDATE`. The value
      set is enforced in Go, not by `CHECK`: SQLite cannot add a `CHECK` via `ALTER TABLE`, which
      is what migration 005 already records for `products.external_source`
    - `CREATE INDEX idx_consumption_events_item_consumed ON consumption_events (item_id, consumed_at)` —
      no migration in this repo creates an index today, so this is a deliberate first. It is the one
      query this feature adds whose cost grows without bound as consumption history accumulates
    - Latest on disk is `005_add_external_source_and_barcode_misses.sql`, so this is `006`. Nothing
      about `app.RunMigrations` changes
    - _Requirements: 2.2, 6.2, 8.2, 10.12_

  - [ ] 1.2 Migration test obligations in `internal/app/migrate_test.go`
    - Add `applyMigrationsThrough005`, in the same shape as the existing
      `applyMigrationsThrough003` and `applyMigrationsThrough004`
    - `TestMigrationApplies`' `want` list loses `cart_integrations` and gains
      `provider_connections`, `fulfillment_ledger`, and `shopping_list_entry_adjustments`
    - `TestMigrationIsIdempotent`'s expected `schema_migrations` count goes from 5 to 6, and the
      comment naming the files gains 006
    - Seed `items` rows against the pre-006 schema with distinct `target_quantity` values
      **including `NULL`**, apply 006, then assert every seeded `id`, `user_id`, `product_id`,
      `target_quantity`, and `created_at` is unchanged and that `replenishment_mode` reads
      `'target'` for every one. This is the assertion that proves a product whose mode has never
      been set yields the quantity it yielded before the mode existed
    - Seed `consumption_events` rows before 006 and assert all survive the new index
    - These are the migration tests, so direct database queries are correct here — this is the
      "last resort" case AGENTS.md permits
    - _Requirements: 6.2_

  - [ ] 1.3 Core types in `internal/cart`
    - Five capability dimensions as **distinct defined string types** — `AuthCapability`,
      `DeliveryCapability`, `ConfirmationCapability`, `MutationCapability`, `IdentityCapability` —
      each with `Valid() bool` and `Dimension() string`. Distinct types make a dimension swap a
      compile error, so only an out-of-set *value* survives to runtime validation. String values
      are exactly the requirement vocabulary, so SQL and JSON need no translation table
    - `Capabilities` as a flat value struct with `Validate() error`. No pointers, no maps: the copy
      the engine holds cannot be mutated behind the registry's back, and it compares with `==`,
      which the contract suite's coverage table relies on
    - The narrow interfaces: `Provider`, `OAuthFlow`, `StaticCredential`, `ServerPush`,
      `HandoffBuilder`, `DerivedIdentity`, `LookedUpIdentity`, `LineUpdater`, `LineRemover`
    - `DerivedIdentity.DeriveIdentity(barcode string)` takes **no `context.Context`** and no
      credential. That is the highest-value signature choice here: a function that cannot receive a
      context cannot make a well-formed outbound call in this codebase, so "sends no request while
      deriving" is enforced by the method's shape rather than by a reviewer noticing
    - `Credential` interface plus `NoCredential`, `BearerCredential`, `HeaderCredential`. The secret
      lives in an unexported field with no getter and `String()` returns `"bearer(redacted)"`, so a
      careless `log.Printf("%v", cred)` cannot leak — the leak is unrepresentable, not merely
      forbidden
    - `ProductIdentity`, `ResolvedItem`, `ProvisionLine` (carrying `Accounts []ResolvedItem`),
      `ProvisionRequest`. `Accounts` lives **on the line** so that walking from a line result back
      to every entry it accounted for is a field read; a side map from identity to entries breaks
      the moment two entries with the same identity land in different batches
    - `ProvisionResult` with `Disposition` (`Accepted` / `Rejected` / `Indeterminate`),
      `Outcome`, `OutcomeReason`, `EntryOutcome`, `ProvisionReport`. `Indeterminate` is a
      first-class value rather than an error, because a timeout under `add_only` must be
      distinguishable from a rejection
    - `LedgerEntry`, `ConnectionState`, and `Connection`. `Connection` carries no JSON tags — it is
      never marshalled, and the connection-status response is a separate type with no token,
      expiry, or auth-state fields at all, so there is nothing to remember to omit
    - Unit tests for `Valid()` and `Capabilities.Validate()` across in-set and out-of-set values
    - _Requirements: 1.2, 1.7, 5.2, 11.7, 14.2, 14.3_

  - [ ] 1.4 `cart.Registry` and registration validation
    - `Register` validates that an adapter implements **exactly** the narrow interfaces its
      declared capabilities require: every required one present, and no interface belonging to a
      capability value it did not declare. This is where an adapter is stopped from declaring
      `derived` identity while implementing a network lookup
    - Reject a duplicate identifier, an empty identifier, an out-of-set dimension value, and a
      missing required interface — registering no provider, leaving every already-registered
      provider unchanged, and returning an error naming the rejected identifier and the rejected
      dimension. This fails at startup rather than mid-operation
    - Hold the per-provider `BatchSize` policy value and the `Credentials_Configured` indication
    - `Get` resolves an identifier to the capabilities/adapter pair; an unknown identifier is a
      distinguishable miss so the HTTP layer can answer 400
    - Nothing in the registry or the engine may branch on the provider identifier
    - _Requirements: 1.1, 1.3, 1.4, 1.5, 1.6, 1.8_
    - _Properties: 6_

  - [ ] 1.5 Delete the `CartExporter` trio and introduce the `Provisioner` seam
    - Delete `internal/shopping/export.go`'s `CartExporter`, `ExportItem`, and `ExportError`
      (with its `Error()` and `Unwrap()`). `ExportError` exists only to squeeze partial failure
      through an `error` return, which is why today's handler answers 500 for a nine-of-ten success
      and makes the nine successes invisible
    - Add `cart.Provisioner` with
      `Provision(ctx context.Context, userID string, p ProviderID) (ProvisionReport, error)`, and
      `cart.NoOpProvisioner` implementing it
    - `NoOpProvisioner` returns `ProvisionReport{Confirmed: 0, Entries: nil}`
    - **This changes an existing observable number.** `internal/server/handler_shopping_list_export_test.go`
      asserts `$.exported == 1` against today's `{"exported": len(exportItems)}`, which reports
      items as exported when nothing was sent anywhere. Change that assertion to `0`. Reporting a
      confirmed count for units no provider ever saw is precisely the belief error this feature
      exists to eliminate, and leaving it in the no-provider path would make the default install the
      one path that still lies
    - Rewire `internal/server/handler_shopping_list_export.go` and `internal/server/server.go`
      (the `Exporter: &shopping.NoOpExporter{}` line) onto the new seam. Handler field is named
      `Provisioner`
    - `DeriveShoppingList` and `MergeEntries` keep their current signatures and bodies — they are
      wrapped in task 4.1, not changed
    - Ends green: build, existing API tests, and the one amended assertion
    - _Requirements: 11.7, 12.4_

  - [ ] 1.6 Run `./scripts/test-coverage.sh` and commit the ratcheted threshold

- [ ] 2. Scaffold: the contract suite and the fake provider

  Built before the engine. This is the task list that makes the capability abstraction verifiable
  rather than asserted, and it is what a second provider inherits for free.

  - [ ] 2.1 The fake provider in `internal/cart/carttest`
    - `carttest` is a **non-test package**, following the `net/http/httptest` pattern. It must be
      importable by both `internal/cart`'s tests and `internal/cart/kroger`'s tests, and a
      `_test.go` file cannot be imported across packages. Only test files import it, so it ships in
      the module but never in the binary
    - `NewFake(caps cart.Capabilities, script *Script) cart.Provider` returns a provider declaring
      exactly `caps` and implementing exactly the narrow interfaces those capabilities require
    - Compose three mixin families — auth (`noAuth`, `oauthAuth`, `keyAuth`), delivery
      (`pushDelivery`, `handoffDelivery`), identity (`derivedIdentity`, `lookedUpIdentity`) — into
      **twelve** one-line composite structs (3 × 2 × 2), each embedding `fakeCore` plus one mixin of
      each kind. `Mutation` widens the delivery mixin's method set with `LineUpdater` /
      `LineRemover`; `Confirmation` only shapes the scripted result, so neither needs a mixin
    - Twelve one-line declarations rather than one fat fake: a fat fake would be the adapter shape
      this design rejects, would need a registration-validation bypass, and would prove the engine
      works against a shape no real adapter has. `NewFake` output must pass the **same**
      `Registry.Register` validation as Kroger, with no bypass
    - `Script` queues per-request dispositions, per-line results for `per_line`, identity lookups,
      and token-exchange outcomes, all in memory. No socket, no credential
    - Record every adapter call so a test can assert which operations were invoked
    - _Requirements: 16.5, 16.6_

  - [ ] 2.2 `RunContractSuite` and the coverage table
    - `RunContractSuite(t *testing.T, p cart.Provider)` reads `p.Capabilities()` and selects cases
      from a `coverage` map keyed `"dimension=value"`
    - A declared capability value with **no entry in the table** fails the suite, naming the
      provider and the value. That makes "every capability value is covered" a lookup miss rather
      than a convention, which is the mechanism that keeps the suite from decaying into a
      description of whatever Kroger does
    - Cases for every value: the three authentication values, both delivery values, all three
      confirmation values, all three mutation values, both identity values
    - `confirmation` is the one dimension with no interface to omit — `per_line`, `per_request`, and
      `none` all go through `ServerPush.Add` — so its three cases are what prove the engine's
      `switch` over the declared value, and they carry more weight than the others
    - Each case gets a fresh `:memory:` database via `app.RunMigrations`, matching the
      `newTestPantry` / `newTestQueue` convention
    - _Requirements: 16.1, 16.2, 16.3, 16.4_

  - [ ] 2.3 Property tests for capability honesty
    - Property 5: five independent `rapid.SampledFrom` draws give the full 3×2×3×3×2 = 108 space.
      Each iteration builds a fake, registers it, runs an operation against a recording wrapper, and
      asserts both halves — that every invoked operation is one the capabilities require, and that
      the constructed provider implements exactly the required interfaces and no others
    - Property 6: the capability generator crossed with `rapid.SampledFrom` over defect kinds
      (`duplicate_id`, `empty_id`, `bad_value`, `missing_interface`) and which dimension carries the
      defect
    - _Requirements: 1.3, 1.4, 15.6, 16.3_
    - _Properties: 5, 6_

  - [ ] 2.4 Confirm the suite needs no network and no secret
    - Assert the suite opens no socket: the fake is in-memory, and any adapter under test is
      constructed with an **injected `http.RoundTripper`** rather than a base URL. Loopback via
      `httptest.Server` is still a network connection, which is why this goes one step beyond the
      existing `product.NewProductOpenerClient(source, baseURL)` precedent
    - `.github/workflows/ci.yml` passes no secret to either job, so the suite must run unchanged on
      a pull request from a fork — otherwise the abstraction is unproven on exactly the
      contributions most likely to break it
    - Requirement 16.9's credentialed live verification goes behind a `-tags=live` build tag that no
      CI command passes, so no pull request result can depend on whether it ran
    - _Requirements: 16.7, 16.8, 16.9_

  - [ ] 2.5 Run `./scripts/test-coverage.sh` and commit the ratcheted threshold

- [ ] 3. Data access and the credential lifecycle

  - [ ] 3.1 `cart.Ledger`
    - `internal/cart/ledger.go`. Named for the domain concept per AGENTS.md — not a `Repo`, not a
      bare `Store`. This is `internal/cart`'s one data-access type
    - `ListForProvider(ctx, p) (map[string]LedgerEntry, error)` — one query, then O(1) per entry, not
      a per-item read inside the entry loop. A missing key is the "no entry exists" case, resolved by
      the caller to `Requested: 0` with a boundary preceding every consumption event, so no
      absent-row branch leaks into the quantity arithmetic
    - The advance is an `INSERT … ON CONFLICT(provider_id, item_id) DO UPDATE SET
      requested_quantity = requested_quantity + excluded.requested_quantity`, performing the
      read-modify-write in SQL so no lost update is possible. `ledger_boundary` is **absent from the
      `SET` list**, which expresses "an advance moves no boundary" as an absent column rather than a
      rule to remember
    - The advance is additionally conditional on the boundary the operation computed against
      (`WHERE ledger_boundary = ?`). Zero rows affected means a stock-in intervened; see 3.5
    - `ResetForItemTx(ctx, tx, itemID, at)` — the only transaction-bound method, existing so that
      every statement touching `fulfillment_ledger` stays in one package
    - `ResetForProvider` for the owner's explicit reset
    - _Requirements: 10.1, 10.3, 10.7, 10.11, 10.12_
    - _Properties: 12_

  - [ ] 3.2 `connection.Directory`
    - `internal/cart/connection/`. A second package rather than a second type in `internal/cart`,
      precisely because of the one-data-access-type-per-package rule: the ledger and the connection
      records are two persisted concepts
    - Read and write the connection record; derive `Connection_State` including `not_required` for a
      provider whose authentication capability is `none`
    - Disconnect clears the credentials and leaves **every ledger entry unchanged**
    - No method returns a token to a caller outside the package except the broker in 3.4
    - _Requirements: 2.2, 2.10, 4.1, 4.2, 4.3_

  - [ ] 3.3 Authorization state generation and consumption
    - Generate at least 32 characters from an injected `io.Reader` (`crypto/rand` in production, so
      a test can control entropy without weakening production), persist exactly one state per
      provider with its generation time, and leave every other provider's record unchanged
    - Consume on callback: reject an absent or mismatched state, reject past 600 seconds, reject a
      callback carrying an error parameter, and discard the state in every one of those cases
    - _Requirements: 2.3, 2.4, 2.5, 2.6, 2.7, 2.8_
    - _Properties: 15_

  - [ ] 3.4 `connection.TokenBroker` — result-sharing single flight
    - Refresh if and only if the remaining access-token lifetime is 60 seconds or less, or no access
      token is persisted
    - The existing `product.Refresher` precedent uses an in-flight map that **drops** duplicate
      work, so a later caller returns with no result. This needs the opposite: every waiting caller
      must receive the token the one in-progress exchange produced. So `inFlight` holds a
      `*refreshCall{done chan struct{}; token string; err error}` — first caller inserts, unlocks,
      exchanges, fills, closes `done`; later callers block on `<-done` and read the same fields
    - Keyed by `ProviderID`, so a slow provider's flight blocks no request to another provider
    - `golang.org/x/sync/singleflight` stays out of `go.mod`: the product-cache-freshness design
      already declined it for the same kind of gate, and this one shares results rather than
      dropping duplicates
    - **Persist with a compare-and-set** against the refresh token the flight started from
      (`… WHERE provider_id = ? AND refresh_token = ?`). A disconnect landing mid-flight would
      otherwise resurrect the tokens the owner just cleared, leaving Pantry `connected` against an
      unlinked account. Zero rows affected returns "a connection to this provider is required"
    - A rejected refresh token moves the provider to `reauth_required`; any other refresh failure is
      a distinct failure category
    - _Requirements: 3.3, 3.4, 3.5, 3.7, 3.8, 3.9, 3.10_
    - _Properties: 16_

  - [ ] 3.5 Stock-in resets the ledger inside the existing transaction
    - `scan.Queue` gains a `Ledger` collaborator field, following the pattern by which it already
      gained `Pantry` and `Broadcaster`
    - `CommitStockIn` calls `Ledger.ResetForItemTx` **inside its existing `tx`**, setting
      `requested_quantity = 0` and `ledger_boundary = now`. A second transaction after
      `tx.Commit()` could be lost to a crash, leaving instances on the shelf and a ledger still
      claiming units are outstanding
    - _Requirements: 10.6_

  - [ ] 3.6 Run `./scripts/test-coverage.sh` and commit the ratcheted threshold

- [ ] 4. Quantity computation

  - [ ] 4.1 `internal/shopping/compute.go`
    - Pure functions, no database access. `ComputeQuantity` is the whole rule in four lines:
      `max(0, consumed − requested)` under `replenish`, `max(0, target − instances − requested)`
      under `target`, and `0` under `target` when no target quantity is recorded. Both modes net of
      the ledger and both clamp at zero, so non-negativity is a property of a four-line function
      rather than of a call graph
    - `ComputeEntries` is a **wrapper by choice**: call the unchanged `DeriveShoppingList` for the
      `target` basis then subtract `Requested` and clamp; build `replenish` entries directly,
      including products with no target quantity (reachable in `replenish`, unreachable in
      `target`); call the unchanged `MergeEntries` to decide which entry wins; then join the winning
      set back against the per-item basis map already in hand
    - The join is the price of reusing `MergeEntries`, whose return type carries only an item ID and
      a quantity. It buys the guarantee that the manual-precedence rule lives in exactly one place.
      Changing `MergeEntries` would mean editing a function the existing list handler also calls,
      for no gain
    - `ComputeEntries` runs **before** `SyncDerivedItems`, and `SyncDerivedItems` receives the
      ledger-net values — otherwise `shopping_list_items.quantity` for auto rows disagrees with the
      quantity the same read returns
    - Unit tests for the formula; property tests for Properties 1 and 2, including
      `rapid.IntRange(-50, 50)` passes to prove the clamp holds on hostile inputs
    - _Requirements: 7.1, 7.2, 7.5, 7.6, 7.7, 7.8, 7.10, 10.10, 15.7_
    - _Properties: 1, 2_

  - [ ] 4.2 Consumed-unit counts on `suggestion.ConsumptionLog`
    - `ListConsumedAtByItems(ctx, itemIDs) (map[string][]time.Time, error)` — one query returning
      `(item_id, consumed_at)`, with the counting done in Go against each item's own boundary
    - Not a per-item `COUNT(*) WHERE consumed_at > ?`: each item has its own boundary, so the SQL
      form is one query per list entry, and `cmd/server/main.go` sets `SetMaxOpenConns(1)`, which
      serializes those against every other request on the server
    - This is the query the new index in 1.1 exists for
    - _Requirements: 7.3, 7.4_

  - [ ] 4.3 Replenishment mode on `inventory.Pantry`
    - `UpdateReplenishmentMode` and the read, mirroring the existing `UpdateTargetQuantity` field
      for field. No new package: the mode is a column on `items`
    - Enforce the two-value set in Go, and treat an unrecorded mode as `target`
    - Setting the mode of a product with no shopping-list entry records it and applies it at the next
      computation — the mode is product state, not list state
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5, 6.7_

  - [ ] 4.4 Adjustments on `shopping.Store`
    - Set, read, and remove an adjustment keyed by `(entry_id, provider_id)`; reject a value outside
      0–999. No new package: an adjustment is shopping-list state
    - `ClearAdjustmentTx(ctx, tx, entryID, providerID)` for the engine's per-request transaction
    - `SyncDerivedItems` must delete an orphaned entry's adjustments **in the same transaction** that
      deletes the entry. SQLite enforces `REFERENCES` only under `PRAGMA foreign_keys = ON`, which
      `cmd/server/main.go` does not set, so the cascade is an explicit Go statement rather than
      inferred behavior
    - _Requirements: 8.2, 8.3, 8.7, 8.12_

  - [ ] 4.5 Run `./scripts/test-coverage.sh` and commit the ratcheted threshold

- [ ] 5. The Provisioning Engine

  `internal/cart/engine.go`. Collaborators are inline interfaces matching the repo's handler style;
  the field holding the shopping store is named `ShoppingList`, not `Store`.

  - [ ] 5.1 Claim the in-flight slot and gate before touching any provider
    - `map[ProviderID]struct{}` under `Engine.mu`, claimed before any provider contact and released
      in a `defer`. A second operation against the same provider is a conflict; an operation against
      a *different* provider proceeds
    - In-process is sufficient and a database advisory lock would add nothing: one binary, one
      SQLite file, `SetMaxOpenConns(1)`. A crash loses the map along with the process, which is the
      correct recovery — the operation is gone too
    - Gate on `Credentials_Configured` and on `Connection_State`, neither of which touches the
      provider
    - Record the constraint `SetMaxOpenConns(1)` imposes: **no transaction may be held open across a
      provider HTTP call.** An operation can span tens of seconds under the retry policy, and a
      transaction held across it would hold the single connection and stall every other request
    - _Requirements: 9.8, 9.10, 12.9_

  - [ ] 5.2 Resolve identities
    - For each computed entry with a provision quantity ≥ 1, in list order, fetch the product's
      barcodes **sorted ascending lexicographically** and present them in that order, taking the
      first identity returned. The sort is what makes two resolutions over the same barcode set
      agree
    - Dispatch through `DerivedIdentity` or `LookedUpIdentity` by the declared value. A quantity of 0
      is skipped entirely and classified as neither resolved nor unresolved
    - An entry with no identity becomes an unresolved item recorded `failed` /
      `no_product_identity`, and no submitted line may account for it
    - Add `ListBarcodesForProduct` to `product.Catalog`
    - A `looked_up` lookup that errors or is abandoned classifies its entry as unresolved rather
      than failing the operation — no shipped provider declares `looked_up`, so the fake provider is
      what covers this path
    - _Requirements: 5.1, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8, 5.9, 5.10, 5.11_
    - _Properties: 9_

  - [ ] 5.3 Batch into well-formed requests
    - Group resolved items by identity, summing quantities clamped to 999, then slice into requests
      of at most `BatchSize` lines
    - Validate every line before handing it to the adapter: non-empty identity, quantity 1–999, and
      only option keys the provider declares. A malformed presented request is a Pantry bug, so it
      is a 500 naming the rejected field, not a user error
    - Validating here is what keeps one malformed line from taking a whole Kroger batch down with
      it — see the confirmed rejection shape in 6.5
    - _Requirements: 9.1, 9.4, 9.5, 15.1, 15.2_
    - _Properties: 10_

  - [ ] 5.4 Submit
    - `server_push` calls `ServerPush.Add` per request, sequentially. `client_handoff` calls
      `HandoffBuilder.BuildHandoff` once and sends nothing
    - A rejected request does **not** stop the operation
    - Under `add_only`, an abandoned attempt is not retried at the request level, because a request
      that timed out may have been applied. The retry transport handles rate-limit and server-error
      statuses; a timeout on a mutating request yields `Indeterminate`. Conflating the two would
      submit a line twice
    - _Requirements: 9.2, 9.3, 9.6, 9.7, 9.11, 13.7_

  - [ ] 5.5 Record outcomes by the declared confirmation capability
    - A `switch` on the declared value: per-line results under `per_line`; one identical outcome for
      every entry of a request under `per_request`; `unknown` for every entry under `none`, for every
      entry of an abandoned or indeterminate request, and for every entry a handoff artifact carries
    - Every entry whose provision quantity was ≥ 1 receives exactly one outcome with exactly one
      reason
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 11.5, 11.6, 11.7, 13.8_
    - _Properties: 11_

  - [ ] 5.6 The per-accepted-request transaction
    - One transaction per **accepted request**, not per operation: advance the ledger for each
      accounted item and clear that entry's adjustment, then commit
    - Per-request because a rejected request does not stop the operation. If the whole operation were
      one transaction, a later rejection rolling back would erase an earlier confirmation —
      re-creating, in mirror image, the silent-loss defect the ledger replaced the global export
      timestamp to remove
    - If the conditional advance of 3.1 affects zero rows, a stock-in intervened: **skip the advance
      and still record `confirmed`.** The units genuinely were requested, but physical evidence of
      purchase is newer and better information
    - A `failed` or `unknown` outcome writes **nothing** — no advance, no adjustment clear — which is
      what makes no-loss hold by construction rather than by remembering to skip a write
    - _Requirements: 8.9, 8.10, 10.1, 10.2, 10.5, 10.9, 15.4, 15.5_
    - _Properties: 3, 4, 12_

  - [ ] 5.7 `ComputeList` as the single quantity path
    - `ComputeList` is step 5.2's input **and** what `GET /api/shopping-list` returns, so the
      quantity a list read shows is produced by the same code path provisioning consumes. A second
      implementation for display would be a place for the two to disagree
    - Resolve the target provider: the sole configured provider when a read names none
    - _Requirements: 7.11, 7.12, 8.1, 8.4, 8.5_
    - _Properties: 13_

  - [ ] 5.8 Isolation property tests
    - Property 7: two independently generated capability combinations, `rapid.SampledFrom` over the
      operation kind, and a snapshot/compare of the untargeted provider's connection row, ledger
      rows, and recorded calls
    - Property 8: `rapid.SampledFrom` over the mutating operation, then snapshot and compare all
      five state kinds
    - _Requirements: 1.8, 3.7, 4.9, 6.6, 7.9, 8.6, 9.12, 10.4, 11.8, 13.9_
    - _Properties: 7, 8_

  - [ ] 5.9 Run `./scripts/test-coverage.sh` and commit the ratcheted threshold

- [ ] 6. The Kroger adapter

  `internal/cart/kroger/`. Imports `internal/cart`; nothing in `internal/cart` imports it.

  - [ ] 6.1 `barcode.go` — normalization against the corrected rule
    - **Read the design's normalization section before writing a line of this.** The first draft of
      this spec had this rule wrong in the most dangerous possible way, and the correction is the
      reason this task exists as its own unit
    - The identifier is the GTIN with its **check digit discarded**, left-padded with `0` to 13
      characters — *not* the zero-padded barcode. 12 digits: validate the check digit, discard it,
      pad the 11 remaining data digits with two zeros, so `011110728227` yields `0001111072822`
    - Validate the carried check digit on every branch and return `ok=false` on disagreement. This
      is a **required** step, not a nicety: Kroger validates only the 13-character length, so a
      wrong value is accepted and the wrong product is added with no error anywhere. Validation is
      also the only defence that runs in production rather than in CI, and it catches a transposed
      UPC-E placement row about 80% of the time
    - 13 digits: discard the check digit and pad with one zero. This branch is an **extrapolation**
      from Kroger's documented conversion rule with no published worked example — check the design's
      open question before relying on it, and do not quietly widen it
    - A 13-character input is **unambiguously an EAN-13**, because a pantry barcode is stored exactly
      as it appears on the real-world object (Requirement 5.12) — an already-normalized identifier
      has no path into the barcode column, so there is no scheme to guess at here. The converted
      value is derived for the current operation only and must **never** be written back to storage
    - Anything else, any non-digit, or empty: no identifier, so the engine reports the entry
      unresolved
    - _Requirements: 5.12, 17.3, 17.4, 17.5, 17.7, 17.8, 17.15, 17.16, 17.17_
    - _Properties: 17, 18_

  - [ ] 6.2 UPC-E expansion per GS1 Table 5-7
    - Implement the six-row placement table keyed on the sixth encoded digit, transcribed from GS1
      General Specifications Release 26.0 section 5.2.2.4.2, Table 5-7. The number-system digit is
      **always `0`** — GS1 requires it — and the 8-character structure is the number-system digit,
      six encoded digits, and the check digit
    - Pair the property test with a **table-driven unit test whose expected values are transcribed
      from GS1, never generated by running the function first.** Writing the expectations from the
      implementation's own output would make the test tautological, which for this algorithm is the
      entire risk. GS1's four worked examples cover the sixth-digit values 5, 4, 0, and 3, so the
      rows for 1 and 2 must be derived by hand **from Table 5-7** and marked as such
    - Property 19 proves the output's *shape* — 12 digits, number system preserved, valid check
      digit. It cannot prove the *placement*, because a transposed row still produces 12 digits with
      a valid check digit; it would just name a different product. The worked examples and the
      check-digit gate are what cover placement
    - `rapid` generator: the number system is `rapid.Just('0')`, there is nothing to sample; draw the
      sixth encoded digit across all ten values so every placement case is hit
    - _Requirements: 17.6, 17.7_
    - _Properties: 19_

  - [ ] 6.3 Cart-add payload serialization and validation
    - One `items` array; each entry carries the identifier in a field named `upc`, an integer
      `quantity` between 1 and 999, and a `modality` of `PICKUP` or `DELIVERY`
    - Reject before sending — with an error naming the rejected field and no request issued — an
      identifier whose length is not 13 or which holds a non-digit, or a modality outside the two
      values
    - Marshal with `github.com/go-json-experiment/json`, the codec the repo already uses
    - `modality` is optional in Kroger's schema and defaults to `PICKUP`; Pantry sends it explicitly
      anyway, which is a deliberate choice to be explicit rather than an API requirement
    - _Requirements: 17.11, 17.12, 17.14_
    - _Properties: 20_

  - [ ] 6.4 `OAuthFlow` implementation
    - `AuthorizationScope()` returns `cart.basic:write`, the scope string the Cart API declares
    - Authorization URL carries the configured client identifier, the configured redirect URI, the
      declared scope, and `response_type=code`, against
      `https://api.kroger.com/v1/connect/oauth2/authorize`
    - `ExchangeCode` and `RefreshAccessToken` against `/v1/connect/oauth2/token`; record the expiry
      as the response receipt time plus the duration the response carried
    - _Requirements: 2.9, 3.1, 3.2, 3.6, 17.2_

  - [ ] 6.5 `ServerPush.Add`, retry policy, and the injected transport
    - `PUT /v1/cart/add`; a `204` with no body is acceptance. There is no per-item result to read,
      which is why the declared confirmation capability is `per_request` — now confirmed from the
      Cart API document rather than assumed as the conservative reading
    - A `400` carries **one** error for the whole request, so a rejection fails every entry that
      request accounted for, including entries whose own line was well formed
    - `Adapter` holds `Transport http.RoundTripper` (injected, so the contract suite opens no
      socket), `BaseURL` (production or the certification host `https://api-ce.kroger.com`),
      `Modality` validated at construction, `Timeouts`, and `Rand`. **No database handle** — tokens
      arrive as a `cart.Credential` argument, so the adapter is a pure protocol translator
    - Reuse `github.com/justinrixx/retryhttp`, already a direct dependency, with the custom
      `shouldRetry` that adds 429 handling as `product.NewProductOpenerClient` does
    - Implement neither `LineUpdater` nor `LineRemover` — the Cart API has exactly one operation, so
      "invoke only the add operation" is not a check but an absence of any method to invoke
    - Note the documented 5,000-calls-per-day quota where the batch-size default is chosen
    - A provider response body that is not valid in the declared format, or is truncated, returns an
      error identifying the malformed response — which the HTTP layer maps to 502 without echoing the
      body, because a provider response body is where tokens live
    - _Requirements: 13.1, 13.2, 13.3, 13.4, 13.5, 13.6, 15.3, 17.9, 17.10, 17.13_

  - [ ] 6.6 Register Kroger and run the contract suite against it
    - Declare `oauth2_authorization_code` / `server_push` / `per_request` / `add_only` / `derived`
    - Run `carttest.RunContractSuite` against the adapter with a canned-response round tripper, in
      the same `go` CI job, with no secret and no socket
    - A test asserts the shipped registry registers cleanly, so a registration failure — which would
      be a programming error rather than a configuration error — surfaces in CI rather than in
      production
    - The separate credentialed `-tags=live` verification must include **one real imported product
      carrying an EAN-13**: that branch of the conversion rule has no published Kroger worked
      example and could not be verified from documentation, so the credentialed run is the first
      point at which it can be checked at all
    - _Requirements: 16.1, 16.9, 17.1_

  - [ ] 6.7 `loadCartRegistry()` in `cmd/server/main.go`
    - Shaped like the existing `loadScanListenerConfig()`: read every namespaced environment
      variable, trim whitespace, treat whitespace-only as absent, log each absent name
    - Apply defaults rather than failing: an out-of-range batch size applies 50, an invalid modality
      logs the rejected value and applies `PICKUP` while still reporting the provider configured
    - **No absent or invalid environment value may stop the process.** Every configuration failure
      logs and continues; a registration failure skips that provider
    - _Requirements: 12.1, 12.2, 12.3, 12.5, 12.6, 12.7, 12.10, 17.10_

  - [ ] 6.8 Run `./scripts/test-coverage.sh` and commit the ratcheted threshold

- [ ] 7. The HTTP surface

  `internal/server` is the composition root and the only place `cart` and `cart/kroger` meet.

  - [ ] 7.1 `NewHandler` gains a `*cart.Registry` parameter
    - `nil` means no provider is configured and wires `cart.NoOpProvisioner`, which is exactly the
      specified behavior — so the four existing call sites (`cmd/server/main.go`,
      `setup_test.go`, `test_runner_test.go`, `handler_scan_headless_test.go`) pass `nil` and keep
      their current behavior. `NewHandler` gained `refresher` the same way
    - Handler field names per AGENTS.md: `Providers`, `Ledger`, `Connections`, `Provisioner`,
      `ShoppingList`
    - _Requirements: 12.4, 12.8_

  - [ ] 7.2 `GET /api/providers`
    - One row per provider: identifier, display name, the five capability values, connection state,
      and the configured indication — **and no other field**. The response type has no token,
      expiry, or auth-state field to omit
    - _Requirements: 1.5, 4.1, 4.2, 12.8, 14.2_

  - [ ] 7.3 Authorization, callback, and disconnect routes
    - `GET /api/providers/{providerId}/authorize`, `GET /api/providers/{providerId}/callback`,
      `DELETE /api/providers/{providerId}/connection`
    - 400 for authorize on a non-OAuth provider, 409 on an unconfigured one; the callback rejects a
      state mismatch, an expiry past 600 seconds, and an error parameter, and answers 502 for a
      rejected code exchange carrying only the provider identifier, the failure category, and the
      provider status — never a provider response body, because that is where tokens live
    - No redirect `Location`, log line, or error body may carry a credential value
    - _Requirements: 2.5, 2.6, 2.7, 2.8, 2.9, 2.11, 2.12, 4.3, 14.4, 14.5, 14.6_

  - [ ] 7.4 The export handler becomes the provisioning endpoint
    - `POST /api/shopping-list/export` keeps its path and accepts `{"provider": "kroger"}`. The
      operation is "provision the shopping list", not an action on a provider resource, and the
      existing frontend already calls this path
    - Response grows `provider`, `exported`, `failedItems`, `unknownItems`, `entries`, and an
      optional `handoff`. `failedItems` stays a flat `string[]` of names because
      `CartExportButton.tsx` already joins it as strings and `client.ts` already declares
      `failedItems?: string[]` — the field the frontend was written against finally exists.
      `failedItems` and `unknownItems` are **projections of `entries`**, so there is one source of
      truth and the legacy contract is a view over it
    - **A partially failed operation returns 200 with a report, never a 500.** An error status is
      reserved for operations that produced no outcome at all: unknown provider (400), unconfigured
      provider (409), missing connection (409), concurrent operation (409)
    - _Requirements: 9.9, 11.7, 11.13, 11.14_

  - [ ] 7.5 Shopping list, adjustment, and mode routes
    - `GET /api/shopping-list?provider=kroger` returns the computed quantity, the mode that produced
      it, its basis, the target provider, and any adjustment
    - `PUT` / `DELETE /api/shopping-list/items/{id}/adjustment`, 422 naming the rejected value
      outside 0–999
    - `POST /api/items/{itemId}/replenishment-mode`, mirroring `SetTargetQuantityHandler` field for
      field including its 422 for a missing or out-of-set value
    - Verify through `afterRequest: exchanges(...)` — an adjustment's effect is read back through a
      subsequent `GET /api/shopping-list`, not a table query
    - _Requirements: 6.3, 6.8, 6.9, 6.10, 7.11, 8.1, 8.7, 8.8, 8.11, 8.12_
    - _Properties: 13_

  - [ ] 7.6 Ledger routes
    - `GET /api/providers/{providerId}/ledger` and
      `POST /api/providers/{providerId}/ledger/reset`
    - The ledger read is what API tests use to verify a provisioning operation's effect
    - The `ledger_boundary` second-level precision check is one of the two permitted direct database
      queries, because no endpoint exposes that precision
    - _Requirements: 10.7, 10.8, 10.11_

  - [ ] 7.7 Unknown-outcome resolution
    - `POST /api/shopping-list/items/{id}/unknown-resolution` — the owner's "it did reach the
      provider" advances the ledger exactly as a confirmation does; "it did not" leaves the entry's
      requested quantity and boundary unchanged
    - _Requirements: 11.10, 11.11, 11.12_

  - [ ] 7.8 Secret-containment property test
    - Property 14: seed a distinctive generated value for each secret kind into the connection record
      and the scripted provider responses, exercise every cart endpoint, and scan every response
      body, header, `Location`, and captured log line for every seeded value. Log capture follows
      `migrate_test.go`'s `log.SetOutput(&buf)` pattern
    - _Requirements: 14.1, 14.2, 14.3, 14.4, 14.5, 14.6, 14.7_
    - _Properties: 14_

  - [ ] 7.9 Run `./scripts/test-coverage.sh` and commit the ratcheted threshold

- [ ] 8. Frontend

  All Vitest, in the existing `frontend` CI job. The Playwright specs in `frontend/e2e/` are **not
  run by CI today** — the `frontend` job runs `tsc -b`, lint, and `vitest --run` — so no requirement
  here may depend on an end-to-end spec.

  - [ ] 8.1 Provider panel
    - One row per provider from `GET /api/providers`: display name, connection state, and the control
      that state permits. `disconnected` + configured → "Connect"; `reauth_required` → an expiry
      message + "Reconnect"; `connected` → "Disconnect"; not configured → a disabled provisioning
      control plus "unconfigured"; `not_required` → no connection control at all
    - _Requirements: 2.1, 4.4, 4.5, 4.8, 12.8_

  - [ ] 8.2 Per-entry quantity and mode controls
    - Show the computed quantity, the mode that produced it, its basis, and the target provider; a
      mode toggle; and a quantity input recording an adjustment 0–999 that shows the adjusted value
      as the provision quantity alongside the computed value it replaces
    - _Requirements: 6.10, 8.1, 8.11_

  - [ ] 8.3 `CartExportButton.tsx` becomes `ProvisionButton.tsx`
    - Enabled only when the state is `connected` or `not_required`, the provider is configured, no
      operation is in flight, and at least one entry has a provision quantity ≥ 1
    - Disabled with a progress indicator while in flight
    - _Requirements: 4.6, 4.7, 11.15, 13.10_

  - [ ] 8.4 Outcome display
    - Confirmed count when nothing failed; failed names capped at 50 with an overflow count and
      "these items remain on your shopping list"; unknown names with the two resolution controls; an
      error message with **no confirmed count** when the operation returned an error
    - A `fast-check` property for the 50-name cap
    - _Requirements: 11.9, 11.10, 11.13, 11.14_

  - [ ] 8.5 Ledger reset control
    - One per provider, labelled as starting a new cart or having checked out, so the remedy for a
      stale belief is discoverable
    - _Requirements: 10.8_

  - [ ] 8.6 Run `./scripts/test-coverage.sh` and the frontend job's `tsc -b`, lint, and
    `vitest --run`
