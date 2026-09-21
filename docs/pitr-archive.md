# PITR WAL archive and restore commands

Kubegres can configure PostgreSQL WAL archiving and recovery with custom commands:

```yaml
spec:
  backup:
    archiveCommand: "/usr/local/bin/kubegres-wal-archive %p %f"
    restoreCommand: "/usr/local/bin/kubegres-wal-restore %p %f"
```

`archiveCommand` is passed as PostgreSQL `archive_command` and enables `archive_mode=on` on every database StatefulSet. `restoreCommand` is passed as PostgreSQL `restore_command`; PostgreSQL invokes it while recovering a WAL segment. Both executables must be present in the selected PostgreSQL image and must return success only after the operation has completed. The commands must be safe to retry.

The database Pod's existing `spec.serviceAccountName` (or its default Pod identity when unset) is responsible for object-storage access. Kubegres does not create or select a separate ServiceAccount for archiving or recovery. Grant that identity only the required bucket permissions, and do not log credentials from either helper.

## Object-storage contract

The archive and restore helpers own provider protocol details. The Kubegres operator only supplies the commands and PostgreSQL configuration. The helpers should provide:

- immutable object keys derived from the WAL filename;
- no overwrite of a different object with the same key;
- checksum and size verification;
- bounded network timeout and retry;
- redacted errors and no credential logging;
- a non-zero exit on authentication, permission, quota, missing-object, or durability failure.

## Acceptance gates

- Generated primary and replica StatefulSets contain the expected archive and restore arguments.
- A custom archive command survives reconciliation and PostgreSQL custom configuration selection.
- A custom restore command is available during PostgreSQL recovery.
- A real helper uploads and retrieves a WAL fixture from a non-production bucket using the Pod identity.
- Repeated uploads are idempotent and checksum-verified.
- Restore tests can read the base backup and WAL archive from the same contract.

## Sources

[7] https://www.postgresql.org/docs/current/continuous-archiving.html — PostgreSQL continuous archiving, `archive_command`, and `restore_command`
