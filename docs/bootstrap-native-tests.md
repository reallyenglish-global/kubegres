# Native logical bootstrap verification

Run as a non-root user with Python 3, openssl, coreutils (`timeout`, `setsid`, `mv`) and PostgreSQL client/server binaries:

```
PG_BINDIR=/usr/lib/postgresql/18/bin make test-bootstrap-native
```

These opt-in tests execute the actual `bootstrap.sh`, `initdb`, `pg_ctl`, `psql`, `pg_dump` and `pg_restore`. They create disposable synthetic source/destination clusters beneath TMPDIR, bind source TLS to loopback, use generated one-day certificates, and clean up their servers. No kubeconfig, shared databases, hosted CI or container runtime is involved. PostgreSQL 18.6 was exercised locally; other majors have not been verified. The staging Unix socket path must fit the host's Unix socket path limit; use a short, private TMPDIR if necessary.

Coverage:

- Real custom-format dump/restore preserves table values (including Unicode, quotes and NULL), sequence state, destination password and replication role attributes.
- TLS verify-full succeeds with the local CA/hostname; mismatched hostname and missing CA fail before publishing PGDATA.
- An unprivileged source user causes a real pg_dump permission failure.
- A deliberately corrupted real dump causes real pg_restore failure; staging is cleaned without publishing PGDATA. Only the corruption injection is a wrapper, not the PostgreSQL executable.
- SIGKILL interruption before restore (with an independently running real staging postmaster) and immediately before rename can restart successfully. The publication gate wraps `mv`; the restore gate wraps `pg_restore` and interrupts before invoking it.
- A stale postmaster PID file is recoverable. Foreign staging identity, nonempty unmarked PGDATA and a marker without PG_VERSION are rejected.
- A completed marker avoids re-import even with an unreachable source; CR UID/PVC name mismatch fails; completed-status input with missing storage fails closed.

The publication-crash regression failed against the original script: removing the identity before `mv` made the next invocation reject its own staging directory. Identity is now retained until publication. A surviving staging postmaster is stopped before staging deletion; failed stop leaves storage untouched.

Boundaries: directories model mounted storage, not Kubernetes PVC lifecycle. `BOOTSTRAP_COMPLETED` is a supplied input, not proof of controller status persistence. This does not establish operator reconciliation, API immutability enforcement, actual PVC replacement, readiness/status race recovery, Pod/container signal behavior, source timeout recovery, interrupted in-progress restore, cross-major imports or failover in a real cluster. Existing Go validation/template tests run separately via `make test-fast`. No live operator/PVC integration was executed.
