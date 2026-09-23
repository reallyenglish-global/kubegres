# Upgrading from reactive-tech/kubegres v1.18

This is for clusters currently running `reactive-tech/kubegres` at (or near) upstream tag `v1.18`, moving to
this fork. It does not cover upgrading between fork releases; see [`CHANGELOG.md`](../CHANGELOG.md) for
what changed release to release.

## CRD compatibility

The fork's CRD changes since `v1.18` are additive. Diffing `api/v1/kubegres_types.go` between `v1.18` and
the current `HEAD` shows no removed or renamed fields on `KubegresSpec`, `KubegresBackUp`, or
`KubegresFailover`; every new top-level field (`podDisruptionBudget`, `cronTasks`, `lifecycle`,
`probe.startupProbe`, `backup.archiveCommand`, `backup.restoreCommand`, `backup.size`, `backup.image`) is
optional and defaults to the upstream v1.18 behavior when left unset.

The one place with new required-at-admission fields is inside `spec.cronTasks[]` entries: each entry
requires `name`, `schedule`, and `image`, and its optional `script` block, if present, requires
`configMapName`, `key`, and `mountPath`. These constraints only apply if you add a `cronTasks` entry;
existing Kubegres manifests that do not set `spec.cronTasks` are unaffected.

## Upgrade steps

1. Complete the [pre-upgrade checklist](#pre-upgrade-checklist) below.
2. Apply the fork's manifest over the existing install. It contains the updated CRDs and the operator
   Deployment:

   ```
   kubectl apply -f https://raw.githubusercontent.com/reallyenglish-global/kubegres/main/kubegres.yaml
   ```

   or a specific release's `install.yaml` asset from the
   [Releases page](https://github.com/reallyenglish-global/kubegres/releases).
3. The operator image changes from `reactivetechio/kubegres` to `ghcr.io/reallyenglish-global/kubegres`.
   `kubectl apply` replaces the Deployment's image; watch the rollout with
   `kubectl -n <namespace> rollout status deployment/kubegres-controller-manager` (namespace and Deployment
   name per your install).
4. Existing `Kubegres` objects keep working unchanged. You do not need to edit them to adopt the new CRD.

No database Pods are restarted by the CRD or operator upgrade itself; the new operator reconciles existing
`Kubegres` objects against the same StatefulSets, Services, and PVCs it already manages.

## Backup validation change

The fork enforces that a `Kubegres` with a backup schedule set (`spec.backup.schedule` non-empty) also sets
either `spec.backup.pvcName` or `spec.backup.size`. If your existing manifests already set
`spec.backup.pvcName`, as v1.18 required, nothing changes for them. This only matters if you have a
scheduled backup with neither field set, which upstream v1.18 did not support either.

## MaintenanceOperation CRD

The fork's manifest installs a `MaintenanceOperation` CRD. It is currently a schema and API contract only;
no controller reconciles it yet, so creating a `MaintenanceOperation` object has no effect on your cluster.
See [`docs/handoff-maintenance-operation-v1.md`](handoff-maintenance-operation-v1.md) for the design. It is
safe to have this CRD installed even if you never use it.

## Rolling back

To roll back to upstream `reactive-tech/kubegres`, reapply the upstream v1.18 manifest. The fork-only spec
fields (`podDisruptionBudget`, `cronTasks`, `lifecycle`, `probe.startupProbe`, the new `backup.*` fields) are
ignored by the upstream operator, which reconciles only the fields it knows about; they are not deleted and
remain stored in etcd on the `Kubegres` objects, so re-upgrading to the fork later picks them back up. If you
used `spec.cronTasks` or `spec.podDisruptionBudget`, the CronJobs and PodDisruptionBudget the fork operator
created are not owned or cleaned up by the upstream operator; remove them manually if you no longer want
them after rollback.

## Pre-upgrade checklist

- [ ] Take a current backup of each database you plan to upgrade, independent of `spec.backup` (a manual
      `pg_dump` or your existing snapshot process).
- [ ] Record the current operator image tag: `kubectl -n <namespace> get deployment kubegres-controller-manager -o jsonpath='{.spec.template.spec.containers[0].image}'` (namespace and Deployment name per your install).
- [ ] List every `Kubegres` object you manage so you can confirm each is still healthy after the upgrade:
      `kubectl get kubegres -A`.
