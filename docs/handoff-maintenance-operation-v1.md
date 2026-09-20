# Handoff: MaintenanceOperation v1 for safe Kubegres node maintenance

## Purpose

This is a bounded implementation handoff for adding an opt-in `MaintenanceOperation` API and controller workflow to Kubegres. It implements the approved contract for classifying database Pod loss and carrying out a safe planned node upgrade/maintenance/rotation.

**Read order**

1. [`pod-loss-classification-and-safe-node-maintenance.md`](pod-loss-classification-and-safe-node-maintenance.md) — approved problem statement, decisions, API proposal, state machine, and acceptance tests.
2. [`primary-drain-failover.md`](primary-drain-failover.md) — existing `onPrimaryPodDrain` feature and compatibility contract.
3. The current source files listed in [Existing implementation and gaps](#existing-implementation-and-gaps).
4. Refresh Git state before modifying anything; this document is a snapshot, not live truth.

## Authorization boundary

Joey authorized documentation and a Luna implementation handoff. This is **not** authorization to:

- apply CRDs or manifests to any cluster;
- change production/QA configuration;
- drain, cordon, or upgrade nodes;
- commit, push, create a PR, merge, or deploy.

Implement in the local worktree only, preserve unrelated changes, run local tests, and report the diff plus exact observed results. Ask for separate approval before any external mutation.

## Approved contract (do not weaken without asking)

1. Add a separate, namespaced `MaintenanceOperation` CR. It references a Kubegres object and a target **Node UID**, has an immutable purpose and expiry, and owns operation status/history.
2. Classify `node_upgrade` / `node_maintenance` only when the Pod/Node evidence matches an active controlled `MaintenanceOperation`. A bare `EvictionByEvictionAPI` is `planned_voluntary_unknown`, not an upgrade.
3. Permit automated planned primary switchover only with one primary plus one healthy caught-up replica on a distinct node/failure domain.
4. Candidate gate: Kubernetes Ready; PostgreSQL streaming verified; replay lag **≤5 seconds** for **60 continuous seconds**; relocation/catch-up deadline **15 minutes**.
5. Ordered workflow: accept/preflight → relocate replica (no extra replica StatefulSet) → verify new location/catch-up → authorize primary drain → prove old-primary fencing and writable endpoint removal → promote → rejoin old primary as replica → complete.
6. Fencing is mandatory. If old-primary termination/isolation and writable Service-endpoint removal cannot both be verified, abort before exposing a new writable primary.
7. Unknown/contradictory evidence: record `unknown`, emit a warning, freeze disruptive automatic recovery, require explicit human-approved `Resume` or `Abort/Close`. No automatic retry or generic-failover fallback.
8. While a maintenance operation is active, defer/reject scale reconciliation and record `ScaleDeferred`; resume only after completion, close, or expiry.
9. Only the approved upgrade-automation service account may create/cancel operations. The approved human workflow must preserve the original requester/approval identity as audit metadata rather than losing it behind service-account impersonation.

## Current LIVE_OS snapshot — refresh before coding

- Repository/worktree: `/mnt/data/re/kubegres-crt-022`
- Branch: `chore/upgrade-controller-runtime-022`
- Local HEAD: `38caf70` (`chore: upgrade controller runtime to v0.22`)
- Tracking state when inspected: **behind `origin/chore/upgrade-controller-runtime-022` by 2 commits**; remote reports `485b49c chore: upgrade controller runtime to v0.23`.
- Existing unrelated dirty files — **do not overwrite, stage, or revert them**:
  - `api/v1/kubegres_types.go`
  - `config/crd/bases/kubegres.reactive-tech.io_kubegres.yaml`
  - `internal/controller/spec/enforcer/resources_count_spec/BackUpCronJobCountSpecEnforcer.go`
  - `internal/controller/spec/template/ResourcesCreatorFromTemplate.go`
  - `internal/test/spec_backup_test.go`
- Documentation created in this handoff:
  - `docs/pod-loss-classification-and-safe-node-maintenance.md`
  - `docs/handoff-maintenance-operation-v1.md`

Run these first and report their literal output before editing:

```bash
cd /mnt/data/re/kubegres-crt-022
git status --short
git branch --show-current
git rev-parse --short HEAD
git log --oneline --left-right --cherry-pick HEAD...origin/chore/upgrade-controller-runtime-022
git diff -- api/v1/kubegres_types.go config/crd/bases/kubegres.reactive-tech.io_kubegres.yaml internal/controller/spec/enforcer/resources_count_spec/BackUpCronJobCountSpecEnforcer.go internal/controller/spec/template/ResourcesCreatorFromTemplate.go internal/test/spec_backup_test.go
```

If the worktree state changed, stop treating the snapshot above as authoritative and preserve the actual changes. Do not pull/rebase/checkout/reset without Joey's approval, because that can disrupt in-progress local work.

## Existing implementation and gaps

The prior feature was committed as `6e103bb Support failover before voluntary primary Pod disruption (#5)`.

- `api/v1/kubegres_types.go` currently has `spec.failover.onPrimaryPodDrain`.
- `internal/controller/spec/enforcer/resources_count_spec/statefulset/failover/PrimaryToReplicaFailOver.go` only treats a primary as a voluntary-drain candidate when it has `DeletionTimestamp`, `DisruptionTarget=True`, and reason `EvictionByEvictionAPI` or `PreemptionByScheduler`.
- It selects a Kubernetes-Ready replica and uses the existing failover path; it does not relocate a replica first, prove PostgreSQL catch-up, or prove fencing.
- `internal/controller/kubegres_controller.go` currently watches `Kubegres`, owned `StatefulSet`, and owned `Service`; it has Pod RBAC but no Pod or Node watch.
- `internal/controller/ctx/log/LogWrapper.go` can emit structured logs and Kubernetes Events attached to the Kubegres CR. The current Kubegres status has no bounded loss-operation history.
- `ReplicaDbCountSpecEnforcer.go` owns normal replica-count convergence and must not race a maintenance operation.

## Implementation boundaries and suggested phases

### Phase A — API, schema, status, and pure classifier

- Define the new Go API type(s), status/condition/history shape, kubebuilder markers, and generated CRD/deepcopy artifacts.
- Decide after inspecting the project conventions whether the new CR is `v1alpha1` in the existing API group or needs an explicit group/version evolution. Preserve existing Kubegres v1 compatibility.
- Implement a pure, table-tested classifier returning at least class, source, evidence, confidence/outcome, and selected policy.
- Include `scale`, `pod_delete`, `node_upgrade`, `node_maintenance`/`node_rotation`, `primary_failure`, `planned_voluntary_unknown`, and `unknown` without claiming actor identity when no trusted evidence exists.
- Keep bounded status history; do not place secrets, SQL, connection strings, or customer data in logs/events/status.

**Phase-A gate:** generated artifacts are current; unit tests cover all classification branches and proof requirements; old `onPrimaryPodDrain` behavior remains backward-compatible when no `MaintenanceOperation` exists.

### Phase B — observation, correlation, and operation lock

- Add the smallest safe Pod and Node watch/correlation design. Map Pod → StatefulSet → Kubegres reliably; do not assume a Pod is directly owned by Kubegres.
- Add only the RBAC required by actual watches.
- Correlate Pod UID, Node UID, deletion timestamp, disruption condition/reason, terminal container state, node conditions/taints, Kubegres generation, and operation ID.
- Create consistent Kubernetes Event reasons and structured keys described in the design document.
- Enforce one active operation per Kubegres and defer scale as `ScaleDeferred` while it is active.

**Phase-B gate:** unit tests prove owner mapping, matching Node UID, expiry, one-active-operation, scale deferral, audit/status/event recording, and unknown cause handling.

### Phase C — maintenance blocking state machine

- Use the existing blocking-operation architecture rather than a parallel untracked loop.
- Add phases: `Pending`, `RelocatingReplica`, `AwaitingPrimaryDrain`, `Fencing`, `Promoting`, `Rejoining`, `Completed`, `Aborted`, `Closed`, `ManualIntervention`.
- Ensure replica relocation preserves desired cardinality: recreate/relocate the selected replica, but never create an extra replica StatefulSet.
- Block normal replica-count enforcement while the operation owns topology.
- Enforce the 5-second/60-second/15-minute gates as configurable defaults, with validation and explicit timeout outcomes.

**Phase-C gate:** focused tests prove transition order, no extra replica, aborted relocation leaves primary untouched, scale is deferred, and operation expiry prevents a new primary drain.

### Phase D — PostgreSQL health and fencing integration

- Inspect Kubegres's actual PostgreSQL scripts/configuration before choosing commands; do not invent a query or version assumption.
- Implement/abstract streaming and replay-lag checks and old-primary write fencing with testable interfaces/timeouts.
- Require old-primary isolation/termination and writable endpoint removal before promotion.
- If the project cannot safely prove a fence with its present architecture, stop at an explicit `ManualIntervention` boundary rather than simulating success.

**Phase-D gate:** tests prove no promotion on unverified fence, stale/catching-up replica, timeout, or contradictory evidence; a healthy controlled path promotes then rejoins the former primary.

## Required verification

At minimum, run focused tests as each phase is implemented, then:

```bash
cd /mnt/data/re/kubegres-crt-022
mise exec go -- go test ./...
git diff --check
```

If integration tests require Kind, first inspect their documented prerequisites. In this repository, ensure Go and Kind are in the same mise process environment, for example:

```bash
mise exec kind go -- bash -lc 'go test ./internal/test'
```

Do not claim integration success unless the cluster harness actually completed. Separate missing local prerequisites from implementation failures.

## Delivery acceptance criteria

The local implementation is ready for Joey's review only when it has all of the following:

- generated CRD/API artifacts for the new public contract;
- no regression to existing `onPrimaryPodDrain` opt-in behavior;
- evidence-backed classification for all required classes, with `unknown` as safe default;
- no unsupported inference that an eviction was a GKE upgrade;
- status-backed bounded history plus structured Events/logs with no sensitive data;
- operation lock, scale deferral, expiry, explicit human recovery boundary;
- ordered replica-first relocation and no extra replica creation;
- health-gate and fencing fail-closed behavior;
- focused tests, broad Go tests when runnable, and `git diff --check` results;
- concise report listing changed files, commands/results, caveats, and no commit/push/deploy.

## Handoff prompt for Luna

Copy the block below exactly into Luna. It deliberately authorizes only local, reviewable implementation.

```text
You are implementing a bounded, local-only Kubegres feature. Read these two documents first, in order:

1. /mnt/data/re/kubegres-crt-022/docs/pod-loss-classification-and-safe-node-maintenance.md
2. /mnt/data/re/kubegres-crt-022/docs/handoff-maintenance-operation-v1.md

Goal: implement the approved MaintenanceOperation v1 contract for safe Kubegres node-upgrade/maintenance/rotation handling and evidence-backed Pod-loss classification.

Before editing, refresh LIVE_OS exactly:
cd /mnt/data/re/kubegres-crt-022
git status --short
git branch --show-current
git rev-parse --short HEAD
git log --oneline --left-right --cherry-pick HEAD...origin/chore/upgrade-controller-runtime-022
git diff -- api/v1/kubegres_types.go config/crd/bases/kubegres.reactive-tech.io_kubegres.yaml internal/controller/spec/enforcer/resources_count_spec/BackUpCronJobCountSpecEnforcer.go internal/controller/spec/template/ResourcesCreatorFromTemplate.go internal/test/spec_backup_test.go

Treat the documents as WORK_OS and Git/source/tests as LIVE_OS. Preserve all unrelated dirty work. Do not pull, rebase, reset, checkout, commit, push, create a PR, apply manifests, access a cluster, cordon/drain nodes, or deploy.

Implement in small phases with tests:
A. API/CRD/status and a pure table-tested classifier.
B. Safe Pod/Node observation and correlation, event/log/status audit records, one-active-operation lock, scale deferral.
C. Blocking maintenance state machine: relocate the replica first without increasing desired replica count; prove its location and catch-up; only then authorize primary drain.
D. Fail-closed PostgreSQL health and fencing interfaces/integration. Never expose a promoted primary unless old-primary isolation/termination and writable endpoint removal are proved.

Non-negotiable contract:
- Separate namespaced MaintenanceOperation CR, target Node UID, purpose, expiry, bounded status history.
- Classify upgrade/maintenance only with matching controlled intent; bare EvictionByEvictionAPI is planned_voluntary_unknown.
- Automated switchover needs a primary + caught-up replica in a distinct failure domain.
- Gate: Ready + streaming + <=5s replay lag for 60 seconds; relocation deadline 15 minutes.
- Unknown/contradictory evidence: warning + frozen disruptive automation + explicit Resume or Abort/Close; no automatic generic-failover fallback.
- Scale changes are deferred/rejected and recorded while operation is active.
- Do not invent PostgreSQL commands or fencing guarantees: inspect current scripts/config and stop at ManualIntervention if safe proof is unavailable.

Run focused tests after each coherent phase and finish with:
mise exec go -- go test ./...
git diff --check

If Kind integration is appropriate and prerequisites exist, use:
mise exec kind go -- bash -lc 'go test ./internal/test'

Report: refreshed Git state, files changed, decisions made, exact test results, remaining gaps, and any condition that requires Joey's approval. Do not claim a feature is complete merely because it compiles.
```
