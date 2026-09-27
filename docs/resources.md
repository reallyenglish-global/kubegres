# PostgreSQL resource requirements

Kubegres applies `spec.resources` to both the primary and replica PostgreSQL
containers. When their workloads need different memory or CPU capacity, set
role-specific requirements under `spec.primary.resources` and
`spec.replica.resources`:

```yaml
spec:
  primary:
    resources:
      requests:
        memory: 4Gi
        cpu: "500m"
      limits:
        memory: 8Gi
  replica:
    resources:
      requests:
        memory: 512Mi
        cpu: "250m"
      limits:
        memory: 1Gi
```

Each role-specific value replaces the common `spec.resources` value for that
role. If a role-specific value is not set, Kubegres keeps the existing behavior
and uses `spec.resources` for that role. This makes it possible to introduce
role-specific sizing without changing existing manifests.

Because a replica may be promoted during failover, ensure that its configured
resources are sufficient for the workload it may serve after promotion.
