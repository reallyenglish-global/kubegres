# Re-work: refactoring plan for quality and readability

Scan date: 2026-10-10, against `main` at `4b33206`.
Scope: `cmd/`, `api/v1/`, `internal/controller/**`, `internal/test/**`, Makefile, lint and CI config.

Goal: make the operator easier to read, test, and change safely, without
changing user-visible behaviour or the CRD/status contract unless an item says so.

## How items are prioritized

Each item is scored with `Priority = (Impact + Risk) x (6 - Effort)`, each 1 to 5.
Phases are ordered so that each one makes the next safer: fix real bugs, add a
safety net (lint and unit tests), clean up mechanically, then restructure.

| Phase | Theme | Rough size |
|---|---|---|
| 0 | Correctness bugs found during the scan | 1 to 2 days |
| 1 | Safety net: lint in CI, unit tests around core logic | 1 week |
| 2 | Mechanical cleanups: dead code, names, constants, logging | 1 week, many small PRs |
| 3 | Structural refactors: enforcers, templates, spec checker, context | 2 to 4 weeks |
| 4 | Move validation and defaults into the CRD, API polish | 1 to 2 weeks |
| 5 | E2E test suite hygiene | ongoing, alongside the above |

Each phase can ship as small PRs alongside feature work. No phase needs a
feature freeze.

---

## Phase 0: correctness bugs (do first, each with a regression test)

These came up during the readability scan. They are real defects, not style issues.

| # | Location | Problem | Fix | Score |
|---|---|---|---|---|
| 0.1 | `spec/enforcer/resources_count_spec/statefulset/failover/PrimaryToReplicaFailOver.go:328-334` | `Client.Update` error is in `err2` but the function returns `err` (nil). A failed promotion is reported as success. | Return the update error; drop the `err2` name. | (5+5)x5 = 50 |
| 0.2 | `states/statefulset/StatefulSetWrapper.go:70-74` | `copy(toCopy, copyAll)` has its arguments reversed, so it copies nothing and returns the internal slice. Callers alias internal state. | `slices.Clone(toCopy)`. | (4+4)x5 = 40 |
| 0.3 | `kubegres_controller.go:84-88` | Every `Get` error, including transient API errors, ends reconcile with `nil`, so no retry. | `client.IgnoreNotFound(err)`. | (4+4)x5 = 40 |
| 0.4 | `resources_count_spec/statefulset/PrimaryDbCountSpecEnforcer.go:259,270` | `_ = Client.Get(...)` treats any API error as "PVC absent". The bootstrap fail-closed check relies on this. | Return `(pvc, found, err)`; only `IsNotFound` means absent. | (4+5)x4 = 36 |
| 0.5 | `ReplicaDbCountSpecEnforcer.go:100` | `resetInSpecManualFailover()` error is ignored. | Check and return it. | (3+3)x5 = 30 |
| ~~0.6~~ | `CustomConfigSpecEnforcer.go:57` | Dropped: false positive. `ConfigureStatefulSet` returns `(changed bool, details string)`, so no error is discarded. | None. | n/a |
| ~~0.7~~ | `StorageClassSizeSpecEnforcer.go` | Moved to 3.2. `MustParse` cannot panic because `SpecChecker.go:99` rejects an unparseable size first. "No difference on error" comes from the `CheckForSpecDifference` interface, which has no error return. | Give `CheckForSpecDifference` an error return as part of 3.2. | n/a |
| 0.8 | `BackUpCronJobCountSpecEnforcer.go:161-164` | `shouldUseEphemeralVolume` error ignored inside `hasSpecChanged`. | Propagate it. | (2+3)x5 = 25 |
| 0.9 | `internal/test/util/DbConnectionTestUtil.go:109-158` | Calls `Close()` on a possibly nil `db`; logs the connection string, which includes the password. | Guard the nil; redact the password. | (2+4)x5 = 30 |

Status: 0.1 to 0.9 are merged (#33 to #39). Their line numbers now point to the rewritten code, so read them as history.

### Phase 0b: further defects found reviewing main after the Phase 0 merges

| # | Location | Problem | Fix | Score |
|---|---|---|---|---|
| 0b.1 | `failover/PrimaryToReplicaFailOver.go:309` | Promotion assigns a new `core.Lifecycle{PostStart: ...}`, which drops the template's `preStop` (graceful `pg_ctl stop`). `PreStopSpecEnforcer` reports no difference when the spec has no preStop, so it is never restored. Every promoted primary loses its graceful-stop hook. | Set only `Lifecycle.PostStart`, creating `Lifecycle` if nil. | (4+5)x5 = 45 |
| 0b.2 | `statefulset_spec/AllStatefulSetsSpecEnforcer.go:220-221` | `RemoveActiveOperation()` runs before `OnSpecUpdatedSuccessfully`. If `PortSpecEnforcer`'s Service delete fails, the operation is already gone and the hook is never retried, so the Service keeps the old port. | Run the hook first and remove the operation only on success. | (3+4)x5 = 35 |
| 0b.3 | `kubegres_controller.go:115-116` | `HasSpecFatalError` returns `ctrl.Result{}, nil`. Several fatal errors are fixed by creating objects the controller does not watch (custom ConfigMap, StorageClass, backup PVC), so the cluster stays stuck until a resync or a manual edit. | Return `RequeueAfter` (about 30s) on fatal spec errors. | (4+4)x5 = 40 |
| 0b.4 | `ResourcesCountSpecEnforcer.go:35-41`, `kubegres_controller.go:151-158` | The first failing count enforcer aborts the reconcile, so an unrelated CronTasks, backup or PDB error blocks all StatefulSet spec enforcement (image, resources, storage resize). | Run every enforcer and `errors.Join` the results, or order the non-critical ones last. | (3+4)x4 = 28 |
| 0b.5 | `kubegres_controller.go:61-66` (RBAC markers) | ClusterRole grants create and delete on pods, PVCs and ConfigMaps, plus update, patch and delete on events, beyond what the code uses. | Trim the markers to the verbs the code uses, then run `make manifests`. | (2+4)x4 = 24 |
| 0b.6 | `spec/template/bootstrap.sh:56-57,68-69` | The password and identity files are written, then `chmod 600`, so they briefly exist with umask-default mode. | `umask 077` at the top of the script. | (1+3)x5 = 20 |
| 0b.7 | `SpecChecker.go:216`, `ReplicaDbCountSpecEnforcer.go:225` | `Client.Update(Kubegres)` decodes the server copy into the shared object, discarding status changes made earlier in the reconcile (blocking-operation transitions). It self-heals next reconcile but can repeat events. Not yet reproduced. | Preserve status across the spec write, or use `Patch` on a copy. | (2+3)x3 = 15 |

Lower confidence, to confirm before fixing: unguarded `[0]` indexing and `*storageClassName` deref in `SpecChecker.go:72,84-85`, and `CronTasksCountSpecEnforcer.go:83-84` calling `Update` on every reconcile without a comparison.

---

## Phase 1: safety net

Restructuring without fast tests means waiting for the 12-shard kind suite to
catch mistakes. Build the net first.

### 1.1 Run lint in CI and upgrade golangci-lint (score 36)
- golangci-lint never runs in CI: `.github/workflows/test.yml` has no lint step, and `test-fast` does not call `make lint`.
- The Makefile pins `v1.61.0`, which most likely cannot load a `go 1.26.0` module. Migrate to v2 with `golangci-lint migrate`: `version: "2"`, drop `gosimple`/`typecheck`, move `gofmt`/`goimports` under `formatters`.
- `.golangci.yml` disables `dupl` and `lll` for `internal/*`, which is nearly all Go code. That hides the duplication listed in Phase 3. Narrow the exclusion to generated code and test fixtures. To avoid a big-bang cleanup, start in "new issues only" mode (`--new-from-rev`).
- Set `gocyclo.min-complexity` explicitly (start at 20, lower over time).

### 1.2 Unit tests for core logic that has none (score 32)
Packages with zero unit tests today: `operation`, `states`, `states/log`, `ctx/**`, `spec/defaultspec`, `spec/enforcer/comparator`, and the main path of `AllStatefulSetsSpecEnforcer`. The Primary and Replica count enforcers and failover now have narrow regression tests from Phase 0, but not decision-logic coverage.

Priority order:
1. `operation.BlockingOperation` state machine. Inject a clock first (`BlockingOperation.go:257` calls `time.Now()` directly).
2. `comparator.PodSpecComparator` (pure logic, cheap to test).
3. Failover selection, timeout and promotion paths in `PrimaryToReplicaFailOver`.
4. `PrimaryDbCountSpecEnforcer` and `ReplicaDbCountSpecEnforcer` decision logic. These become easy to test after 3.3.
5. `defaultspec` defaulting.

Use the table-test style already used in `maintenance/` and `majorupgrade/`.

### 1.3 Remove the dead `test-e2e` Makefile target (score 15)
`Makefile:99` runs `go test ./test/e2e/`, which does not exist. Delete it or point it at `internal/test`.

---

## Phase 2: mechanical cleanups (low risk, high readability return)

Each bullet is one small PR. None of them change behaviour.

### 2.1 Delete dead and commented-out code (score 28)
- `PrimaryToReplicaFailOver.go`: `hasLastFailOverAttemptTimedOut` (192), `isWaitingBeforeStartingFailOver` (239), `getStatefulSetByInstanceIndex` (254), `promoteReplicaToPrimary` (289), `waitBeforePromotingReplicaToPrimary` (340), `activateOperationWaitingBeforeFailingOver` (369), `activateOperationFailingOver` (381). None has a caller.
- `CustomConfigSpecHelper.go`: `getVolumeMountIndex`, `isCustomConfigMapNameDifferentThanKubegresSpec`, `updateCustomConfigMapNameWithKubegresSpec`, `doesCustomConfigExist`; also `ResourcesCreatorFromTemplate.go:369 doesCustomConfigExist`.
- Commented-out blocks: `StatefulSetWrapper.go:71-81`, `ResourcesCreatorFromTemplate.go:66`, `kubegres_controller.go:81`, `ResourcesContext.go:87`.
- Leftover kubebuilder `TODO(user)` comments: `kubegres_controller.go:73-76`, `cmd/main.go:122`.
- `PostgresMinorUpgradeEnforcer.go:39-41`: a no-op `continue`.
- Unused `resourcesStates` fields in `LivenessProbeSpecEnforcer.go:32` and `StartupProbeSpecEnforcer.go:33`.

### 2.2 Name the magic values (score 24)
- Label keys `"app"`, `"index"`, `"replicationRole"` and the roles `"primary"`/`"replica"` are scattered across states, templates and enforcers. Add one `labels.go` with constants. Pod-disruption reasons in `PodsStates.go:140` already exist as constants in `maintenance/classifier.go:27-30`; reuse those.
- Bootstrap states `"completed"`/`"reserved"`: make them a typed constant set.
- PVC naming `"postgres-db-"+name+"-"+idx+"-0"` (`PrimaryDbCountSpecEnforcer.go:195,274`): one `pvcNameFor(idx)` helper.
- Timeouts (300/60/10/600/30 s), the 10 s and 40 s failover waits, and the 20 s requeue cap (`BlockingOperation.go:57`): named `time.Duration` constants.
- `KubegresInternalAnnotationKey` actually holds kubectl's `last-applied-configuration`; rename it.
- `defaultMode 0777` on the custom config volume: world-writable is not needed; use `0555` or `0755`.

### 2.3 Fix names and spelling (score 20)
- `Nbre*` becomes `Num*`/`Count`; `returnn` becomes `finishWithStatus`; `getDeployedCustomCustomConfigMap`; `"Based ConfigMap"`; `"Cannot active"` (`BlockingOperationError.go:24`); `PostgreConf`; `"PostgreSql"`.
- `StorageClassSizeSpecEnforcer` enforces size, not storage class; rename it to `StorageSizeSpecEnforcer`.
- `BackUpCronJobCountSpecEnforcer.go:227` logs reason `PodSpecEnforcementErr` for a CronJob delete.
- Do not rename persisted status fields here (for example `TimeOutEpocInSeconds`). See 4.4.

### 2.4 Rename packages that shadow stdlib names (score 21)
`ctx` and `log` shadow `context` variable conventions and the stdlib `log`, which forces aliases like `ctx2`, `log2`, `log3`, `template2`, `statefulset_spec2`, `resources_count_spec2`. `ResourcesContext.go:36-37` imports the same package twice under two names.
- Rename the packages to `kgcontext` and `kglog` (or similar), then remove every numbered alias.
- Standardize file names on lowercase snake_case. PascalCase files (`SpecChecker.go`) now sit next to snake_case ones (`bootstrap_validation.go`) in the same package. Do this rename in its own PR so `git log --follow` stays clean.

### 2.5 Logging noise (score 20)
- `kubegres_controller.go:83-84`: two `=====` banners at Info on every reconcile. Delete them.
- `ResourcesContext.go:86` logs the whole status at Info. `ResourcesStatesLogger` and `BlockingOperationLogger` emit about 10 Info lines per reconcile. Move these to `V(1)` or collapse each into one structured line.
- `cmd/main.go:79` sets zap `Development: true` in the production setup.

### 2.6 Small consistency items (score 15)
- `Requeue: true` with `RequeueAfter` (`kubegres_controller.go:105`): `Requeue` is deprecated and redundant here.
- `SetupWithManager` builds its own `context.Background()`; take the ctx from the caller.
- `CreateOwnerKeyIndexation.go:36-91`: the same closure repeated three times; replace it with a loop.
- Map iteration used for validation order (`bootstrap_validation.go:24-38`, `majorupgrade/plan.go:36`) makes error messages nondeterministic. Use a slice.

---

## Phase 3: structural refactors

These have the most readability payoff. Each one needs Phase 1 tests for the
area it touches.

### 3.1 Remove `time.Sleep` from Reconcile (score 32)
`kubegres_controller.go:140` sleeps 1 s on every reconcile to wait for the cache.
It blocks a worker, and with `MaxConcurrentReconciles` it throttles every
cluster. Replace it with an uncached `APIReader` read where freshness matters, or
a `ResourceVersion` check plus `RequeueAfter`. The comment explains why it
exists, so this needs a targeted e2e test for the stranded-operation case before
it is removed.

### 3.2 Collapse the per-field StatefulSet enforcers (score 28)
About 10 files under `statefulset_spec/` (Image, ServiceAccountName, Liveness, Readiness, Startup, PreStop, Affinity, Tolerations, SecurityContext, TerminationGracePeriod) repeat the same steps: read current, read expected, compare, record a difference, assign.
- Replace them with one generic `fieldEnforcer[T]{name, get, set, equal}` and a table of field definitions.
- 15 of 16 `OnSpecEnforcedSuccessfully` implementations just `return nil`. Make it an optional `PostEnforceHook` interface; only `PortSpecEnforcer` implements it.
- `VolumeSpecEnforcer.go` has type-duplicated pairs (compare, index-of, to-string for volumes and mounts). Replace them with generics and `slices.IndexFunc`.
- Give `CheckForSpecDifference` an error return, so lookups such as the PVC read in `StorageClassSizeSpecEnforcer` stop turning API errors into "no difference" (moved from 0.7).
- Move `isPrimaryDbReady` (defined 5 times) and `hasPrimaryEverBeenDeployed` (twice) onto `states.StatefulSetsStates`.

### 3.3 Split decide from apply in the count enforcers and failover (score 27)
`PrimaryDbCountSpecEnforcer`, `ReplicaDbCountSpecEnforcer`, `BaseConfigMapCountSpecEnforcer` and `PrimaryToReplicaFailOver` mix decisions, status writes and API calls in long functions (`ReplicaDb.Enforce` plus `enforceReplicaCount` 70 lines, `deployNewPrimaryStatefulSet` 53).
- Introduce a pure `decide(states) Action` step and a thin `apply(Action)` step. The decision logic then becomes unit-testable without a cluster.
- Extract the repeated "activate blocking operation, act, on error remove the operation, log" sequence into one `withBlockingOperation(config, action)` helper (it appears in `ReplicaDb...:252-282,296-319`, `PrimaryDb...:181-206`, `BaseConfigMap...:122-144`).
- Extract the repeated "operation timed out, re-enable, log" block (`ReplicaDb...:82-92`, `BaseConfigMap...:70-80`, `AllStatefulSets...:111-121`).
- `AllStatefulSetsSpecEnforcer.go:280-360`: three near-identical completion checkers and three activate wrappers become one parameterized helper each.

### 3.4 Templates: `go:embed`, decode once, patch by name (score 27)
- `yaml/Templates.go` (627 lines) is generated by `cmd/yamlcopy`, which ignores all its errors, only works when run from `cmd/`, and can drift from the YAML. Replace both with `//go:embed *.yaml`.
- `ResourceTemplateLoader` re-parses YAML on every call and uses unchecked type assertions that panic if a template kind changes. Decode once at startup, `DeepCopy()` per use, and use comma-ok assertions.
- Code patches templates by fixed index: `Containers[0]`, `Env[0..3]`, `Volumes[1]` and so on (36 uses of `Containers[0]` in enforcers alone). Reordering a YAML entry silently writes the wrong field. Add `postgresContainer(sts)`, `findEnv(c, name)` and `findVolume(spec, name)` helpers, and use them everywhere.
- Primary and replica StatefulSet templates are about 90% identical, and `initStatefulSet` reads the role back from a template label. Use one base template plus a typed role parameter with role-specific mutators.
- Split `initStatefulSet` (105 lines) into `applyMetadata`, `applyContainer`, `applyStorage`, `applyScheduling`, `applySecurity` and `applyProbes`.

### 3.5 Split `SpecChecker.CheckSpec` (score 24)
`CheckSpec` is 163 lines. It:
- sets `HasSpecFatalError` in 20 copy-pasted blocks, where (except the three that return early) each later message overwrites the earlier one, so users only see the last error;
- writes rollback values into the CR and calls `Client.Update` (side effects inside a checker), logging but not returning that error;
- always returns a nil `error`.

Refactor into small validators that return `field.ErrorList`, aggregate all errors into one message, and move the rollback writes into a separate enforcer step. The three `doCustomVolume*HaveReservedName` loops (332-357) become one helper.

### 3.6 Slim down `ResourcesContext` and the context and logging plumbing (score 21)
- `ResourcesContext` is a hand-built DI container with 22 exported fields, rebuilt on every reconcile. `addBlockingOperationConfigs` silently depends on running after the other `add*` functions. Builders should return values instead of mutating `rc`, and the struct should expose only what `Reconcile` uses (status, blocking operation, spec checker, two top-level enforcers).
- `context.Context` is stored in `KubegresContext` and `KubegresStatusWrapper` and copied by value into about 30 components. Pass `ctx` as a parameter, at least on methods that do I/O.
- `LogWrapper`: `WithValues`/`WithName` mutate a struct copied by value, so the change is lost. `InfoEvent` passes its varargs as a single argument, which is why `InterfaceArrayUtil.go` needs reflection. Use `logr` directly plus a small `Eventf` helper, and delete the reflection code.
- Constructors are Java-style `CreateX` returning values with pointer-receiver methods. Move to `NewX` returning `*X` as each file is touched (not as a big-bang rename).

### 3.7 BlockingOperation clarity (score 20)
- `LoadActiveOperation` loads state, mutates status and returns requeue seconds. Split it into `Load()` and `RequeueAfter() time.Duration`.
- `removeOperationIfNotActive` (`BlockingOperation.go:190-222`) is a 33-line nested-if with duplicated branches. Use a single `complete(timedOut bool)` path.
- `getConfig` returns a zero config on a miss. Use map `ok` and return an error.
- Replace the string-sentinel errors in `BlockingOperationError.go` with `var Err... = errors.New` and `errors.Is`.
- Operation and step IDs are human sentences persisted in status. Changing them is a status migration, so add typed constants that keep the current string values now, and handle any value change under 4.4.

### 3.8 States loaders (score 18)
- The "Get, NotFound means nil, otherwise ErrorEvent" block is repeated 6 times (`BackUpStates.go:88,108`, `ConfigStates.go:151`, `DbStorageClassStates.go:69`, `ServicesStates.go:83`, `StatefulSetsStates.go:159`, `PodsStates.go:89`). Use one generic `getOptional[T]` helper. The `List` callers can never get NotFound, so drop that branch there.
- Instance-index parsing is written three times (`StatefulSetsStates.go:171`, `PodsStates.go:125`, `StatefulSetWrappersSorting.go:56`), and two of them discard the parse error. Write one helper.
- `StatefulSetWrappers.Add` re-sorts on every insert; sort once when reading.
- Status writes use a full `Status().Update`; switch to `Status().Patch(MergeFrom)` to reduce conflicts.

---

## Phase 4: move validation and defaults into the CRD, polish the API

### 4.1 Validation markers and CEL (score 20)
Validation done at reconcile time today, which could be rejected at admission:
- `spec.image` and `spec.database.size` required; `replicas >= 1`; size quantity pattern;
- immutable `database.volumeMount` and `storageClassName` (`self == oldSelf`);
- reserved volume names (`!self.exists(v, v.name in [...])`);
- `KubegresBootstrapSource` required fields (no markers today). Also, the port check allows 0 while its message says 1 to 65535.

`validateCronTasks` repeats checks the CRD already enforces (`kubegres_types.go:73-100,186-187`). Keep the reconcile-time copy only while older CRDs may still be installed, then delete it.

### 4.2 Defaults via markers (score 18)
`UndefinedSpecValuesChecker` (it sets values, so it is a defaulter, not a checker) mutates the CR and calls `Client.Update` during reconcile. Move the static defaults (`port: 5432`, `database.volumeMount`, `customConfig`) to `+kubebuilder:default=` markers, as `KubegresLogicalBootstrap.TimeoutSeconds` already does. Keep only the dynamic defaults (storage class, affinity), and rename the type to `SpecDefaulter`.

### 4.3 API docs and naming (score 15)
- Many types and fields have no doc comment, so `kubectl explain` shows nothing: `KubegresDatabase`, `KubegresBackUp`, `KubegresFailover`, `KubegresScheduler`, `Volume`, most `KubegresSpec` fields and all status types.
- `maintenance_operation_types.go`: add docs, `MinLength` on `kubegresRef.name`, and `+listType=map`/`+listMapKey=type` on `Conditions`.
- `BootstrapState` and `Classification` are free strings; make them typed enums.
- Remove the stray `protobuf:` tag at `kubegres_types.go:150`.

### 4.4 Breaking renames (defer to a versioned API change)
Do not do these as part of this re-work. Record them here for the next API version only: the generic exported names `Volume`, `Probe`, `Lifecycle` and `VolumeClaimTemplate`; `TimeOutEpocInSeconds`; `OperationId` vs `OperationID`; `BackUp` vs `Backup`; and the sentence-style blocking operation IDs persisted in status.

---

## Phase 5: e2e test suite hygiene

Do this alongside the other phases. It mainly speeds up the slow 12-shard CI.

- **Remove sleeps inside `Eventually`.** 30 test files call `time.Sleep(TestRetryInterval)` (5 s), many inside an `Eventually` callback after it has already succeeded (for example `spec_resource_test.go:206,239`, `spec_backup_test.go:415`). Remove them, or use `Consistently` where a hold is intended.
- **Replace fixed sleeps with waits.** Swap the fixed 5 to 10 s sleeps between deletes (`primary_failure_and_recovery_test.go:192,217,233`, `suite_test.go:180`) for `Eventually(...NotFound)`.
- **Extract a shared fixture.** The `BeforeEach`/`AfterEach` block is repeated in 32 files, and `thenPodsStatesShouldBe` is copy-pasted in 19. Move both into `internal/test/util`.
- **Consolidate `TestResourceCreator` helpers.** It has seven near-identical `CreateConfigMap*` functions and two near-identical `CreateBackUpPvc*` (only the name differs); replace each set with one parameterized helper. `DeleteResource` should return the error, not a `bool`.
- **Use one source for shard assignment.** Shard labels are hardcoded in each `Describe` and duplicated in `hack/ci-shards.json`. Make the JSON the only source, and rebalance from the uploaded `report.json` timings.

---

## Wire in or park the newer packages

`internal/controller/maintenance` and `internal/controller/majorupgrade` are the
cleanest code in the repo: pure functions, typed enums, injected time, table
tests. Nothing outside their own tests imports them, so they are not wired into
the reconciler yet. Either finish wiring them in, or mark them clearly as
work in progress in `docs/`, so readers do not assume they are live. Their style
is the target style for the refactors above.

## Suggested PR order

1. Phase 0 bugs, one PR each with a regression unit test (done: #33 to #39).
1b. Phase 0b defects in score order (0b.1 failover preStop first), one PR each with a regression test.
2. 1.1 lint in CI ("new issues only"), 1.3 dead Makefile target.
3. 2.1 dead code, 2.5 logging noise (quick wins, no behaviour change).
4. 1.2 unit tests for `operation`, `comparator`, failover.
5. 2.2 constants and 2.4 package renames, then file renames on their own.
6. 3.2 generic field enforcer, 3.4 templates, 3.5 spec checker.
7. 3.3 decide and apply, 3.6 context slimming, 3.7 and 3.8.
8. 3.1 remove the reconcile sleep, behind a targeted e2e test.
9. Phase 4 CRD validation and defaults.
10. Phase 5 alongside, whenever a test file is touched.

## Definition of done for each refactor PR

- `go build ./...`, `go vet ./...`, `make test-fast` and lint all pass.
- No change to rendered StatefulSet/Service/CronJob specs unless intended. Compare the generated objects before and after for template refactors.
- No change to persisted status field names or values (see 4.4).
- The affected e2e shard is green.
