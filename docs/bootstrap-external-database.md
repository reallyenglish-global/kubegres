# One-time logical bootstrap from an external PostgreSQL database

Kubegres supports an opt-in, create-time-only logical import from one external PostgreSQL database. The operator runs `pg_dump` over TLS, restores a custom-format dump into a private temporary PostgreSQL instance, creates the Kubegres destination database and replication role, and publishes the result into the new primary PVC only after the restore has stopped cleanly.

Example:

```yaml
spec:
  replicas: 1
  image: postgres:17.2
  containerSecurityContext:
    runAsUser: 999
    runAsGroup: 999
    runAsNonRoot: true
    allowPrivilegeEscalation: false
  database:
    size: 20Gi
  env:
    - name: POSTGRES_PASSWORD
      valueFrom:
        secretKeyRef:
          name: destination-postgres
          key: password
    - name: POSTGRES_REPLICATION_PASSWORD
      valueFrom:
        secretKeyRef:
          name: destination-replication
          key: password
  bootstrap:
    logical:
      source:
        host: source-db.example.internal
        port: 5432
        database: application
        username: migration_reader
        passwordSecretKeyRef:
          name: external-source
          key: password
        tls:
          caSecretKeyRef:
            name: external-source-ca
            key: ca.crt
      database: application
      timeoutSeconds: 3600
```

Safety contract:

- `bootstrap` is immutable. It is intended only for a newly created Kubegres resource.
- The source must be reachable with password authentication and TLS certificate verification. The source hostname must match its certificate.
- The import executes objects and data from the source dump with destination database privileges; use only a trusted source. The supported image contract is a PostgreSQL image with the PostgreSQL client/server tools available, and the container must run as a non-root PostgreSQL UID with a writable empty PVC.
- The source database, source username, and destination database are restricted to safe PostgreSQL identifiers. System destination databases are rejected.
- The destination and replication passwords are taken from the existing `POSTGRES_PASSWORD` and `POSTGRES_REPLICATION_PASSWORD` environment variables. Bootstrap-specific environment names, `POSTGRES_USER`, `POSTGRES_DB`, and `PGDATA` cannot be overridden.
- A mounted `PGDATA` directory must be empty, or contain the operator's valid completion marker and `PG_VERSION`. Existing non-empty data is never overwritten. Interrupted or ambiguous staging fails closed.
- A completed import is recorded in the PVC. The controller also records that the initial deployment has been attempted and will not create a replacement primary or re-import after PVC loss; recovery after storage loss is intentionally fail-closed and requires an explicit operator decision.
- The source password and CA Secret are mandatory during the initial import; the Pod references them as optional after completion so deleting them does not prevent marker-only restarts.
- Custom ConfigMaps containing initialization hooks are not supported in bootstrap mode. PostgreSQL configuration, PITR, scheduled backups, physical cloning, multi-database migration, tablespaces, continuous following, zero-downtime cutover, and arbitrary image/tool combinations are out of scope.

This is not full lifecycle migration or CloudNativePG bootstrap parity. The feature performs one initial import and does not preserve source roles, ACLs, tablespaces, or other cluster-level objects. Verify the imported application data and destination credentials before directing clients to the service.
