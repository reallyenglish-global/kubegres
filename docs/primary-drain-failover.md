# Primary failover on voluntary node disruption

Kubegres can optionally promote a Ready replica when Kubernetes marks the primary Pod for voluntary disruption, such as a node-drain eviction.

Enable it explicitly:

```yaml
spec:
  replicas: 2
  failover:
    onPrimaryPodDrain: true
```

## Behavior

1. A replica must already be deployed and Ready.
2. Kubernetes marks the primary Pod with a deletion timestamp and `DisruptionTarget` condition.
3. Kubegres uses its existing failover state machine to select and promote a Ready replica.
4. The primary Service is updated by the existing role/service reconciliation.
5. The old primary StatefulSet is rebuilt as a replica by the existing replica-count reconciliation.

The feature is opt-in and does not treat ordinary Pod deletion as a drain. It recognizes the Kubernetes voluntary-disruption reasons `EvictionByEvictionAPI` and `PreemptionByScheduler`.

## Upgrade-controller contract

The external node-upgrade controller must still move and verify the replica before allowing the primary node to be disrupted. `onPrimaryPodDrain` is a database-side safety net and failover trigger; it is not a replacement for checking replication lag, fencing the old primary, or verifying application reconnects.

Kubegres does not initiate node drain or restart a replica as part of this feature. The external controller owns that sequence and must wait for the replica to become Ready and streaming before requesting or permitting the primary disruption.

Do not enable this with only one database instance. If no Ready replica exists, Kubegres logs the existing no-replica failover condition and does not promote anything.

## PDB and ordered migration

Enable both features together:

```yaml
spec:
  replicas: 2
  failover:
    onPrimaryPodDrain: true
  podDisruptionBudget:
    enabled: true
    minAvailable: 1
```

Kubegres creates one owner-managed PDB selecting both `primary` and `replica` Pods. This permits one database Pod to be evicted while ensuring a second Pod remains available; separate PDBs per role would incorrectly block replica migration. `onPrimaryPodDrain` reacts only to the Kubernetes `DisruptionTarget` condition with eviction/preemption reasons. It does not react to arbitrary Pod deletions.

The sequence is: move/restart the replica and wait for it to become Ready and streaming; allow drain of the primary; promote the verified replica; then let Kubegres reconstruct the former primary as a replica. The old primary must not remain writable during promotion.

## Rollback

Set `onPrimaryPodDrain: false` to disable the automatic trigger. Existing manual promotion through `failover.promotePod` and existing crash failover behavior remain unchanged.
