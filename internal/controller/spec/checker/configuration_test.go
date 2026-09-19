package checker

import (
	v1 "k8s.io/api/core/v1"
	postgresV1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
	"testing"
)

// These decisions are pure: failures should be found before starting Kind.
// Integration tests remain responsible for API persistence and reconciliation.
func TestConfigurationPredicates(t *testing.T) {
	for _, tc := range []struct {
		name, config   string
		deployed, want bool
	}{
		{"no override", "", false, false}, {"base config", ctx.BaseConfigMapName, false, false},
		{"missing custom", "custom", false, true}, {"deployed custom", "custom", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := SpecChecker{}
			r.resourcesStates.Config.IsCustomConfigDeployed = tc.deployed
			spec := postgresV1.KubegresSpec{CustomConfig: tc.config}
			if got := r.isCustomConfigNotDeployed(&spec); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	r := SpecChecker{kubegresContext: ctx.KubegresContext{Kubegres: &postgresV1.Kubegres{}}}
	spec := &r.kubegresContext.Kubegres.Spec
	if r.isBackUpConfigured(spec) {
		t.Fatal("empty schedule enables backup")
	}
	spec.Backup.Schedule = "*/5 * * * *"
	if !r.isBackUpConfigured(spec) {
		t.Fatal("schedule does not enable backup")
	}
	spec.Env = []v1.EnvVar{{Name: ctx.EnvVarNameOfPostgresSuperUserPsw}}
	if !r.doesEnvVarExist(ctx.EnvVarNameOfPostgresSuperUserPsw) || r.doesEnvVarExist(ctx.EnvVarNameOfPostgresReplicationUserPsw) {
		t.Fatal("environment name lookup")
	}
	spec.Database.VolumeMount = "/data"
	spec.Volume.VolumeMounts = []v1.VolumeMount{{Name: "custom", MountPath: "/other"}}
	if r.doCustomVolumeMountsHaveReservedPath() {
		t.Fatal("unrelated mount rejected")
	}
	spec.Volume.VolumeMounts[0].MountPath = "/data"
	if !r.doCustomVolumeMountsHaveReservedPath() {
		t.Fatal("reserved database path accepted")
	}
	if got := r.validateCronTasks([]postgresV1.KubegresCronTask{{Name: "analyse", Schedule: "0 2 * * *", Image: "postgres:16"}}); got != "" {
		t.Fatalf("valid cron task rejected: %s", got)
	}
	if got := r.validateCronTasks([]postgresV1.KubegresCronTask{{Name: "analyse", Schedule: "0 2 * * *", Image: "postgres:16"}, {Name: "analyse", Schedule: "0 3 * * *", Image: "postgres:16"}}); got == "" {
		t.Fatal("duplicate cron task accepted")
	}
	if got := r.validateCronTasks([]postgresV1.KubegresCronTask{{Name: "analyse", Schedule: "0 2 * * *", Image: "postgres:16", Script: &postgresV1.KubegresCronTaskScript{ConfigMapName: "scripts"}}}); got == "" {
		t.Fatal("incomplete task script accepted")
	}
}
