package template

import (
	"strings"
	"testing"

	core "k8s.io/api/core/v1"
	postgresv1 "reactive-tech.io/kubegres/api/v1"
	controllerctx "reactive-tech.io/kubegres/internal/controller/ctx"
)

func TestConfigureLogicalBootstrapUsesMountedEmptyPGDATAAndRunsFirst(t *testing.T) {
	kubegres := &postgresv1.Kubegres{Spec: postgresv1.KubegresSpec{
		Image:    "postgres:17",
		Database: postgresv1.KubegresDatabase{VolumeMount: "/var/lib/postgresql/data"},
		Bootstrap: &postgresv1.KubegresBootstrap{Logical: &postgresv1.KubegresLogicalBootstrap{
			Source: postgresv1.KubegresBootstrapSource{
				Host: "source.example", Port: 5432, Database: "source_db", Username: "importer",
				PasswordSecretKeyRef: core.SecretKeySelector{LocalObjectReference: core.LocalObjectReference{Name: "source"}, Key: "password"},
				TLS:                  postgresv1.KubegresBootstrapTLS{CASecretKeyRef: core.SecretKeySelector{LocalObjectReference: core.LocalObjectReference{Name: "source-ca"}, Key: "ca.crt"}},
			},
			Database: "app_db", TimeoutSeconds: 600,
		}},
		Env: []core.EnvVar{
			{Name: "POSTGRES_PASSWORD", ValueFrom: &core.EnvVarSource{SecretKeyRef: &core.SecretKeySelector{LocalObjectReference: core.LocalObjectReference{Name: "destination"}, Key: "password"}}},
			{Name: "POSTGRES_REPLICATION_PASSWORD", ValueFrom: &core.EnvVarSource{SecretKeyRef: &core.SecretKeySelector{LocalObjectReference: core.LocalObjectReference{Name: "replication"}, Key: "password"}}},
		},
	}}
	pod := core.PodSpec{InitContainers: []core.Container{{Name: "existing-init"}}}

	if err := configureLogicalBootstrap(kubegres, &pod, "postgres-db-example-1-0"); err != nil {
		t.Fatal(err)
	}
	if got := pod.InitContainers[0].Name; got != bootstrapInitContainerName {
		t.Fatalf("first init container = %q", got)
	}
	bootstrap := pod.InitContainers[0]
	if got := envValue(bootstrap.Env, "PGDATA").Value; got != "/var/lib/postgresql/data/pgdata" {
		t.Fatalf("PGDATA = %q", got)
	}
	if got := envValue(bootstrap.Env, "BOOTSTRAP_TIMEOUT_SECONDS").Value; got != "600" {
		t.Fatalf("timeout = %q", got)
	}
	if got := envValue(bootstrap.Env, "BOOTSTRAP_SOURCE_PASSWORD").ValueFrom.SecretKeyRef.Name; got != "source" {
		t.Fatalf("source secret = %q", got)
	}
	passwordSelector := envValue(bootstrap.Env, "BOOTSTRAP_SOURCE_PASSWORD").ValueFrom.SecretKeyRef
	if passwordSelector.Optional == nil || !*passwordSelector.Optional {
		t.Fatal("source password must be optional at Pod admission so marker-only restarts survive Secret deletion")
	}
	if got := envValue(bootstrap.Env, "BOOTSTRAP_PVC_NAME").Value; got != "postgres-db-example-1-0" {
		t.Fatalf("PVC identity = %q", got)
	}
	if !strings.Contains(bootstrap.Command[2], "find \"$target\" -mindepth 1") || !strings.Contains(bootstrap.Command[2], "completion marker") {
		t.Fatal("bootstrap script does not safely handle a mounted empty PGDATA")
	}
	if len(pod.Volumes) != 1 || pod.Volumes[0].Name != controllerctx.BootstrapCAVolumeName {
		t.Fatalf("bootstrap volumes = %#v", pod.Volumes)
	}
}

func envValue(env []core.EnvVar, name string) core.EnvVar {
	for _, value := range env {
		if value.Name == name {
			return value
		}
	}
	return core.EnvVar{}
}
