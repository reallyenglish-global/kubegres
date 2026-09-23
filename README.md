[Kubegres](https://www.kubegres.io/) is a Kubernetes operator allowing to deploy one or many clusters of PostgreSql pods with data
replication and failover enabled out-of-the box. It brings simplicity when using PostgreSql considering how complex managing
stateful-set's life-cycle and data replication could be with Kubernetes.

## About this fork

This is `reallyenglish-global/kubegres`, a fork of [reactive-tech/kubegres](https://github.com/reactive-tech/kubegres) taken at upstream tag `v1.18`. It exists because the fork needed newer Kubernetes and `sigs.k8s.io/controller-runtime` support than upstream tracked at the time, and a set of operational features upstream does not have: preStop and startupProbe overrides, failover on primary Pod drain with a managed PodDisruptionBudget, drain-aware replica-first failover, custom backup images, coordinated PostgreSQL minor image upgrades, independent scheduled cron tasks, automatic database PVC expansion, ephemeral backup PVCs, and WAL archive/restore commands.

See [CHANGELOG.md](CHANGELOG.md) for release history. If you are moving from `reactive-tech/kubegres` v1.18, read [`docs/upgrading-from-upstream.md`](docs/upgrading-from-upstream.md) first.

## Install

Apply the manifest tracking `main` (CRDs and operator, in one file):

```
kubectl apply -f https://raw.githubusercontent.com/reallyenglish-global/kubegres/main/kubegres.yaml
```

Or pin to a release by applying its `install.yaml` asset from the [Releases page](https://github.com/reallyenglish-global/kubegres/releases), for example:

```
kubectl apply -f https://github.com/reallyenglish-global/kubegres/releases/download/v1.22/install.yaml
```

The operator image is published at `ghcr.io/reallyenglish-global/kubegres`.

## Compatibility

| | |
|---|---|
| Kubernetes, tested | 1.36 (kind v1.36.1 in CI) |
| Kubernetes, minimum | 1.31 for all features, 1.27 for core features |
| PostgreSQL, validated by tests | 17.0, 17.2 |
| controller-runtime | 0.24 |
| Go | 1.26 |

The 1.31 floor is for `spec.podDisruptionBudget.unhealthyPodEvictionPolicy` (GA in 1.31). Core features need only 1.27, for `batch/v1` CronJob `timeZone` (GA in 1.27) and the Pod `DisruptionTarget` condition used to classify drain-triggered evictions (GA in 1.26). Automatic PVC expansion additionally needs a StorageClass with `allowVolumeExpansion: true` and a CSI driver that supports online expansion.

## Getting started

Install the fork as described above. For the `kind: Kubegres` resource fields, use the [upstream Kubegres reference](http://www.kubegres.io/doc/getting-started.html); the resource shape is unchanged except for the additions documented here:

* [`docs/cron-tasks.md`](docs/cron-tasks.md): independent `spec.cronTasks` CronJobs used for scheduled database maintenance. The feature is separate from, and does not change, `spec.backup`.
* [`docs/backup-storage.md`](docs/backup-storage.md): using an existing backup PVC or temporary generic ephemeral storage for backup Pods.
* [`docs/container-overrides.md`](docs/container-overrides.md): replacing the startup probe (`spec.probe.startupProbe`) and the `preStop` hook (`spec.lifecycle.preStop`) on the PostgreSQL container.
* [`docs/primary-drain-failover.md`](docs/primary-drain-failover.md): the opt-in `spec.failover.onPrimaryPodDrain` feature and the managed PodDisruptionBudget.
* [`docs/pitr-archive.md`](docs/pitr-archive.md): custom WAL archive and restore commands using the database Pod identity.
* [`docs/postgres-minor-upgrades.md`](docs/postgres-minor-upgrades.md): how a `spec.image` minor-version tag change is rolled out as a coordinated upgrade.
* [`docs/pod-loss-classification-and-safe-node-maintenance.md`](docs/pod-loss-classification-and-safe-node-maintenance.md): design for classifying why a database Pod was lost and running safe planned node maintenance.
* [`docs/handoff-maintenance-operation-v1.md`](docs/handoff-maintenance-operation-v1.md): implementation handoff for the `MaintenanceOperation` CRD contract (schema only, no reconciler yet).
* [`docs/upgrading-from-upstream.md`](docs/upgrading-from-upstream.md): moving an existing `reactive-tech/kubegres` v1.18 install to this fork.

**Features**

* It can manage one or many clusters of Postgres instances.
  Each cluster of Postgres instances is created using a YAML of "kind: Kubegres". Each cluster is self-contained and is
  identified by its unique name and namespace.

* It creates a cluster of PostgreSql servers with [Streaming Replication](https://wiki.postgresql.org/wiki/Streaming_Replication) enabled: it creates a Primary PostgreSql pod and a
  number of Replica PostgreSql pods and replicates primary's database in real-time to Replica pods.

* It manages fail-over: if a Primary PostgreSql crashes, it automatically promotes a Replica PostgreSql as a Primary.

* It supports database PVC expansion. Increase `spec.database.size` when the database volume needs more space; the operator updates each PVC in replica-first order and leaves running Pods untouched when the CSI provider supports online filesystem resize. The StorageClass must set `allowVolumeExpansion: true`; shrinking is rejected.

* It has a data backup option allowing to dump PostgreSql data regularly in a given volume.

* It provides a very simple YAML with properties specialised for PostgreSql.

* It is resilient and has an automated test suite, [run on every change](https://github.com/reallyenglish-global/kubegres/tree/main/internal/test), and has been running in production.


**How does Kubegres differentiate itself?**

Kubegres is fully integrated with Kubernetes' lifecycle as it runs as an operator written in Go.  
It is minimalist in terms of codebase compared to other open-source Postgres operators. It has the minimal and
yet robust required features to manage a cluster of PostgreSql on Kubernetes. We aim keeping this project small and simple.

Among many reasons, there are [5 main ones why we recommend Kubegres](https://www.kubegres.io/#kubegres_compared).

**Sponsor**

Kubegres is sponsored by [Etikbee](https://www.etikbee.com) 
which is using Kubegres in production with over 25 clusters of Postgres.
Etikbee is a UK based marketplace which promotes reuse by allowing merchants 
to list their products for rent, for sale and advertise services such as product repair.

**Contribute**

If you would like to contribute to Kubegres, please read the page [How to contribute](http://www.kubegres.io/contribute/).

**More details about the project**

[Kubegres](https://www.kubegres.io/) was developed by [Reactive Tech Limited](https://www.reactive-tech.io/)  and Alex
Arica as the lead developer. Reactive Tech offers [support services](https://www.kubegres.io/support/) for Kubegres,
Kubernetes and PostgreSql. This repository is a fork maintained independently by reallyenglish-global; upstream support
arrangements do not cover fork-specific changes.

It was developed with the framework [Kubebuilder](https://book.kubebuilder.io/), an SDK for building Kubernetes
APIs using CRDs. Kubebuilder is maintained by the official Kubernetes API Machinery Special Interest Group (SIG). This
fork uses the Kubebuilder v4 project layout.

**Support**

[Reactive Tech Limited](https://www.reactive-tech.io/) offers support for organisations using upstream Kubegres. And we prioritise
new features requested by organisations paying supports as long the new features would benefit the Open Source community.
We start working on the implementation of new features within 24h of the request from organisations paying supports.
More details in the [support page](https://www.kubegres.io/support/).

**Interesting links**
* A webinar about Kubegres was hosted by PostgresConf on 25 May 2021. [Watch the recorded video.](https://postgresconf.org/conferences/2021_Postgres_Conference_Webinars/program/proposals/creating-a-resilient-postgresql-cluster-with-kubegres)
* The availability of Kubegres was published on [PostgreSql's official website](https://www.postgresql.org/about/news/kubegres-is-available-as-open-source-2197/).
* Google talked about Kubegres in their [Kubernetes Podcast #146](https://kubernetespodcast.com/episode/146-kubernetes-1.21/).
