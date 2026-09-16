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

Do not enable this with only one database instance. If no Ready replica exists, Kubegres logs the existing no-replica failover condition and does not promote anything.

## Rollback

Set `onPrimaryPodDrain: false` to disable the automatic trigger. Existing manual promotion through `failover.promotePod` and existing crash failover behavior remain unchanged.
