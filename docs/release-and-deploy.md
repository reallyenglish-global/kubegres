# ReallyEnglish Kubegres release and deployment

This repository is the `reallyenglish-global/kubegres` fork. The operator image used by this fork is **our own image** at `ghcr.io/reallyenglish-global/kubegres`; do not use `reactivetechio/kubegres` for a ReallyEnglish release.

## Release contract

A GitHub release published from a signed-off `v*` tag builds and publishes a multi-architecture image for `linux/amd64` and `linux/arm64` through `.github/workflows/release.yml`. The workflow is the release publisher and has the minimal permissions required to write only this repository's GitHub Container Registry package.

For a normal release:

1. Start from a green commit on `main`; record its SHA in the release review.
2. Update the checked-in image tag in `config/manager/kustomization.yaml` and regenerate `kubegres.yaml` from that exact source commit.
3. Review the generated manifest and run the focused test gates.
4. Create and publish the corresponding GitHub release/tag (for example, `v1.21`). This invokes the release workflow, which publishes `ghcr.io/reallyenglish-global/kubegres:v1.21` and its immutable digest. Non-prerelease releases also receive the `latest` tag.
5. Verify the package version and digest in GHCR, then record the image digest with the release evidence.

The existing `v1.20` GitHub release has no container asset and the prior checked-in installer still references the upstream image. It must not be treated as a deployable ReallyEnglish operator release. The next release after this change is the first release published by the fork's registry workflow.

## Build and manifest generation

The repository requires Go `1.26`, matching `go.mod`. Build an architecture-specific image locally with an explicit non-release tag:

```bash
make docker-build IMG=ghcr.io/reallyenglish-global/kubegres:dev-<short-sha>
```

Build and publish the supported multi-architecture manifest only after the image tag and release commit have been approved:

```bash
make docker-buildx IMG=ghcr.io/reallyenglish-global/kubegres:v<version>
```

`docker-buildx` pushes to GHCR. It requires a token that is authorized to write the organization package; it does not deploy to a Kubernetes cluster.

To produce an installer for a reviewed image, run:

```bash
make build-installer IMG=ghcr.io/reallyenglish-global/kubegres:v<version>
```

This intentionally updates `config/manager/kustomization.yaml` before writing `dist/install.yaml`. Review both the configuration change and generated installer. Copy the approved generated installer to the checked-in `kubegres.yaml` as part of the release commit; this keeps the deploy target deterministic.

## Deploy contract

`make deploy` now has one meaning: it applies the already reviewed and checked-in `kubegres.yaml` to the kubeconfig-selected cluster. It does **not** build or push an image and it must not be used until all of the following are true:

- the tag is published in `ghcr.io/reallyenglish-global/kubegres` and its digest has been checked;
- the checked-in manifest references that approved tag/digest;
- the target context and namespace have been read back and approved; and
- a rollback manifest/image has been identified.

A Kubernetes deployment, upgrade, drain, CRD mutation, or rollback remains a separate approval-gated production operation. Do not run `make deploy` for a documentation or image-build change.

Before applying, use read-only checks such as:

```bash
kubectl config current-context
kubectl -n kubegres-system get deployment kubegres-controller-manager -o wide
kubectl -n kubegres-system get pods -l control-plane=controller-manager -o wide
```

After an approved deployment, verify the Deployment's observed image digest, rollout status, controller logs, and Kubegres resource reconciliation. Do not infer success from `kubectl apply` alone.

## Rollback

Keep the previous approved `kubegres.yaml` and image digest with the release record. A rollback is an explicit deployment operation: restore the previous reviewed manifest, apply it only with approval, and read back the rollout and controller health. CRD removal is not part of an ordinary operator rollback.
