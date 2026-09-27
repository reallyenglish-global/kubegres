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

By default, Kubegres marks the Pod terminating, waits five seconds for Service
endpoint removal to propagate, and then asks PostgreSQL to stop in `smart` mode.
That mode stops new work without forcibly disconnecting existing clients. Set
`spec.terminationGracePeriodSeconds` high enough for the longest expected
transaction or session drain; the default remains 10 seconds for compatibility.

The five-second drain is part of the built-in hook. Replacing
`spec.lifecycle.preStop` replaces that protection, so custom hooks must provide
their own endpoint-drain delay and orderly PostgreSQL shutdown.

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
        command: ["sh", "-c", "sleep 5 && pg_ctl -D \"$PGDATA\" stop -m smart"]
```

Changing either field rolls the primary and replica Pods.
