# Scheduled maintenance tasks

`spec.cronTasks` creates independent `batch/v1` CronJobs alongside the existing
`spec.backup` CronJob. It is intended for database maintenance such as
`ANALYZE`, vacuum scripts, integrity checks, or application-specific scheduled
operations. It does **not** change or replace the backup implementation.

Each task must have a unique `name`, a `schedule`, and an `image`. Kubegres
creates a deterministic owned CronJob (`<kubegres>-task-<task>`, with a stable
hash when the name would exceed the Kubernetes limit), updates it when the task
changes, and removes it when the task is removed from the Kubegres resource.

```yaml
apiVersion: kubegres.reactive-tech.io/v1
kind: Kubegres
metadata:
  name: database
spec:
  cronTasks:
    - name: analyse
      schedule: "15 2 * * *"
      image: postgres:16
      command: ["sh", "-c"]
      args: ["/scripts/analyse.sh"]
      script:
        configMapName: database-maintenance-scripts
        key: analyse.sh
        mountPath: /scripts/analyse.sh
      env:
        - name: TARGET_DATABASE
          value: app
      volumes:
        - name: task-data
          emptyDir: {}
      volumeMounts:
        - name: task-data
          mountPath: /task-data
      concurrencyPolicy: Forbid
      successfulJobsHistoryLimit: 3
      failedJobsHistoryLimit: 1
```

## Environment and storage

- Kubegres-level `spec.env` is inherited by each task. A task-local `env` entry
  with the same name overrides the inherited entry. Use `valueFrom` for database
  passwords and object-storage credentials; do not put credentials in a script
  or in the Kubegres manifest.
- `volumes` and `volumeMounts` are task-local Kubernetes `Volume` and `VolumeMount`
  definitions. They are not shared with the PostgreSQL Pods. A PVC can therefore
  be mounted for a base-backup/WAL export task, or an `emptyDir` can hold short-
  lived query results.
- `script` is optional. When configured, its ConfigMap key is mounted as a
  single file at `mountPath`; all three script fields are required. The task's
  `command` and `args` decide how the script is run.
- `concurrencyPolicy` may be `Allow`, `Forbid`, or `Replace`. Omit it to use
  the Kubernetes CronJob default. `Forbid` is recommended for backups and
  maintenance that must not overlap.

### Examples

The same mechanism supports WAL/archive or base-backup jobs and read-only
performance/statistics collection. The examples below assume a ConfigMap named
`database-maintenance-scripts`; the scripts are intentionally supplied by the
application owner because storage/object-store tooling is deployment-specific.

```yaml
spec:
  cronTasks:
    - name: wal-export
      schedule: "*/5 * * * *"
      image: postgres:16
      command: ["sh", "-c"]
      args: ["/scripts/wal-export.sh"]
      script:
        configMapName: database-maintenance-scripts
        key: wal-export.sh
        mountPath: /scripts/wal-export.sh
      env:
        - name: PGHOST
          value: database
        - name: PGUSER
          value: postgres
        - name: PGPASSWORD
          valueFrom:
            secretKeyRef:
              name: database-password
              key: password
      volumes:
        - name: archive
          persistentVolumeClaim:
            claimName: database-wal-archive
      volumeMounts:
        - name: archive
          mountPath: /archive
      concurrencyPolicy: Forbid

    - name: query-stats
      schedule: "*/15 * * * *"
      image: postgres:16
      command: ["sh", "-c"]
      args: ["/scripts/query-stats.sh"]
      script:
        configMapName: database-maintenance-scripts
        key: query-stats.sh
        mountPath: /scripts/query-stats.sh
      env:
        - name: PGHOST
          value: database
        - name: PGUSER
          value: postgres
        - name: PGPASSWORD
          valueFrom:
            secretKeyRef:
              name: database-password
              key: password
      concurrencyPolicy: Forbid
```

Keep WAL/base-backup work separate from query-statistics work so a slow or
failed export cannot delay monitoring. Scripts should use bounded timeouts and
write logs to stdout/stderr so normal Kubernetes Job history and log collection
can observe failures. This feature creates CronJobs only; it does not configure
PostgreSQL `archive_mode`, `archive_command`, replication slots, retention, or
object-storage lifecycle policy.

Kubegres validates duplicate task names, required task fields, incomplete
scripts, and unsupported concurrency policies before reconciling resources.
