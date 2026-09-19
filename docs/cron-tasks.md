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
  with the same name overrides the inherited entry.
- `volumes` and `volumeMounts` are task-local Kubernetes `Volume` and
  `VolumeMount` definitions. They are not shared with the PostgreSQL Pods.
- `script` is optional. When configured, its ConfigMap key is mounted as a
  single file at `mountPath`; all three script fields are required.
- `concurrencyPolicy` may be `Allow`, `Forbid`, or `Replace`. Omit it to use
  the Kubernetes CronJob default.

Kubegres validates duplicate task names, required task fields, incomplete
scripts, and unsupported concurrency policies before reconciling resources.
