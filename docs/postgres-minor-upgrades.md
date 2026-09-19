# PostgreSQL minor-image upgrades

When `spec.image` changes on an existing Kubegres resource, the controller treats a
numeric PostgreSQL image tag change within the same major version as a coordinated
minor upgrade rather than allowing the normal StatefulSet image rollout.

The blocking operation:

1. updates one ready replica and waits for its Pod to become ready with the target image;
2. removes the old primary and promotes that upgraded replica after the existing failover safety window;
3. waits for the promoted primary to be healthy;
4. releases the replica-count enforcer, which recreates the old primary position as a replica from the new primary.

The controller rejects untagged, digest-only, non-numeric, and major-version image
changes. Major PostgreSQL upgrades require an explicit migration procedure (such as
`pg_upgrade` or dump/restore) and are not performed by changing `spec.image`.
