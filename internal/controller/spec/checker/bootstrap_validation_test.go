package checker

import (
	"testing"

	core "k8s.io/api/core/v1"
	postgresv1 "reactive-tech.io/kubegres/api/v1"
)

func validBootstrapSpec() *postgresv1.KubegresSpec {
	nonRoot := true
	uid := int64(999)
	replicas := int32(1)
	return &postgresv1.KubegresSpec{
		ContainerSecurityContext: &core.SecurityContext{RunAsNonRoot: &nonRoot, RunAsUser: &uid},
		Replicas:                 &replicas,
		Bootstrap: &postgresv1.KubegresBootstrap{Logical: &postgresv1.KubegresLogicalBootstrap{
			Source: postgresv1.KubegresBootstrapSource{
				Host: "source.example", Port: 5432, Database: "source_db", Username: "importer",
				PasswordSecretKeyRef: core.SecretKeySelector{LocalObjectReference: core.LocalObjectReference{Name: "source"}, Key: "password"},
				TLS:                  postgresv1.KubegresBootstrapTLS{CASecretKeyRef: core.SecretKeySelector{LocalObjectReference: core.LocalObjectReference{Name: "ca"}, Key: "ca.crt"}},
			},
			Database: "app_db", TimeoutSeconds: 600,
		}},
		Env: []core.EnvVar{{Name: "POSTGRES_PASSWORD"}, {Name: "POSTGRES_REPLICATION_PASSWORD"}},
	}
}

func TestBootstrapSpecErrorRejectsUnsafeOrIncompleteConfiguration(t *testing.T) {
	tests := []struct {
		name string
		edit func(*postgresv1.KubegresSpec)
	}{
		{"missing timeout", func(spec *postgresv1.KubegresSpec) { spec.Bootstrap.Logical.TimeoutSeconds = 0 }},
		{"nonempty custom config", func(spec *postgresv1.KubegresSpec) { spec.CustomConfig = "custom-config" }},
		{"system destination", func(spec *postgresv1.KubegresSpec) { spec.Bootstrap.Logical.Database = "postgres" }},
		{"unsafe source username", func(spec *postgresv1.KubegresSpec) { spec.Bootstrap.Logical.Source.Username = "importer-name" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := validBootstrapSpec()
			test.edit(spec)
			if got := (&SpecChecker{}).bootstrapSpecError(spec); got == "" {
				t.Fatal("invalid bootstrap configuration was accepted")
			}
		})
	}
}

func TestBootstrapSpecErrorAcceptsBoundedLogicalImport(t *testing.T) {
	if got := (&SpecChecker{}).bootstrapSpecError(validBootstrapSpec()); got != "" {
		t.Fatalf("valid bootstrap rejected: %s", got)
	}
}
