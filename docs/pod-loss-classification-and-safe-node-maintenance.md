# Pod-loss classification and safe node maintenance: gap assessment

## Goal and scope

Kubegres must tell *why* a database Pod is being lost so that it can use a different safety policy and emit a durable, queryable operational record. This document assesses the current `onPrimaryPodDrain` implementation and defines the work needed for these cases:

1. an intentional scale change to `spec.replicas`;
2. direct deletion of a Pod;
3. a node upgrade;
4. planned node maintenance or rotation; and
5. an unplanned primary failure.

This is an implementation design and test plan. It does **not** authorize applying it to a cluster or changing an existing CRD.

## Current implementation (verified in this checkout)

The `onPrimaryPodDrain` feature is opt-in. It recognises a primary Pod only when it has both a deletion timestamp and a `DisruptionTarget=True` condition whose reason is `EvictionByEvictionAPI` or `PreemptionByScheduler`.

```yaml
spec:
  replicas: 2
  failover:
    onPrimaryPodDrain: true
  podDisruptionBudget:
    enabled: true
    minAvailable: 1
```

The current controller has these relevant properties:

- `PrimaryToReplicaFailOver.ShouldWeFailOver()` selects a Ready replica and has a `PrimaryPodDrainFailover` Kubernetes Event for the above voluntary-disruption subset.
- The controller watches `Kubegres`, owned `StatefulSet`, and owned `Service` resources. It has Pod RBAC, but it does **not** currently watch Pods, Nodes, Events, or a maintenance/upgrade intent resource.
- A failover is selected using Kubernetes readiness only. It does not verify PostgreSQL streaming state, replay/flush LSN, replication lag, synchronous-commit eligibility, or a fencing result before promotion.
- When the primary drain condition is observed, the existing path deletes the primary StatefulSet, waits ten seconds, and then promotes the selected replica. It does not first orchestrate migration of a replica off the draining node.
- Replica-count reconciliation later creates missing replica StatefulSets. It has no maintenance-operation state that prevents normal replacement logic from racing with a planned sequence.
- `InfoEvent`, `WarningEvent`, and `ErrorEvent` write a controller log entry and a Kubernetes Event attached to the `Kubegres` CR. The CR status contains only blocking-operation and replica-count data; it has no loss classification or bounded event history.

Consequently, the current feature is a useful *last-moment primary-drain safety net*, but it is not yet a complete node-upgrade controller or a root-cause classifier.

## What Kubernetes can and cannot identify

Kubernetes classifies disruptions, not business intent. The stable `PodDisruptionTarget` condition (Kubernetes 1.31+) can carry these reasons:

- `EvictionByEvictionAPI` — for example `kubectl drain`, GKE drain/upgrade tooling, or another API client;
- `PreemptionByScheduler`;
- `DeletionByTaintManager` — typically a `NoExecute` taint such as an unreachable/out-of-service node;
- `DeletionByPodGC`;
- `TerminationByKubelet` — such as graceful node shutdown.

`EvictionByEvictionAPI` alone cannot distinguish **node upgrade**, **node rotation**, **routine maintenance**, or a human running `kubectl drain`. A direct `kubectl delete pod` normally has no unique `DisruptionTarget` reason that identifies the actor. Once the Pod has disappeared, the operator cannot reliably reconstruct the user or command from the Pod object.

Therefore, a trustworthy distinction between “upgrade” and “maintenance/rotation” needs a supplied intent signal, such as a short-lived, owner-referenced maintenance CR, an authenticated admission annotation, or cloud/provider audit data correlated by an external upgrade controller. It must not be inferred from a generic eviction.

A PodDisruptionBudget limits simultaneous **voluntary** disruptions only. It neither prevents node/power failure nor establishes PostgreSQL replication health. Kubernetes documentation also recommends the Eviction API for node drains; tools that delete Pods directly can bypass the PDB protection.

## Required classification contract

Use a conservative classification. When sufficient evidence is absent, record `unknown` rather than guessing `primary_failure`.

| Class | Required evidence | Automated policy |
| --- | --- | --- |
| `scale` | `Kubegres.spec.replicas` generation changed, with old and new desired counts recorded before reconciling | Reconcile only the requested cardinality. Never promote merely because scale-down/scale-up removed a Pod. Validate that scaling cannot select or remove the sole healthy primary. |
| `pod_delete` | A watched Pod has a deletion timestamp without a recognised disruption condition, and no active Kubegres maintenance operation; actor is `unknown` unless audit/admission provides it | Do not call it a node incident. For a replica, recreate/rejoin under normal policy. For a primary, use the incident safety path only after fencing and a verified candidate. Emit a warning that direct deletion bypasses the planned-maintenance protocol. |
| `node_upgrade` | An active maintenance operation with `purpose=node-upgrade`, target node UID/name, expiry, and a matching Pod/node/eviction observation | Run the ordered maintenance state machine below. |
| `node_maintenance` | The same active operation but `purpose=node-maintenance` or `node-rotation` | Use the identical safe state machine; preserve the purpose in events/audit. |
| `primary_failure` | No matching planned operation, and evidence of an involuntary/unhealthy primary: Node NotReady/Unreachable, termination/taint condition, terminal Pod/container failure, or sustained readiness failure after a configurable grace period | Freeze competing maintenance/scale operations, fence the old primary, require a verified replica, then controlled failover. Escalate instead of promoting if fencing or data-safety checks fail. |
| `unknown` | Evidence is incomplete or contradictory | Emit warning and take no destructive action beyond existing safe reconciliation. Escalate for operator decision; never silently classify it as an incident. |

The value recorded as “cause” must be an evidence-backed classification, not an assertion about the human actor. `actor` and `source` should be separate optional fields.

## Safe node-upgrade / maintenance state machine

The requested ordering is sound, with one correction: moving a replica normally means Kubernetes recreates the **same** replica StatefulSet and Pod identity after eviction; it must not create an extra replica StatefulSet. The desired replica count remains unchanged throughout the operation.

Preconditions:

- at least two database instances, with primary and at least one replica on distinct failure domains;
- a PDB that allows one voluntary disruption while retaining a healthy database Pod;
- topology spread / anti-affinity sufficient for the replacement replica to avoid the target node;
- a maintenance intent naming the target node and expiring automatically;
- a PostgreSQL health gate stronger than Kubernetes `Ready`: streaming connected, lag/replay threshold met, and the candidate is eligible for promotion;
- a fencing plan that prevents the old primary from accepting writes before promotion (for example, terminate/isolated old primary plus service endpoint/connection checks).

State machine:

1. **`MaintenanceAccepted`** — validate the intent, desired count, target node, PDB, distinct nodes, candidate replica, and health/lag gate. Write the operation ID and source evidence to status; do not drain anything yet.
2. **`ReplicaEvictionRequested`** — select a replica on the target node. Permit its eviction/restart only; keep the requested replica count unchanged and block ordinary replica-count repair from creating an additional replica StatefulSet.
3. **`ReplicaRelocatedAndCaughtUp`** — wait until the replacement Pod is scheduled away from the target node, Kubernetes Ready, streaming from the current primary, and within the configured replay/lag threshold for a stable observation period. Timeout is a hard stop: leave the primary untouched.
4. **`PrimaryDrainAuthorized`** — only after step 3 succeeds, let the node drain evict the primary. The PDB should prevent a second voluntary database disruption during this window.
5. **`PrimaryFenced`** — confirm the old primary cannot serve writes. This must occur before the candidate is exposed as primary; a simple `Ready` check is not fencing.
6. **`ReplicaPromoted`** — promote the verified replica, wait for its PostgreSQL role and Kubernetes Service/endpoints to converge, and verify an application connection/retry path.
7. **`FormerPrimaryRejoined`** — when the old primary is safely gone or returns, rebuild/reconfigure it as a replica following the new primary. Wait for streaming/catch-up, not only a running container.
8. **`Completed`** — release the maintenance lock and let normal reconciliation resume. Record final topology and elapsed durations.

Abort conditions: no suitable replica, failed PDB preflight, replica not relocated/caught up before deadline, unknown primary writability, promotion timeout, or an unexpected second disruption. On abort, emit a warning/error event and retain the operation record for manual recovery; never continue with an unverified primary promotion.

For a provider-managed upgrade, the external upgrade/drain controller must pause between steps 3 and 4. A controller seeing the primary only after an eviction has begun is too late to guarantee “replica first”. `onPrimaryPodDrain` remains valuable as a fallback, but it cannot replace that coordination.

## Observability and audit design

Add a bounded, status-backed operation record and consistent structured logs/Kubernetes Events. Kubernetes Events are short-lived and can be aggregated, so they are notification evidence, not the sole audit trail.

Suggested API shape (names are proposal only):

```yaml
status:
  podLossOperations:
  - id: "uuid"
    observedAt: "RFC3339"
    class: node_upgrade        # scale|pod_delete|node_upgrade|node_maintenance|primary_failure|unknown
    source: eviction_api       # spec_change|pod_watch|node_watch|event_watch|external_maintenance
    actor: null                # populated only by trusted admission/audit integration
    pod:
      name: db-1-0
      uid: "..."
      role: replica
      node: gke-pool-a-123
    evidence:
      disruptionReason: EvictionByEvictionAPI
      kubegresGeneration: 42
      nodeUID: "..."
      maintenanceOperationID: "..."
    state: ReplicaRelocatedAndCaughtUp
    outcome: pending           # completed|aborted|manual_intervention|required
```

Keep the history bounded (for example the latest 20 records) and put full retention in the cluster logging backend. Every transition should include the same keys in controller logs and the Kubernetes Event message:

- `operation_id`, `class`, `source`, `state`, `outcome`;
- `kubegres`, namespace, Pod name/UID/role, StatefulSet, node name/UID;
- CR generation, old/new replica count when applicable;
- `disruption_reason`, maintenance purpose and expiry when present;
- selected candidate, observed PostgreSQL lag/LSN result, fencing result, and deadline.

Recommended event reasons:

- `PodLossClassified`
- `MaintenanceAccepted`
- `ReplicaRelocationStarted`
- `ReplicaRelocationReady`
- `PrimaryDrainAuthorized`
- `PrimaryFenced`
- `FailoverPromoted`
- `FormerPrimaryRejoined`
- `MaintenanceAborted`
- `PodLossUnknown`

Do not log database credentials, connection strings, SQL text, or customer data.

## Implementation gaps and work items

1. **Intent API and status** — add an opt-in maintenance-operation CR/field with purpose, target node UID, expiry, and a controller-owned status state. Regenerate the Kubegres CRD and deepcopy artifacts. Define conflict rules: a scale change or another maintenance operation is rejected/deferred while one is active.
2. **Watch and correlate evidence** — add Pod and Node watches plus explicit mapping from a Pod → owning StatefulSet → owning Kubegres CR. Observe Pod UID, deletion timestamp, `DisruptionTarget`, container termination, and Node conditions/taints. RBAC must include Nodes if watched. Do not assume an Event watch is a durable audit source.
3. **Capture direct-delete intent early** — Kubernetes audit logging or a validating/mutating admission integration is needed if the actor/command matters. Without it, classify only `pod_delete` with `actor=unknown`.
4. **Separate generic failure from planned disruption** — replace the current boolean drain check with a classifier that returns class, source, evidence, and confidence. `DeletionByTaintManager` and `TerminationByKubelet` must not be silently lumped into generic primary failure; correlate them with the active maintenance operation and node state.
5. **Maintenance state machine** — implement the eight states above as a blocking operation. Hold replica-count reconciliation while the operation owns topology, so the system cannot spawn an extra replica or race the relocation.
6. **PostgreSQL safety gates and fencing** — introduce pluggable, timeout-bound checks for streaming/catch-up and old-primary writability. Kubernetes readiness alone is not sufficient for promotion. Decide the supported PostgreSQL versions and synchronous/asynchronous replication policy before defining lag limits.
7. **Scheduling and PDB policy** — add first-class templates/fields for required anti-affinity or topology spread, PDB `minAvailable`, and (where supported) unhealthy-pod eviction policy. Validate unschedulable combinations at preflight.
8. **Events/log/status tests** — test every class, state transition, timeout/abort, and status/event record. Add integration tests with kind for Pod eviction and node drain; provider-specific GKE upgrade/rotation behaviour requires a separate non-production test environment.
9. **Runbook and metrics** — publish supported `kubectl`/provider workflows, a recovery runbook, and metrics for classification count, transition duration, aborts, promotion success, and lag-gate failures.

## Acceptance tests

The implementation is not complete until these tests pass:

- changing `spec.replicas` records `scale`, reconciles the exact requested count, and causes no primary promotion;
- deleting a replica Pod records `pod_delete` with `actor=unknown` unless trusted audit data is supplied, then restores the same desired count without creating an extra StatefulSet;
- deleting a primary Pod records `pod_delete`, fences before promotion, and either promotes a verified replica or stops safely;
- a labelled/test maintenance operation evicts a replica first, proves it is on a non-target node and caught up, then permits primary eviction, promotes it, and re-joins the former primary as a replica;
- `EvictionByEvictionAPI` without trusted maintenance intent is recorded as planned-voluntary-but-`unknown` purpose, not falsely as an upgrade;
- Node NotReady/Unreachable, `DeletionByTaintManager`, `TerminationByKubelet`, and container/OOM failure paths are separately recorded and never bypass fencing/health gates;
- PDB prevents two voluntary database Pod evictions at once; an involuntary failure is correctly documented as not preventable by PDB;
- all terminal operations produce one status record and structured Event/log transitions, while sensitive data is absent.

## Evidence consulted

- Current checkout: `internal/controller/spec/enforcer/resources_count_spec/statefulset/failover/PrimaryToReplicaFailOver.go`, `ReplicaDbCountSpecEnforcer.go`, `kubegres_controller.go`, and `internal/controller/ctx/log/LogWrapper.go`.
- Current checkout: `docs/primary-drain-failover.md` and commit `6e103bb` (`Support failover before voluntary primary Pod disruption`).
- Kubernetes documentation: [Disruptions](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/), [Pod conditions](https://kubernetes.io/docs/concepts/workloads/pods/pod-condition/), and [Node shutdowns](https://kubernetes.io/docs/concepts/cluster-administration/node-shutdown/).

## Contract decisions confirmed

The following decisions were selected for the first implementation contract:

1. **Maintenance declaration:** a separate namespaced `MaintenanceOperation` CR. It names the Kubegres cluster and target **node UID**, has a purpose, expiry, status/state machine, and creates the durable audit record.
2. **Classification evidence:** only a matching controlled `MaintenanceOperation` may classify an eviction as `node_upgrade` or `node_maintenance`. `EvictionByEvictionAPI` without it is recorded as `planned_voluntary_unknown`.
3. **Promotion gate:** the candidate must be Kubernetes Ready, streaming from the primary, and within a configured replay-lag threshold for a configured stable observation window. Any failed or timed-out check aborts the operation before primary disruption.
4. **Fencing:** before exposing a promoted primary, require confirmed old-primary termination or isolation **and** removal from writable Service endpoints. Failure to prove fencing aborts promotion.
5. **Ambiguous evidence:** record `unknown`, emit a warning, freeze disruptive automated recovery, and require a human decision after a bounded safety check.

These decisions favour data safety and accurate auditability over completing a maintenance operation automatically.

## Operational defaults and authority boundaries confirmed

6. **Authority:** only the dedicated Kubernetes service account used by approved upgrade automation may create or cancel `MaintenanceOperation` resources. A human must use a separately approved CLI/workflow that authenticates as that service account; ordinary namespace-admin or Kubegres-patch permission is insufficient.
7. **Minimum topology:** automated planned switchover requires one primary plus at least one healthy, caught-up replica in a distinct node/failure domain. Otherwise the operation aborts before primary drain.
8. **Initial safety thresholds:** candidate replay lag must be **≤ 5 seconds** and the candidate must continuously pass readiness/streaming/lag checks for **60 seconds**. Replica relocation and catch-up have a hard **15-minute** deadline.
9. **Concurrent scale:** while an operation is active, scale reconciliation is deferred/rejected and recorded as `ScaleDeferred`. It resumes only after completion, explicit close, or operation expiry.
10. **Manual recovery:** after an abort or unknown disruption, retain the operation lock and evidence. An explicit human-approved **Resume** or **Abort/Close** action is required; there is no automatic retry or fallback to generic failover.

## Proposed first-version API contract

```yaml
apiVersion: kubegres.reactive-tech.io/v1alpha1
kind: MaintenanceOperation
metadata:
  name: orders-db-node-upgrade-20260918
  namespace: database
spec:
  kubegresRef:
    name: orders-db
  purpose: node-upgrade # node-upgrade | node-maintenance | node-rotation
  targetNode:
    name: gke-db-pool-abc
    uid: "required-immutable-node-uid"
  expiresAt: "2026-09-18T23:00:00Z"
  safety:
    maxReplayLagSeconds: 5
    stableForSeconds: 60
    relocationDeadlineSeconds: 900
status:
  phase: Pending # Pending | RelocatingReplica | AwaitingPrimaryDrain | Fencing | Promoting | Rejoining | Completed | Aborted | Closed | ManualIntervention
  operationID: "uuid"
  classification: node_upgrade
  conditions: []
  history: [] # bounded, structured operation/evidence records
```

### Validation and lifecycle rules

- The target Node UID is mandatory and immutable after creation. The Node name is diagnostic only; names can be reused.
- `expiresAt` is mandatory. The controller must refuse a new primary drain after expiry, transition the operation to `ManualIntervention`, and retain its evidence.
- The CR is accepted only from the dedicated upgrade-automation identity. RBAC alone is not an audit statement; admission policy or the approved CLI must enforce this identity rule.
- Only one active `MaintenanceOperation` may reference a Kubegres cluster. Active means any non-terminal phase; `Completed` and explicitly `Closed` records remain as audit history.
- The controller snapshots the referenced Kubegres generation and desired replica count at acceptance. A changed generation creates `ScaleDeferred` and blocks topology mutation until the operation is terminal.
- The controller rejects a request if it cannot find one primary and one distinct-node candidate replica that pass the selected health gate.
- `Resume` is allowed only after new evidence satisfies the preflight again. `Abort/Close` releases the maintenance lock but never fabricates a successful outcome.

## Implementation start point

For the bounded Luna implementation scope, live-worktree snapshot, verification gates, and copyable continuation prompt, read [the implementation handoff](handoff-maintenance-operation-v1.md). Implement the CRD/status schema, identity/admission boundary, and a unit-tested classifier first. Then add the replica-relocation blocking state and PostgreSQL/fencing gates before allowing this API to authorize primary drain. Do not treat creation of the CR alone as permission to evict a primary.
