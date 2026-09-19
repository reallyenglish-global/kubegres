package resources_count_spec

import (
	"context"
	"testing"

	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	postgresV1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCronTasksEnforcerCreatesUpdatesAndDeletesManagedCronJobs(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batch.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	postgres := &postgresV1.Kubegres{
		ObjectMeta: metav1.ObjectMeta{Name: "database", Namespace: "default", UID: types.UID("database-uid")},
		Spec: postgresV1.KubegresSpec{
			Env: []core.EnvVar{{Name: "INHERITED", Value: "before"}, {Name: "OVERRIDDEN", Value: "old"}},
			CronTasks: []postgresV1.KubegresCronTask{{
				Name: "analyse", Schedule: "15 2 * * *", Image: "postgres:16", Command: []string{"sh", "-c"}, Args: []string{"/scripts/analyse.sh"},
				Script:       &postgresV1.KubegresCronTaskScript{ConfigMapName: "maintenance-scripts", Key: "analyse.sh", MountPath: "/scripts/analyse.sh"},
				Env:          []core.EnvVar{{Name: "OVERRIDDEN", Value: "new"}, {Name: "TARGET_DATABASE", Value: "app"}},
				Volumes:      []core.Volume{{Name: "data", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}}},
				VolumeMounts: []core.VolumeMount{{Name: "data", MountPath: "/data"}}, ConcurrencyPolicy: "Forbid",
			}},
		},
	}
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	enforcer := CreateCronTasksCountSpecEnforcer(ctx.KubegresContext{Ctx: context.Background(), Client: kubeClient, Kubegres: postgres})
	if err := enforcer.EnforceSpec(); err != nil {
		t.Fatal(err)
	}

	cronJob := &batch.CronJob{}
	key := client.ObjectKey{Namespace: "default", Name: "database-task-analyse"}
	if err := kubeClient.Get(context.Background(), key, cronJob); err != nil {
		t.Fatal(err)
	}
	if cronJob.Spec.Schedule != "15 2 * * *" || cronJob.Spec.ConcurrencyPolicy != batch.ForbidConcurrent {
		t.Fatalf("unexpected CronJob spec: %#v", cronJob.Spec)
	}
	container := cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0]
	if container.Image != "postgres:16" || len(container.VolumeMounts) != 2 || len(container.Env) != 3 {
		t.Fatalf("task container did not include requested image, mounts, and merged environment: %#v", container)
	}
	if got := container.VolumeMounts[1]; got.Name != "kubegres-task-script" || got.MountPath != "/scripts/analyse.sh" || got.SubPath != "analyse.sh" {
		t.Fatalf("script was not mounted as the requested ConfigMap key: %#v", got)
	}
	if container.Env[0].Name != "INHERITED" || container.Env[1].Value != "new" {
		t.Fatalf("task-local environment did not override inherited environment: %#v", container.Env)
	}

	postgres.Spec.CronTasks[0].Schedule = "30 3 * * *"
	if err := enforcer.EnforceSpec(); err != nil {
		t.Fatal(err)
	}
	if err := kubeClient.Get(context.Background(), key, cronJob); err != nil || cronJob.Spec.Schedule != "30 3 * * *" {
		t.Fatalf("CronJob was not updated: %v %#v", err, cronJob.Spec)
	}

	postgres.Spec.CronTasks = nil
	if err := enforcer.EnforceSpec(); err != nil {
		t.Fatal(err)
	}
	if err := kubeClient.Get(context.Background(), key, cronJob); err == nil {
		t.Fatal("CronJob was not deleted after task removal")
	}
}

func TestCronTaskNameIsDeterministicAndBounded(t *testing.T) {
	name := cronTaskName("a-very-long-kubegres-name-that-cannot-fit-with-a-task", "a-very-long-task-name-that-cannot-fit")
	if len(name) > 63 || name != cronTaskName("a-very-long-kubegres-name-that-cannot-fit-with-a-task", "a-very-long-task-name-that-cannot-fit") {
		t.Fatalf("expected bounded deterministic CronJob name, got %q", name)
	}
}
