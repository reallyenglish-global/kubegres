package template

import (
	_ "embed"
	"fmt"

	core "k8s.io/api/core/v1"
	"reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
)

//go:embed bootstrap.sh
var logicalBootstrapScript string

const (
	bootstrapInitContainerName = "logical-bootstrap"
	bootstrapCAFile            = "/var/run/kubegres-bootstrap/ca/ca.crt"
)

func configureLogicalBootstrap(kubegres *v1.Kubegres, statefulSetSpec *core.PodSpec) error {
	if kubegres.Spec.Bootstrap == nil || kubegres.Spec.Bootstrap.Logical == nil {
		return nil
	}
	logical := kubegres.Spec.Bootstrap.Logical
	source := logical.Source
	port := source.Port
	if port == 0 {
		port = ctx.DefaultContainerPortNumber
	}
	destinationUser := "postgres"
	if env := findEnv(kubegres.Spec.Env, "POSTGRES_USER"); env.Value != "" {
		destinationUser = env.Value
	}

	bootstrap := core.Container{
		Name:    bootstrapInitContainerName,
		Image:   kubegres.Spec.Image,
		Command: []string{"/bin/sh", "-ceu", logicalBootstrapScript},
		Env: []core.EnvVar{
			{Name: "BOOTSTRAP_SOURCE_HOST", Value: source.Host},
			{Name: "BOOTSTRAP_SOURCE_PORT", Value: fmt.Sprintf("%d", port)},
			{Name: "BOOTSTRAP_SOURCE_DATABASE", Value: source.Database},
			{Name: "BOOTSTRAP_SOURCE_USERNAME", Value: source.Username},
			{Name: "BOOTSTRAP_DESTINATION_DATABASE", Value: logical.Database},
			{Name: "BOOTSTRAP_TIMEOUT_SECONDS", Value: fmt.Sprintf("%d", logical.TimeoutSeconds)},
			{Name: "BOOTSTRAP_CA_FILE", Value: bootstrapCAFile},
			{Name: "PGDATA", Value: kubegres.Spec.Database.VolumeMount + "/" + ctx.DefaultDatabaseFolder},
			{Name: "POSTGRES_USER", Value: destinationUser},
			{Name: "BOOTSTRAP_CR_UID", Value: string(kubegres.UID)},
			{Name: "BOOTSTRAP_PVC_NAME", Value: ctx.DatabaseVolumeName},
			{Name: "BOOTSTRAP_COMPLETED", Value: fmt.Sprintf("%t", kubegres.Status.BootstrapState == "completed")},
			{Name: "BOOTSTRAP_SOURCE_PASSWORD", ValueFrom: &core.EnvVarSource{SecretKeyRef: optionalSecretKeySelector(source.PasswordSecretKeyRef)}},
		},
		VolumeMounts: []core.VolumeMount{
			{Name: ctx.DatabaseVolumeName, MountPath: kubegres.Spec.Database.VolumeMount},
			{Name: ctx.BootstrapCAVolumeName, MountPath: "/var/run/kubegres-bootstrap/ca", ReadOnly: true},
		},
	}
	// Preserve the complete destination env entries, including literal values.
	for _, name := range []string{ctx.EnvVarNameOfPostgresSuperUserPsw, ctx.EnvVarNameOfPostgresReplicationUserPsw} {
		bootstrap.Env = append(bootstrap.Env, findEnv(kubegres.Spec.Env, name))
	}
	if len(statefulSetSpec.Containers) > 0 {
		primary := statefulSetSpec.Containers[0]
		bootstrap.SecurityContext = primary.SecurityContext.DeepCopy()
		bootstrap.Resources = primary.Resources
	}
	// The primary template currently has no init container, but prepend rather
	// than append so this remains the first gate if that changes.
	statefulSetSpec.InitContainers = append([]core.Container{bootstrap}, statefulSetSpec.InitContainers...)
	statefulSetSpec.Volumes = append(statefulSetSpec.Volumes, core.Volume{
		Name: ctx.BootstrapCAVolumeName,
		VolumeSource: core.VolumeSource{Secret: &core.SecretVolumeSource{
			SecretName: source.TLS.CASecretKeyRef.Name,
			Optional:   boolPtr(true),
			Items:      []core.KeyToPath{{Key: source.TLS.CASecretKeyRef.Key, Path: "ca.crt"}},
		}},
	})
	return nil
}

func optionalSecretKeySelector(selector core.SecretKeySelector) *core.SecretKeySelector {
	selector.Optional = nil
	return &selector
}

func boolPtr(value bool) *bool { return &value }

func findEnv(env []core.EnvVar, name string) core.EnvVar {
	for _, value := range env {
		if value.Name == name {
			return value
		}
	}
	return core.EnvVar{}
}
