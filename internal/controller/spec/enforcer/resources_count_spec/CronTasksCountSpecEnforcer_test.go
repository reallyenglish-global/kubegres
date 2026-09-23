package resources_count_spec

import (
	"context"
	"testing"

	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	postgresV1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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

// Proves that spec.cronTasks[].timeZone is wired through to the CronJob's
// spec.timeZone so the task runs in the requested IANA time zone.
func TestCronTasksEnforcerSetsCronJobTimeZone(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batch.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	timeZone := "America/New_York"
	postgres := &postgresV1.Kubegres{
		ObjectMeta: metav1.ObjectMeta{Name: "database", Namespace: "default", UID: types.UID("database-uid")},
		Spec: postgresV1.KubegresSpec{
			CronTasks: []postgresV1.KubegresCronTask{{
				Name: "analyse", Schedule: "15 2 * * *", Image: "postgres:16", TimeZone: &timeZone,
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
	if cronJob.Spec.TimeZone == nil || *cronJob.Spec.TimeZone != timeZone {
		t.Fatalf("expected CronJob spec.timeZone %q, got %#v", timeZone, cronJob.Spec.TimeZone)
	}

	newZone := "UTC"
	postgres.Spec.CronTasks[0].TimeZone = &newZone
	if err := enforcer.EnforceSpec(); err != nil {
		t.Fatal(err)
	}
	if err := kubeClient.Get(context.Background(), key, cronJob); err != nil {
		t.Fatal(err)
	}
	if cronJob.Spec.TimeZone == nil || *cronJob.Spec.TimeZone != newZone {
		t.Fatalf("expected CronJob spec.timeZone to be updated to %q, got %#v", newZone, cronJob.Spec.TimeZone)
	}
}

// Proves that EnforceSpec lists CronJobs with a label selector scoped to the
// reconciling Kubegres, instead of listing every CronJob in the namespace and
// filtering client-side. This keeps reconciles cheap in namespaces with many
// unrelated CronJobs.
func TestCronTasksEnforcerListsCronJobsScopedByLabel(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batch.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	postgres := &postgresV1.Kubegres{
		ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "default", UID: types.UID("alpha-uid")},
	}
	var capturedOpts []client.ListOption
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			capturedOpts = opts
			return c.List(ctx, list, opts...)
		},
	}).Build()
	enforcer := CreateCronTasksCountSpecEnforcer(ctx.KubegresContext{Ctx: context.Background(), Client: kubeClient, Kubegres: postgres})

	if err := enforcer.EnforceSpec(); err != nil {
		t.Fatal(err)
	}

	listOpts := &client.ListOptions{}
	for _, o := range capturedOpts {
		o.ApplyToList(listOpts)
	}
	if listOpts.LabelSelector == nil || listOpts.LabelSelector.Empty() {
		t.Fatal("expected the CronJob listing to carry a non-empty label selector")
	}
	if !listOpts.LabelSelector.Matches(labels.Set{cronTaskOwnerLabel: "alpha"}) {
		t.Fatalf("expected the CronJob listing to be scoped to kubegres %q via label %q, got selector %v", "alpha", cronTaskOwnerLabel, listOpts.LabelSelector)
	}
}

// Proves that EnforceSpec scopes its CronJob listing to the reconciling
// Kubegres, so enforcing one Kubegres's cron tasks never deletes another
// Kubegres's cron task CronJob, while still deleting its own orphan.
func TestCronTasksEnforcerScopesListingToOwningKubegres(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batch.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	alpha := &postgresV1.Kubegres{
		ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "default", UID: types.UID("alpha-uid")},
		Spec: postgresV1.KubegresSpec{
			CronTasks: []postgresV1.KubegresCronTask{{Name: "job", Schedule: "0 0 * * *", Image: "postgres:16"}},
		},
	}
	beta := &postgresV1.Kubegres{
		ObjectMeta: metav1.ObjectMeta{Name: "beta", Namespace: "default", UID: types.UID("beta-uid")},
		Spec: postgresV1.KubegresSpec{
			CronTasks: []postgresV1.KubegresCronTask{{Name: "job", Schedule: "0 0 * * *", Image: "postgres:16"}},
		},
	}
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	alphaEnforcer := CreateCronTasksCountSpecEnforcer(ctx.KubegresContext{Ctx: context.Background(), Client: kubeClient, Kubegres: alpha})
	betaEnforcer := CreateCronTasksCountSpecEnforcer(ctx.KubegresContext{Ctx: context.Background(), Client: kubeClient, Kubegres: beta})
	if err := alphaEnforcer.EnforceSpec(); err != nil {
		t.Fatal(err)
	}
	if err := betaEnforcer.EnforceSpec(); err != nil {
		t.Fatal(err)
	}

	alphaKey := client.ObjectKey{Namespace: "default", Name: "alpha-task-job"}
	betaKey := client.ObjectKey{Namespace: "default", Name: "beta-task-job"}
	if err := kubeClient.Get(context.Background(), betaKey, &batch.CronJob{}); err != nil {
		t.Fatalf("expected beta's CronJob to exist before alpha is reconciled: %v", err)
	}

	// Remove alpha's task and re-enforce alpha only: alpha's own orphaned
	// CronJob must be deleted, and beta's CronJob must be untouched.
	alpha.Spec.CronTasks = nil
	if err := alphaEnforcer.EnforceSpec(); err != nil {
		t.Fatal(err)
	}

	if err := kubeClient.Get(context.Background(), alphaKey, &batch.CronJob{}); err == nil {
		t.Fatal("expected alpha's orphaned CronJob to be deleted")
	}
	if err := kubeClient.Get(context.Background(), betaKey, &batch.CronJob{}); err != nil {
		t.Fatalf("expected beta's CronJob to remain untouched by alpha's reconcile: %v", err)
	}
}

func TestCronTaskNameIsDeterministicAndBounded(t *testing.T) {
	name := cronTaskName("a-very-long-kubegres-name-that-cannot-fit-with-a-task", "a-very-long-task-name-that-cannot-fit")
	if len(name) > 63 || name != cronTaskName("a-very-long-kubegres-name-that-cannot-fit-with-a-task", "a-very-long-task-name-that-cannot-fit") {
		t.Fatalf("expected bounded deterministic CronJob name, got %q", name)
	}
}
