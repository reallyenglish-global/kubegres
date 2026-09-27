# PostgreSQL major upgrade contract

Kubegres does not treat a major-version image edit as a StatefulSet rollout. A
major upgrade requires a separate `MajorUpgrade` object, an isolated destination,
and an explicit migration procedure. This prevents an incompatible PostgreSQL
binary from being started against source PVCs.

## Core/operator responsibility

The `MajorUpgrade` API provides the stable, resumable phase contract for a
workflow runner to record which hook is current:

1. `preflight` — compatibility, permissions, extensions, and backup checks;
2. `bootstrap` — create the target cluster/storage without touching source PVCs;
3. `copy` — logical replication or dump/restore, selected by `strategy`;
4. `validate` — lag, schema/data checks, sequences, and application smoke tests;
5. `cutover` — application-coordinated write freeze, fencing, and endpoint switch;
6. optional `cleanup` — source retirement after an explicit retention decision.

The operator must not infer credentials, invent SQL, mutate source storage, or
promise zero downtime. The source remains authoritative until the owner’s
cutover hook has verified the destination. Rollback after divergent writes is a
manual decision.

## User-controlled hooks

Each required hook supplies an image and either a command/args pair or a
ConfigMap-backed script. The hook image and script are intentionally outside the
Kubegres controller so deployments can choose their logical-replication tool,
`pg_dump`/`pg_restore` workflow, secrets, network policy, validation depth, and
application cutover mechanism. Keep secrets in Kubernetes Secrets or an external
secret provider rather than in the `MajorUpgrade` object.

The API rejects floating or digest-only image references and requires the source
and target tags to have different numeric PostgreSQL major versions. Changing
`Kubegres.spec.image` remains the existing same-major/minor upgrade path and is
not a major-upgrade shortcut.

Apply the sample after replacing its example image and script references:

```sh
kubectl apply -f config/samples/kubegres_v1_majorupgrade.yaml
kubectl get pgmajor app-postgres-15-to-16 -o yaml
```
