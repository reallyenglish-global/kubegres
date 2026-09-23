# Changelog

All notable changes to `reallyenglish-global/kubegres` are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). This fork was taken from `reactive-tech/kubegres`
at upstream tag `v1.18`; entries below start from that point.

## [v1.22] - 2026-09-23

This release contains the changes merged to `main` since `v1.21`, including the post-upgrade hardening and
event-driven watch work from pull request #26.

### Added

- PITR WAL archive and restore commands: `spec.backup.archiveCommand` and `spec.backup.restoreCommand` configure
  PostgreSQL `archive_command`/`restore_command` using the database Pod's own identity (#16). See
  [`docs/pitr-archive.md`](docs/pitr-archive.md).
- Ephemeral backup PVCs: `spec.backup.size` requests a generic ephemeral PVC for the backup Pod when
  `spec.backup.pvcName` is empty or names a PVC that does not exist (#24). See
  [`docs/backup-storage.md`](docs/backup-storage.md).
- `MaintenanceOperation` CRD contract: schema, phases, and a pure classifier for planned node
  upgrade/maintenance/rotation. Contract only; no reconciler ships yet (#19). See
  [`docs/handoff-maintenance-operation-v1.md`](docs/handoff-maintenance-operation-v1.md).
- Fork-owned image publishing to `ghcr.io/reallyenglish-global/kubegres` on tagged release (#23).
- `spec.cronTasks[].schedule` and `spec.backup.schedule` accept an IANA `timeZone`, passed through to the
  underlying `batch/v1` CronJob (pending on this branch; requires Kubernetes 1.27 for CronJob `timeZone` GA).
- `spec.podDisruptionBudget.unhealthyPodEvictionPolicy`, passed through to the managed PodDisruptionBudget
  (pending on this branch; requires Kubernetes 1.31 for `unhealthyPodEvictionPolicy` GA).
- Additional `kubectl get kubegres` printer columns (pending on this branch).
- `--max-concurrent-reconciles` manager flag to bound concurrent Kubegres reconciles (pending on this branch).

### Fixed

- Closed fork test-coverage gaps and fixed ephemeral backup PVC validation (#25).
- Restored informer-cache settling during reconciliation so resource creation and status transitions do not stall
  while the cache catches up (#26).

## [v1.21] - 2026-09-19

- Added independent, configurable `spec.cronTasks` CronJobs, separate from `spec.backup` (#20). See
  [`docs/cron-tasks.md`](docs/cron-tasks.md).
- Added a custom backup CronJob image: `spec.backup.image` (#17).
- Added automatic database PVC expansion: increasing `spec.database.size` resizes each PVC in
  replica-first order when the StorageClass allows online expansion (#21).
- Added coordinated PostgreSQL minor image upgrades: a numeric tag change within the same major version
  is rolled out as a blocking, replica-first upgrade instead of a plain StatefulSet image rollout (#18).
  See [`docs/postgres-minor-upgrades.md`](docs/postgres-minor-upgrades.md).
- Added drain-aware, replica-first primary failover (#15). See
  [`docs/primary-drain-failover.md`](docs/primary-drain-failover.md).
- Upgraded `controller-runtime` through v0.24 (#10, #12, #13) and aligned Kind CI with the Kubernetes 1.36
  node image.

## [v1.20] - 2026-09-17

- Added `spec.failover.onPrimaryPodDrain`: opt-in promotion of a Ready replica when Kubernetes marks the
  primary Pod for voluntary disruption, plus a managed PodDisruptionBudget (#5). See
  [`docs/primary-drain-failover.md`](docs/primary-drain-failover.md).
- Added `spec.probe.startupProbe` override for the PostgreSQL container (#4).
- Added `spec.lifecycle.preStop` override for the PostgreSQL container (#2).
- Upgraded `controller-runtime` from v0.20.1 to v0.21.0, `k8s.io` dependencies from v0.32.0 to v0.33.0, the
  Go module baseline to Go 1.24, `controller-tools` to v0.18.0, `envtest` to Kubernetes 1.33, and Kind CI
  tooling to Kind v0.33.0 with Kubernetes 1.33 node images (#8, #9).
- Added the `Kubegres CI` GitHub Actions workflow and enabled `setup-go` caching (#3, #6).

## [v1.19] - 2025-05-11

First tagged release of the fork. Upstream release notes for this tag were not published; this entry is
built from `git log`.

- Migrated to the Kubebuilder v4 project layout to restore compatibility with Kubernetes up to 1.31, and
  validated against PostgreSQL 17 (#186 upstream).
- Updated the README for the fork.

[v1.21]: https://github.com/reallyenglish-global/kubegres/releases/tag/v1.21
[v1.22]: https://github.com/reallyenglish-global/kubegres/releases/tag/v1.22
[v1.20]: https://github.com/reallyenglish-global/kubegres/releases/tag/v1.20
[v1.19]: https://github.com/reallyenglish-global/kubegres/releases/tag/v1.19
