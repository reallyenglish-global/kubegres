# PITR WAL archive delivery

Kubegres can configure PostgreSQL WAL archiving with a custom command and an archive-specific Pod ServiceAccount:

```yaml
spec:
  backup:
    archiveCommand: "/usr/local/bin/kubegres-wal-archive %p %f"
    serviceAccountName: kubegres-wal-archive
```

`archiveCommand` is passed as PostgreSQL `archive_command` and `archive_mode=on` on every database StatefulSet. The executable must be present in the selected PostgreSQL image. PostgreSQL invokes the command for completed WAL segments; the command must return success only after the segment is durably stored and must be safe to retry.[7]

The archive-specific ServiceAccount is used when `spec.serviceAccountName` is empty. An explicit database `spec.serviceAccountName` takes precedence. On GKE, bind this Kubernetes identity to a narrowly scoped Google service account using Workload Identity Federation; grant write access only to the intended bucket prefix.[8][9]

## Sidecar boundary

This first slice does not add an asynchronous uploader sidecar. A sidecar may be added later, but `archive_command` must copy to a durable spool and wait for a verified upload acknowledgement before returning success. An `emptyDir` queue or fire-and-forget upload is unsafe because PostgreSQL may recycle the WAL segment after a successful archive command.

A future sidecar design must define a durable spool PVC, atomic file handoff, checksum/size acknowledgement, retry and backpressure behavior, restart recovery, retention/cleanup, and metrics for oldest pending WAL. It must be tested with uploader outage and PostgreSQL restart scenarios.

## Object-storage contract

The archive helper or future sidecar owns provider protocol details. The Kubegres operator only supplies the command, Pod identity, and PostgreSQL configuration. The helper must provide:

- immutable object keys derived from the WAL filename;
- no overwrite of a different object with the same key;
- checksum and size verification;
- bounded network timeout and retry;
- redacted errors and no credential logging;
- a non-zero exit on authentication, permission, quota, or durability failure.

## Acceptance gates

- Generated primary and replica StatefulSets contain the expected archive arguments.
- An explicit database ServiceAccount overrides the archive-specific fallback.
- A custom archive command survives reconciliation and PostgreSQL custom configuration selection.
- A real helper uploads a WAL fixture to a non-production bucket using Workload Identity or an equivalent projected identity.
- Repeated uploads are idempotent and checksum-verified.
- Restore tests can read the base backup and WAL archive from the same contract.
- Sidecar mode remains disabled until durable acknowledgement and outage recovery are implemented.

## Sources

[7] https://www.postgresql.org/docs/current/continuous-archiving.html — PostgreSQL continuous archiving and `archive_command`
[8] https://kubernetes.io/docs/concepts/security/service-accounts — Kubernetes Pod ServiceAccounts
[9] https://docs.cloud.google.com/kubernetes-engine/docs/concepts/workload-identity — GKE Workload Identity Federation
