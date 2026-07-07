# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- `slurm_account` now refuses to delete the built-in `root` account. A plan
  that would destroy a `slurm_account` named `root` — an explicit `tofu
  destroy`, removing the resource from configuration, a `for_each` key
  disappearing, or a rename (`name` forces replacement) — fails at `tofu
  plan` time (via `ModifyPlan`) with a clear error explaining why, before any
  request reaches `slurmrestd`. There is no valid Slurm workflow that deletes
  just the root account while the cluster stays registered, so this is an
  unconditional error, not a warning. Creating/updating a `slurm_account`
  named `root` is unaffected. Covered by unit tests
  (`account_protected_test.go`) and a live-cluster acceptance fixture
  (`test/fixtures/protected-root-account/`).

- **First-class `fairshare = "parent"` support.** Slurm's fairshare has a
  special "parent" mode in which an account/association inherits its parent
  account's fairshare weight instead of carrying its own; Slurm stores it as
  `shares_raw = 2147483647` (`INT32_MAX` / `SLURMDB_FS_USE_PARENT`) and the
  REST API surfaces it as exactly that number (verified identical across Slurm
  25.05 / 25.11 / 26.05). `slurm_account.fairshare` and the `slurm_user`
  `association.fairshare` attribute now accept the string `"parent"` to request
  that mode, and read it back as `"parent"` (the sentinel is mapped both ways).
  `tools/generate_import/generate_import.py` emits `"parent"` for such
  associations. Covered by `fairshare_test.go` (converters + validator).

### Changed

- **BREAKING: `slurm_account.fairshare` and `slurm_user` `association.fairshare`
  are now strings, not numbers.** This was required to express the `"parent"`
  mode above alongside ordinary integer weights in a single attribute. A plain
  weight must now be quoted: `fairshare = 100` becomes `fairshare = "100"`
  (bare numbers are auto-converted by OpenTofu in most cases, but quoting is
  the supported form and what `tofu plan` shows). The read-only
  `data.slurm_account.fairshare` / `data.slurm_user.association[*].fairshare`
  data-source attributes changed type accordingly.
  - **Migration**: quote every `fairshare` value in your `slurm_account` and
    `slurm_user` configuration (`fairshare = "N"`); no state migration or
    resource replacement is needed — only the attribute's declared type
    changed. In the `examples/big-cluster/` layout, `generate.tf` wraps the
    value in `tostring()`, so both quoted and bare-number YAML keep working.
    The literal number `2147483647` is rejected in config with a hint to use
    `"parent"` instead (it canonicalises to `"parent"` on read).

- **`examples/big-cluster/data/users.yaml` split into `data/users/admin_level.yaml`,
  `data/users/default_accounts.yaml`, and `data/users/wckeys.yaml`** — one file
  per exception concern instead of one combined file, so each stays scannable
  as the cluster grows (skimming "who's an admin?" no longer requires reading
  past every multi-account user's default-account pin). A user needing more
  than one exception gets an entry in more than one file. `generate.tf` and
  `generate_import.py`'s `--layout big-cluster` writer updated to match
  (verified byte-identical, as always); `examples/big-cluster/README.md`,
  `tools/generate_import/README.md`, and `CLAUDE.md` updated throughout.
  - **Migration**: rename `data/users.yaml` into the three new files, keyed by
    which fields each user had set (`admin_level` → `admin_level.yaml`,
    `default_account` → `default_accounts.yaml`, `default_wc_key` →
    `wckeys.yaml`). No provider-level or resource-address changes — this only
    affects the illustrative big-cluster example and the importer's generated
    output.
- Documented that the `Coordinator` role (Slurm's per-account admin list,
  distinct from `slurm_user.admin_level`) is not supported by this provider —
  `internal/client/account.go`'s `Coordinators` field is deserialized from
  API responses but never exposed, read, or written by `slurm_account`. Noted
  in `docs/resources/account.md` (and its template), `examples/big-cluster/README.md`,
  and as a detailed future-work entry in `CLAUDE.md`'s "What's Left" section.
- **`generate_import.py` now emits bare (unquoted) YAML scalars where safe.**
  The `--layout big-cluster` writer previously double-quoted every string; it
  now quotes only values a YAML parser could misread — numeric-looking names
  (`007`), reserved words (`no`, `yes`, `null`), or anything containing YAML
  metacharacters — leaving ordinary usernames and account names (`alice`,
  `web-admin`) unquoted for readability. Purely a formatting change to
  generated output; the parsed values are identical (verified round-trip).
- **`examples/big-cluster/`: account-wide `association_defaults`.** An account
  YAML may now declare an optional `association_defaults:` block (same
  `account_overrides:`/`association:` sub-maps as a member) whose values apply
  to every member that doesn't set the field itself — so a value shared by all
  members, typically `fairshare: parent`, is written once instead of repeated
  on every user. Per-field precedence is member value → account default →
  the field's normal omitted-resolution. `generate.tf` encodes the fallback;
  `generate_import.py` now **emits** this form, hoisting any field carried
  identically by every member of an account into `association_defaults` (only
  ever-unanimous fields, so hoisting can't change anyone's effective config)
  and listing those members as bare names. `teaching.yaml` demonstrates it
  (members inherit `parent`; `dave` overrides with his own `fairshare: 15`);
  documented under "Account-wide association defaults" in
  `examples/big-cluster/README.md`.

### Documentation

- **Documented that removing an Optional attribute from configuration does not
  reset it in Slurm — it only stops managing it.** Because every
  `slurm_account` attribute besides `id`/`name` is Optional-only, deleting a
  field (e.g. `fairshare = "parent"`) omits it from the API upsert and the
  null-preservation rule keeps it out of state, so Slurm keeps the live value
  and `tofu plan` stays clean. To reset, set the value explicitly (`fairshare
  = "1"`); to stop managing without changing Slurm, use `tofu state rm`. Added
  a "Removing an attribute from config does not reset it" section to
  `docs/resources/account.md` (and its `templates/` source) and a note on the
  `fairshare` attribute description in both `slurm_account` and `slurm_user`.
- **Made the `association_defaults` demonstration in `examples/big-cluster/`
  more representative.** Expanded `teaching.yaml` from 3 members to a TA plus
  six bare students, so the "many members inherit one default, written once"
  win (all students get `fairshare: parent` with no per-member repetition) is
  obvious at a glance rather than incidental.

## [0.3.0] - 2026-07-06

### Changed

- **`examples/big-cluster/` account YAML: `members:` renamed to
  `user_associations:`, and the object form's flat list of override keys
  split into two explicit sub-keys.** Prompted by user feedback that
  `partition` (and other association-only fields) being settable in a
  "member override" block was confusing when the account itself has no
  `partition` field to override.
  - `account_overrides` — fields `slurm_account` also has (`fairshare`,
    `default_qos`, `allowed_qos`, `max_jobs`, the 6 TRES fields). A value
    here overrides what the member would otherwise inherit from the
    account.
  - `association` — the 9 fields with no account-level equivalent at all
    (`partition`, `priority`, `max_jobs_accrue`, `max_submit_jobs`,
    `max_wall_pj`, `grp_jobs`, `grp_jobs_accrue`, `grp_submit_jobs`,
    `grp_wall`). Declared, never "overridden" — there's nothing to inherit.
  - `generate.tf` and `generate_import.py`'s `--layout big-cluster` writer
    updated to match (byte-identical, as always); all 6 real
    `data/accounts/*.yaml` fixtures, `examples/big-cluster/README.md`,
    `tools/generate_import/README.md`, and `CLAUDE.md` updated throughout.
  - Verified end-to-end against a live 26.05.1 cluster: `examples/big-cluster/`
    apply → clean plan → destroy, and `generate_import.py` for both
    `--layout flat` and `--layout big-cluster` against the same populated
    cluster, each through the full import → reconcile → clean-plan cycle.
  - **Migration**: this only affects the illustrative big-cluster example and
    the importer's generated output, not the `slurm_user`/`slurm_account`
    provider schema itself — no provider-level compatibility impact. If
    you've already adopted the big-cluster layout from a prior release,
    rename `members:` to `user_associations:` in every
    `data/accounts/*.yaml` file, and split each object-form entry's override
    keys into `account_overrides:`/`association:` per the tables in
    `examples/big-cluster/README.md`.

### Fixed

- **Registry docs (`docs/resources/user.md`) still referenced the pre-rename
  `qos` association attribute** in the import-behavior table and a section
  heading ("Why `qos` is not read during import"), missed when
  `slurm_user`'s `qos` attribute was renamed to `allowed_qos` in `v0.2.1`.
  Corrected to `allowed_qos` throughout, in both
  `templates/resources/user.md.tmpl` and the generated `docs/resources/user.md`.
- Stale `"big-cluster members"` wording in a `generate_import.py` docstring
  (`_account_fields()`), left over from the `user_associations` rename above.

### Documentation

- **`parent_account` drift-blindness is now documented in the published
  registry docs**, not just `CLAUDE.md`. Reproduced live: reconciling
  `parent_account` to a non-root value, then resetting it to `root`
  out-of-band via `sacctmgr`, leaves `tofu plan` reporting "No changes"
  indefinitely — Slurm returns an empty parent for any account directly
  under `root`, which is indistinguishable from "not yet tracked" by the
  provider's null-preservation guard. Added a new
  "`parent_account` drift is not detected once set" section to
  `docs/resources/account.md` (and its `templates/` source), cross-referenced
  from `CLAUDE.md`.
- Clarified `docs/resources/account.md`'s "Null-preservation after import"
  wording: a reconcile apply is only needed for fields your configuration
  actually sets a value for — a bare import against a bare config is already
  a clean `tofu plan` with no reconcile step required. Verified live.
- **`examples/big-cluster/README.md`: verified the account/per-user field
  tables are exhaustive against the current `slurm_account`/`slurm_user`
  schema, and added a "What this layout cannot express" section.** Confirmed
  the 6 real `data/accounts/*.yaml` fixtures collectively exercise all 19
  per-user fields (`account_overrides` + `association`) and 12 of the 13
  account-level fields live — `parent_account` is the sole, deliberate
  exception, already covered by the ordering-caveat note. Added a new section
  spelling out what these YAML files can *never* express and why: QOS
  definitions themselves (belong to `slurm_qos`/`qos.tf`), the built-in
  `root` account, multi-cluster fan-out, an association's `account` (implicit
  from file placement here), `default_wc_key` (user-level only, lives in
  `data/users.yaml`), and `id` (always computed). Also added direct links
  from the two field-reference sections to the matching sections of the
  generated registry docs (`docs/resources/account.md`,
  `docs/resources/user.md`) so the example and the authoritative schema
  reference stay easy to cross-check.
- **`examples/big-cluster/README.md`: added a "Debugging `generate.tf`"
  section.** Explains that the `for_each`/`locals` inversion is expanded
  entirely in memory (no generated `.tf` file ever written to disk), and
  documents concrete tools to inspect it stage-by-stage: `tofu console`
  against individual locals (`local.accounts`, `local.memberships`,
  `local.users`), isolating a single `yamldecode()` call, `tofu plan`/
  `show -json`, `tofu state show`, and `TF_LOG=debug` for provider/API-level
  issues.

## [0.2.1] - 2026-07-04

### Breaking Changes

- **`slurm_user` association attribute `qos` renamed to `allowed_qos`.** It
  was the same Slurm concept as `slurm_account.allowed_qos` (an
  association's QOS list) but named differently depending on which resource
  exposed it — confusing enough in practice to fix now, while the schema is
  still young (pre-1.0; breaking changes ship in minor/patch releases per
  semver's rules for `0.x` versions).

  **Migration**: rename `qos = [...]` to `allowed_qos = [...]` inside every
  `slurm_user` `association` block (`slurm_account.allowed_qos` is
  unaffected — it was already named `allowed_qos`). No resource replacement
  occurs and no manual state surgery is needed: after updating your `.tf`
  files, `tofu plan`/`apply` reconciles the association in place using the
  normal update path (same account+partition key). The `qosAccessHint`
  diagnostic text and all examples, fixtures, and registry docs have been
  updated to match.

### Added

- **CI now exercises `examples/big-cluster/` and `generate_import.py`
  end-to-end**, across the full Slurm version matrix. Two new acceptance
  steps: (1) apply/plan/destroy the big-cluster example directly, and (2)
  populate the cluster with it, then run `generate_import.py` for both
  `--layout flat` and `--layout big-cluster` against that same live state
  and verify each generated config's full import → reconcile → clean-plan
  cycle. This is the CI coverage for everything verified manually while
  building the field reference above (the `allowed_qos` rename, TRES/
  job-count/wall-clock limits at both account and per-member scope,
  `default_wc_key`, and the two documented workarounds).
- **`examples/big-cluster/` README now documents every supported field**
  with complete reference tables and full worked YAML examples for both
  account-level fields (all 13) and per-member override fields (all 19),
  plus a `name:` sanitized-filename example. The real `data/accounts/*.yaml`
  fixtures were enriched to exercise every field live (previously only a
  handful were demonstrated); verified end-to-end against a live 26.05.1
  cluster (`tofu apply` → clean `tofu plan` → `tofu destroy`).
- **Two previously-unknown Slurm/provider interactions discovered and
  documented** while verifying the above (both confirmed against a live
  cluster; not fixed in this release — see `CLAUDE.md`'s "Known
  limitation" entries and the corresponding Notes in
  `examples/big-cluster/README.md`):
  - Setting `partition` on the association for a user's own
    `default_account` creates a phantom duplicate (unscoped) association,
    because `slurm_user.Create()`'s atomic bootstrap call always creates an
    unscoped association for `default_account` before the real,
    plan-declared associations are applied. Safe on any other account a
    multi-account user belongs to.
  - TRES-list fields (`max_tres_per_job`, etc.) merge **per TRES type** on
    read: a member overriding only one TRES type (e.g. `gpu`) while the
    account sets others (e.g. `cpu`) will see Slurm return both merged
    together, so a config declaring only the intended override never
    converges — every type the account sets must be restated. The importer
    already handles this correctly by construction; it only affects
    hand-written configs.
  - Also confirmed (and reverted after testing): a `for_each`-shared
    `slurm_account` resource cannot reference other instances of itself to
    get automatic dependency ordering for `parent_account` — OpenTofu
    rejects it as a cycle even when the specific instances involved
    wouldn't actually cycle at runtime. Documented as a hard limitation
    alongside the pre-existing `parent_account` drift-blindness note.
- **`insecure_skip_ssl_verify` provider option** for connecting to slurmrestd
  over HTTPS with a self-signed or internally-issued certificate. TLS
  certificates are validated by default (secure by default); set
  `insecure_skip_ssl_verify = true` or `SLURM_INSECURE_SKIP_SSL_VERIFY=true` to skip
  validation. The provider emits a plan-time warning whenever it's enabled,
  and the setting has no effect over plain `http://`. Verified against a
  real self-signed-certificate HTTPS endpoint (both the default-secure
  rejection and the opt-in bypass).
- **Data sources for every managed entity**: `slurm_qos`, `slurm_account`,
  and `slurm_user` read existing Slurm entities by name without bringing
  them under provider management.
- **`slurm_partition` data source**: looks up a partition from slurmctld
  (state, flags, allow/deny accounts and QOS, limits, priority). Works with
  every supported Slurm release (API v0.0.42+). Partitions are deliberately
  exposed read-only: Slurm 26.05 added partition create/update/delete via
  REST, but REST-created partitions are not persisted to `slurm.conf` and
  vanish on slurmctld restart, so a managed resource would drift on every
  controller restart.
- **`slurm_conf` and `slurm_dbd_conf` data sources** (Slurm 26.05+ /
  API v0.0.45+): the active slurmctld / slurmdbd configuration as a
  `map(string)` plus typed convenience fields (`slurm_version`,
  `cluster_name`, `conf_path`). Useful for preflight assertions (e.g.
  `AccountingStorageEnforce` includes `associations`) and cluster facts.
- **Version-aware error handling**: features that need a newer Slurm release
  fail at plan time with an actionable message naming the feature, the
  required Slurm release and API version, the configured `api_version`, and
  the fix — before any HTTP call. An HTTP 404 fallback covers the inverse
  case (new `api_version`, older cluster). The provider's connectivity ping
  now also explains a 404 as an `api_version`/server mismatch and prints the
  version table.
- **Plan-time validation**: enum and range checks on resource attributes,
  cross-field invariants on `slurm_user` (`default_account` must match an
  association), and a warning when managing Slurm's built-in system QOS
  (`normal`).
- **Transient-failure retries**: the client retries retryable HTTP failures
  (5xx subset, 408, 429, network errors) with exponential backoff;
  deterministic Slurm rejections are returned immediately.
- Docker test cluster image for **Slurm 26.05.1**; the acceptance-test matrix
  now covers 25.05.4 (v0.0.42), 25.11.5 (v0.0.44), and 26.05.1 (v0.0.45).
  The docker README documents the Slurm-release → API-version mapping.
- Registry documentation: a Slurm version-compatibility matrix on the
  provider index page, per-page version-requirement callouts on the
  v0.0.45-only data sources, and CI validation of every registry doc snippet
  against the live provider schema.
- The API client now propagates `context.Context` through every request and
  retry backoff sleep: cancelling an operation (Ctrl-C, timeouts) aborts
  in-flight HTTP calls and pending retries immediately instead of letting
  them run to completion. Covered by new unit tests
  (`TestDoRequest_CancelledContextStopsRetries`, `TestSleepCtx_ReturnsEarlyOnCancel`).
- Every API request sends `User-Agent: terraform-provider-slurm/<version>`
  so slurmrestd logs can attribute provider traffic. Covered by
  `TestDoRequest_SendsAuthAndUserAgentHeaders`.
- CI: `golangci-lint` job (minimal v2 config: standard set + misspell,
  unconvert) and a `docs-check` job that fails when `docs/` drifts from the
  provider schema.
- `tools/generate_import/generate_import.py` gained a `--layout` flag. The new
  `--layout big-cluster` emits a complete, self-contained account-centric
  directory (`main.tf`, `qos.tf`, `generate.tf`, `data/accounts/*.yaml`,
  `data/users.yaml`, and `for_each`-addressed `imports.tf`) instead of the flat
  per-resource HCL. Account membership is inverted from Slurm's associations,
  `data/users.yaml` holds only exceptions (admins + multi-account defaults),
  account filenames are sanitized while the real name is preserved in a `name:`
  key, and association-less users are skipped with a warning. The flat and
  big-cluster emitters share all field-extraction logic so they cannot drift.
- `examples/big-cluster/`: a data-driven, human-friendly layout for managing
  large clusters (hundreds of accounts, thousands of users). Sysadmins edit
  account-centric YAML under `data/` (one file per account with its member
  list, plus a small `users.yaml` for exceptions), and `generate.tf` inverts
  it into the user-centric `slurm_user`/`slurm_account` resources via
  `for_each`. Includes a README with a daily-ops cheat sheet and a worked
  multi-account user example.
- Documented account-level and per-member **TRES limits** in the
  `examples/big-cluster/` README: `generate.tf` already wired
  `max_tres_per_job`/`max_tres_per_node`/`max_tres_mins_per_job`/`grp_tres`/
  `grp_tres_mins`/`grp_tres_run_mins` through for both account YAML files
  and per-member object-form overrides, but neither was documented. Added a
  reference table for both scopes and a worked example (verified against a
  live plan) to `lab_physics.yaml`.

### Fixed

- CI: `golangci-lint-action` bumped `v7`→`v9` and `hashicorp/setup-terraform`
  bumped `v3`→`v4`, both to pick up their Node.js 24 runtime (removes the
  "Node.js 20 is being deprecated" warnings). Also fixed the actual lint
  failure this surfaced: an unchecked `os.Unsetenv` in
  `internal/provider/provider_test.go`, replaced with `t.Setenv(envVar, "")`.
- `generate_import.py`'s `generate_users()` (`flat` layout) assumed every
  user has at least one association and unconditionally emitted a
  `slurm_user` block, which fails Terraform validation for a
  zero-association user. Fixed to skip such users with a warning,
  mirroring `generate_bigcluster_users()`'s existing behavior. Found via a
  real CI failure: `test/fixtures/user-association-tests/negative/`
  intentionally fails association creation after the user already exists in
  Slurm (an untracked, orphaned resource — see the new `CLAUDE.md` entry),
  and destroying that fixture's own (tracked) account cascade-deletes the
  orphaned user's association too, leaving a genuine zero-association user
  for the next CI step's `generate_import.py` run to trip over.
- CI: the negative-test fixture now deletes its two orphaned users directly
  via the REST API after its own `tofu destroy`, so they don't leak into
  later steps in the same job.
- `examples/big-cluster/data/accounts/lab_chem.yaml` and `shared.yaml` set
  `default_qos` without an `allowed_qos` list, which Slurm rejects at the
  account level with `HTTP 422: ... would not have access to their default
  qos` — apparently never caught before because the full example had never
  been applied end-to-end in one go. Fixed by adding `allowed_qos` to both.
- `slurm_qos` `usage_factor` and `usage_threshold` now use `Float64`, so
  fractional values (e.g. `0.5`) round-trip without truncation.
- The `slurm_user` data source exposes `association` as a
  `SetNestedAttribute`, fixing schema handling of multi-association users.
- The `slurm_partition` data source tolerates fields that older API versions
  omit: `flags` and `preempt_mode` only exist in the v0.0.45 partition
  schema and are null on older versions instead of failing.
- **`tools/generate_import/generate_import.py`** had drifted from the
  `qos`→`allowed_qos` rename above: its internal `_assoc_fields()` extraction
  and its embedded `generate.tf` template still emitted `qos`, which the
  provider no longer accepts and which the big-cluster `generate.tf` would
  silently drop (falls through `try(m.allowed_qos, null)` to `null`). Fixed,
  and the checked-in `examples/big-cluster/generate.tf` is re-verified
  byte-identical to the generator's output.
- The importer was missing `slurm_user.default_wc_key` entirely (both
  layouts); added, reading `user.default.wckey`. Note: a Slurm REST API
  limitation (verified against a live 26.05.1 cluster; not fixable in this
  project) means `default.wckey` always reads back empty regardless of API
  version, so this field can currently never be populated by the importer
  even when set in Slurm — the provider has the identical read limitation.
  Documented in `tools/generate_import/README.md` and `CLAUDE.md`.
- The importer's `_account_fields()` emitted a spurious `organization` equal
  to the account's own name: Slurm defaults `Organization` to the account
  name when unset (verified via `sacctmgr show account`), indistinguishable
  from an explicit value — same class of issue as the existing `fairshare`
  handling. Now omitted when it matches the account name.
- The importer's `_assoc_fields()` could mistake an **inherited** value for
  an explicit per-user override on five association fields (`default_qos`,
  `max_jobs`, `max_tres_per_job`, `max_tres_per_node`,
  `max_tres_mins_per_job`): verified against a live cluster, Slurm's REST
  API resolves these to the account's own effective value even when the
  user's association never set them, with no flag distinguishing the two
  cases. The importer now omits each of these when it matches the parent
  account's own value (generalizing the anti-inheritance check already
  used for `allowed_qos`). `grp_tres`/`grp_tres_mins`/`grp_tres_run_mins`
  do not inherit this way (also verified) and are unaffected. Without this
  fix, importing a user who merely inherits an account limit would pin
  that value to the user forever, silently breaking future propagation of
  account-limit changes.

### Changed

- The API client is split by entity (`cluster.go`, `account.go`, `qos.go`,
  `user.go`, `assoc.go`, `partition.go`, `conf.go`) instead of one
  monolithic `client.go`.
- `examples/` now contains only end-user-facing content (basic example,
  `big-cluster/`, registry-doc fragments). The seven acceptance-test
  fixture directories moved to `test/fixtures/`; CI workflow paths updated.
- `go.mod` now declares directly-imported deps as direct
  (`terraform-plugin-framework-validators`, `terraform-plugin-go`) and pins
  tfplugindocs once (v0.21.0, chosen for compatibility with the committed
  docs formatting and the go 1.24 toolchain); the Makefile runs it via a
  plain `go run` so the go.mod pin is the single source of truth.

### Removed

- Dead client methods `GetAccounts`, `GetAllQOS`, `GetAssociation`,
  `GetUsers`, `GetClusters`, `DeleteCluster` (no references) and the
  superseded `_account_depth()` in `generate_import.py`. The data sources
  share one `configureDataSourceClient` helper instead of inline copies.

### Verified

- slurmrestd (Slurm 25.05.4) returns HTTP 304 for DELETE on nonexistent
  accounts, users, QOS and associations; the client already treats 304 as
  success, so `Delete` is idempotent when a resource was removed
  out-of-band. Locked in by `TestDoRequest_TreatsNotModifiedAsSuccess` and
  exercised end-to-end (out-of-band delete followed by `tofu destroy`).
