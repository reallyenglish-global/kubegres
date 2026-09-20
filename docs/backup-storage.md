# Backup storage

Kubegres backup CronJobs support either a durable, user-managed PVC or a
per-Pod generic ephemeral PVC.

## Existing PVC

Set `spec.backup.pvcName` to use an existing PVC. Kubegres checks the PVC
before creating the CronJob and mounts it directly when it exists. The PVC is
not owned or deleted by Kubegres.

```yaml
spec:
  backup:
    schedule: "0 2 * * *"
    pvcName: postgres-backups
    volumeMount: /var/backups/postgres
```

## Temporary backup storage

If `pvcName` is omitted, or names a PVC that does not exist, set
`spec.backup.size`. Kubegres then uses a generic ephemeral PVC for each backup
Pod:

```yaml
spec:
  backup:
    schedule: "0 2 * * *"
    size: 20Gi
    volumeMount: /var/backups/postgres
```

If `pvcName` is supplied but is not present, the same `size` field is used to
provision the temporary claim. The claim is owned by the Pod. Kubegres sets the completed Job TTL to zero in
this mode, so the Job and Pod are removed promptly after completion and the
claim is cleaned up with the Pod. This requires a Kubernetes cluster that
supports generic ephemeral volumes and the TTL-after-finished controller.

Temporary storage is not a durable backup destination. The backup script must
copy or upload the dump to durable storage (for example, object storage) before
the Pod is removed. If the dump must remain available on a PVC, create that
PVC first and use `pvcName`.

`size` is required whenever Kubegres needs to provision temporary storage. If a
named PVC exists, its own capacity is used and `size` is ignored.
