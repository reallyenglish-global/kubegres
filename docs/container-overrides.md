# PostgreSQL container overrides

Kubegres sets a startup probe and a `preStop` lifecycle hook on the PostgreSQL
container. Both can be replaced from the Kubegres spec. The override replaces
the whole probe or handler; it is not merged with the default.

## Startup probe

`spec.probe.startupProbe` is a standard `corev1.Probe`. Use it when the
database takes longer to open than the default cadence allows, for example on
large volumes or slow storage. `livenessProbe` and `readinessProbe` are
overridden the same way.

```yaml
spec:
  probe:
    startupProbe:
      exec:
        command: ["sh", "-c", "pg_isready -U postgres -h localhost"]
      periodSeconds: 10
      failureThreshold: 60
```

## Graceful shutdown

When a Pod is stopped, Kubernetes runs the `preStop` hook and removes the Pod
from the Service endpoints at the same time, then sends `SIGTERM`, then
`SIGKILL` once `spec.terminationGracePeriodSeconds` has elapsed. The built-in
hook uses that window as follows:

```sh
sleep 5 && pg_ctl -D $PGDATA -w -t 2 stop -m smart || pg_ctl -D $PGDATA -w stop -m fast
```

1. Five seconds of drain, so endpoint removal propagates and no new connection
   is routed to a database that is about to stop.
2. A short `smart` shutdown attempt, which lets sessions that are about to
   finish end on their own without being disconnected.
3. A `fast` shutdown, which disconnects remaining sessions, rolls back their
   open transactions and checkpoints before exiting.

The escalation to `fast` matters: `smart` alone never completes while a client
holds a connection open, which is the normal state with a connection pool. The
Pod would then be `SIGKILL`ed mid-shutdown and the next start would need crash
recovery.

`spec.terminationGracePeriodSeconds` defaults to 10 seconds, unchanged from
earlier versions, which leaves roughly three seconds for the `fast` shutdown.
Raise it for databases whose shutdown checkpoint takes longer than that. Values
below about 8 seconds leave no usable shutdown window at all, because the drain
and the `smart` attempt consume 7 of them.

The drain and the escalation are part of the built-in hook. Replacing
`spec.lifecycle.preStop` replaces them, so a custom hook must provide its own
endpoint-drain delay and a shutdown that completes within the grace period.

The built-in hook is written when a StatefulSet is created. Kubegres only
enforces `preStop` on an existing StatefulSet when `spec.lifecycle.preStop` is
set, so clusters created by an earlier version keep the hook they were created
with until their StatefulSet is re-created.

## PreStop hook

`spec.lifecycle.preStop` is a standard `corev1.LifecycleHandler`. Kubegres runs
it on the PostgreSQL container before the Pod is stopped, so a custom handler
can perform an orderly shutdown or notify an external system. Keep it shorter
than the Pod termination grace period.

```yaml
spec:
  lifecycle:
    preStop:
      exec:
        command: ["sh", "-c", "sleep 5 && pg_ctl -D \"$PGDATA\" -w -t 40 stop -m smart || pg_ctl -D \"$PGDATA\" -w stop -m fast"]
  terminationGracePeriodSeconds: 60
```

Changing `spec.probe.startupProbe`, `spec.lifecycle.preStop` or
`spec.terminationGracePeriodSeconds` rolls the primary and replica Pods.
