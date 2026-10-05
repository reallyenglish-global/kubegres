# Kubegres safe node-maintenance improvement

## Why introduce this feature?

Kubegres already supports an opt-in `onPrimaryPodDrain` failover trigger. That feature is useful when Kubernetes is about to evict the primary Pod, but it is deliberately a last-moment safety net: it can select a Kubernetes-Ready replica and promote it, yet it does not own the complete node-upgrade sequence.

A node upgrade or planned node rotation needs a stronger guarantee:

> Move and verify the replica first; only then allow the primary to leave the node; fence and promote the verified replica; finally rebuild the former primary as a replica.

Without this ordering, a drain can remove the primary before a safe promotion candidate is available. Kubernetes readiness alone also does not prove that PostgreSQL is streaming, caught up, or safe to promote.

This improvement therefore introduces an explicit, controlled maintenance workflow rather than trying to infer operator intent from a generic Kubernetes eviction.

## The feature idea in one sentence

Add an opt-in, expiring `MaintenanceOperation` resource that gives Kubegres trusted intent and a durable state machine for replica-first node maintenance, with PostgreSQL health checks and fail-closed fencing before promotion.

## What problem does it solve?

The current system has three important limitations:

1. **Kubernetes does not identify the business reason for an eviction.**
   `EvictionByEvictionAPI` could mean a node upgrade, routine maintenance, a manual `kubectl drain`, or another API client. It must not automatically be labelled as an upgrade.
2. **The current failover decision is too infrastructure-focused.**
   A Pod may be Kubernetes-Ready while PostgreSQL replication is disconnected or materially behind.
3. **Replica relocation and normal reconciliation can race.**
   A planned operation needs to preserve the requested database count while it moves a replica. Normal replica-count reconciliation must not create an extra replica or interfere with the operation.

The feature addresses these limitations by making intent explicit, correlating it with observed Pod and Node evidence, and holding the normal topology reconciliation loop while the operation owns the sequence.

## Proposed user-facing contract

A caller creates a namespaced `MaintenanceOperation` for one Kubegres cluster and one immutable target Node UID:

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
```

The Node name is useful for diagnostics, but the UID is authoritative because a node name can be reused. The purpose and expiry are immutable. An expired operation must not authorize a primary drain.

The controller records phase, operation ID, classification, conditions, desired replica count, and bounded transition history in status. The record is intended to make the decision and evidence inspectable without placing credentials, SQL text, or customer data in Kubernetes status or events.

## Ordered behavior

The intended state machine is:

1. **Pending / accepted**
   - Validate the Kubegres reference, target Node UID, expiry, replica count, PDB, topology, and operation authority.
   - Require one primary and at least one candidate replica in a distinct failure domain.
   - Snapshot the observed Kubegres generation and desired replica count.
   - Do not disrupt the primary.

2. **RelocatingReplica**
   - Identify a replica on the target node.
   - Restart or evict only that replica so Kubernetes schedules its replacement away from the target node.
   - Keep the desired replica count unchanged. This is relocation/recreation of the existing replica, not permission to create an additional replica StatefulSet.
   - Defer ordinary scale and replica-count topology changes while the operation is active.

3. **AwaitingPrimaryDrain**
   - Confirm that the replacement replica is on a different node or failure domain.
   - Confirm Kubernetes readiness and PostgreSQL streaming from the current primary.
   - Confirm replay/replication lag is within the configured threshold, initially `<= 5` seconds, continuously for `60` seconds.
   - Abort before touching the primary if the candidate does not become safe within the `15` minute relocation deadline.

4. **Fencing**
   - Allow the planned primary disruption only after the replica gate succeeds.
   - Prove that the old primary is terminated or isolated and can no longer accept writes.
   - Prove that the writable Service/endpoints no longer expose the old primary.
   - If fencing cannot be proved, stop at `ManualIntervention`; do not expose a second writable primary.

5. **Promoting**
   - Promote the verified replica.
   - Wait for PostgreSQL role convergence and Kubegres Service/endpoints reconciliation.
   - Verify the application connection/retry path separately from infrastructure state.

6. **Rejoining**
   - Rebuild or reconfigure the former primary as a replica of the new primary.
   - Wait for streaming and catch-up, not merely a running Pod.

7. **Completed**
   - Record the final topology and outcome.
   - Release the maintenance lock and resume normal reconciliation.

Any contradictory or incomplete evidence produces a warning and freezes disruptive automation. Recovery requires an explicit human-approved resume or abort/close action; there is no automatic retry that could turn uncertainty into an unsafe promotion.

## Why use a separate `MaintenanceOperation`?

A separate resource is preferable to another boolean on `Kubegres` because it represents an individual, expiring operational action rather than a permanent cluster preference.

It provides:

- **Intent:** the controller knows whether the approved purpose is upgrade, maintenance, or rotation.
- **Scope:** one Kubegres cluster and one exact Node UID.
- **Expiry:** stale authorization cannot remain valid indefinitely.
- **Concurrency control:** only one active operation may own a Kubegres topology.
- **Auditability:** status history links observed evidence to each transition.
- **A safe human boundary:** an operator can inspect, resume, or close a stalled operation without editing the cluster's normal desired state.

The resource does not mean that its creation alone grants permission to drain a primary. It is an authorization and coordination signal that still requires all health and fencing gates to pass.

## Classification: intent plus evidence, not guesswork

Kubegres should classify events conservatively:

- `scale`: the desired replica count changed; reconcile the requested count without treating it as failover.
- `pod_delete`: a Pod disappeared without trusted maintenance intent; preserve the actor as unknown unless trusted audit data exists.
- `node_upgrade`: an active operation with `purpose=node-upgrade` matches the target Node and observed disruption.
- `node_maintenance` or `node_rotation`: the corresponding active purpose matches the observed disruption.
- `primary_failure`: evidence indicates an involuntary or unhealthy primary failure and no matching planned operation exists.
- `unknown`: evidence is incomplete or contradictory.

A generic eviction must remain `planned_voluntary_unknown` when no matching maintenance operation exists. This prevents Kubegres from claiming that a human or cloud provider performed a particular operation when Kubernetes cannot establish that fact.

## Relationship to `onPrimaryPodDrain`

`onPrimaryPodDrain` remains useful and backwards compatible:

```yaml
spec:
  replicas: 2
  failover:
    onPrimaryPodDrain: true
```

It continues to provide an opt-in response to recognised voluntary primary disruption. The new maintenance workflow is stronger and more deliberate:

- `onPrimaryPodDrain` is a failover safety net.
- `MaintenanceOperation` is the controlled, replica-first workflow for planned node operations.

The new controller must not weaken existing crash-failover behavior or silently reinterpret ordinary evictions. Where the controlled workflow cannot establish its preconditions, it must stop safely rather than fall back to an unverified planned promotion.

## Safety principles behind the design

### Data safety before availability automation

A fast promotion is not necessarily a safe promotion. Streaming state, replay lag, and old-primary fencing matter more than completing the drain automatically.

### Explicit intent before destructive coordination

Kubernetes disruption signals describe what happened to a Pod, not why a human or provider initiated it. A short-lived operation resource prevents the controller from inventing intent.

### Preserve cardinality

Replica relocation must not increase the requested database count. The operation owns the existing topology temporarily and blocks competing scale reconciliation.

### Fail closed on uncertainty

Unknown Node identity, stale status, failed lag checks, expired intent, unexpected second disruption, or unverified fencing must lead to an abort or manual-intervention state.

### Keep infrastructure and application verification separate

A promoted PostgreSQL Pod and updated Kubernetes Service do not prove that the application has reconnected successfully. The controller should report infrastructure convergence, while application-level retry and write-path checks remain an explicit verification step.

## Rollout and implementation phases

1. **API and contract**
   - Add the `MaintenanceOperation` API, validation, generated CRD/deepcopy artifacts, status conditions, and bounded history.
   - Preserve existing `Kubegres` compatibility.

2. **Evidence observation**
   - Add the minimum Pod and Node watches needed to correlate Pod → StatefulSet → Kubegres and target Node UID.
   - Add only RBAC required by the watches.
   - Record structured events without secrets or raw SQL.

3. **Operation lock and state machine**
   - Reuse Kubegres's blocking-operation architecture.
   - Defer scale and replica-count reconciliation while maintenance owns the topology.
   - Test relocation, timeout, expiry, abort, and no-extra-replica behavior.

4. **PostgreSQL health and fencing**
   - Inspect the repository's existing PostgreSQL scripts and supported versions before selecting probes or promotion commands.
   - Implement testable, timeout-bound streaming, lag, and fencing interfaces.
   - Stop at `ManualIntervention` if the architecture cannot prove that the old primary is no longer writable.

5. **Operational verification**
   - Add kind integration coverage for replica-first eviction and promotion ordering.
   - Test provider-specific node upgrade behavior separately in a non-production environment.
   - Publish a recovery runbook and metrics for lag-gate failures, aborts, promotion success, and operation duration.

## Non-goals

This feature does not:

- infer a cloud-provider upgrade from a generic eviction;
- replace Kubernetes node-drain or cloud upgrade tooling;
- guarantee protection from involuntary power or node failure through a PDB;
- treat Kubernetes `Ready` as proof of PostgreSQL replication safety;
- authorize production changes, cluster mutations, deployments, or merges by itself.

## Success criteria

The improvement is ready for review when it can demonstrate that:

- a controlled operation relocates and verifies the replica before primary disruption;
- the desired replica count remains stable and no extra replica StatefulSet is created;
- promotion is blocked when replication health or fencing is unverified;
- the former primary is rebuilt and caught up as a replica after promotion;
- generic evictions are not falsely labelled as upgrades;
- expiry, ambiguity, timeout, and conflicting scale changes stop safely;
- every terminal operation leaves bounded status evidence and structured events;
- existing `onPrimaryPodDrain` and crash-failover behavior remain covered by tests.

## Related documents

- [`primary-drain-failover.md`](primary-drain-failover.md) — current opt-in voluntary-drain safety net.
- [`pod-loss-classification-and-safe-node-maintenance.md`](pod-loss-classification-and-safe-node-maintenance.md) — detailed design, evidence model, state machine, and acceptance tests.
- [`handoff-maintenance-operation-v1.md`](handoff-maintenance-operation-v1.md) — implementation phases and local-only delivery boundaries.
