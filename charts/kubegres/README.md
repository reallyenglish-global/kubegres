# Kubegres Helm chart

This chart installs the Kubegres operator and its `kubegres.reactive-tech.io` CRD.
It is maintained in the Kubegres repository so the chart, CRD, RBAC, and controller
image can be reviewed and released together.

## Install

```sh
helm upgrade --install kubegres ./charts/kubegres \
  --namespace kubegres-system \
  --create-namespace
```

The default image is the fork-owned `ghcr.io/reallyenglish-global/kubegres:v1.22`.
Override it explicitly when installing another release:

```sh
helm upgrade --install kubegres ./charts/kubegres \
  --namespace kubegres-system --create-namespace \
  --set image.repository=ghcr.io/reallyenglish-global/kubegres \
  --set image.tag=v1.23
```

## CRD lifecycle

Helm installs the CRD from `crds/kubegres.yaml`, but Helm does not upgrade or delete
CRDs automatically. Review and apply CRD changes separately before upgrading a chart
that contains a schema change. Kubegres custom resources are namespaced and can be
created after the operator is ready.

## Configuration

The main settings are available in `values.yaml`: controller image, resources,
metrics service, leader election, scheduling constraints, and security contexts.
The chart defaults to HTTPS metrics on port 8443, matching the repository's
Kustomize installation.
