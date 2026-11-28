package resources_count_spec

import (
	"context"
	"errors"
	"testing"

	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	postgresV1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
	"reactive-tech.io/kubegres/internal/controller/ctx/log"
	"reactive-tech.io/kubegres/internal/controller/spec/template"
	states2 "reactive-tech.io/kubegres/internal/controller/states"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// newEphemeralBackUpFixture returns a Kubegres whose backup names a PVC, and
// a CronJob created with an ephemeral volume because that PVC did not exist.
func newEphemeralBackUpFixture() (*postgresV1.Kubegres, *batch.CronJob) {
	kubegres := &postgresV1.Kubegres{
		ObjectMeta: metav1.ObjectMeta{Name: "pg", Namespace: "default"},
		Spec: postgresV1.KubegresSpec{
			Image: "postgres:18",
			Backup: postgresV1.KubegresBackUp{
				Schedule:    "0 * * * *",
				VolumeMount: "/var/lib/backup",
				PvcName:     "backup-pvc",
			},
		},
	}
	cronJob := &batch.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "backup-pg", Namespace: "default"},
		Spec: batch.CronJobSpec{
			Schedule: "0 * * * *",
			JobTemplate: batch.JobTemplateSpec{Spec: batch.JobSpec{Template: core.PodTemplateSpec{Spec: core.PodSpec{
				Containers: []core.Container{{Image: "postgres:18", VolumeMounts: []core.VolumeMount{{MountPath: "/var/lib/backup"}}}},
				Volumes: []core.Volume{
					{VolumeSource: core.VolumeSource{Ephemeral: &core.EphemeralVolumeSource{}}},
					{VolumeSource: core.VolumeSource{ConfigMap: &core.ConfigMapVolumeSource{LocalObjectReference: core.LocalObjectReference{Name: "base-config-pg"}}}},
				},
			}}}},
		},
	}
	return kubegres, cronJob
}

func newBackUpEnforcerForTest(t *testing.T, kubegres *postgresV1.Kubegres, cronJob *batch.CronJob, pvcGetErr error) (BackUpCronJobCountSpecEnforcer, *bool) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := batch.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := core.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	deleted := false
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cronJob).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, isPvc := obj.(*core.PersistentVolumeClaim); isPvc && pvcGetErr != nil {
					return pvcGetErr
				}
				return c.Get(ctx, key, obj, opts...)
			},
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				deleted = true
				return c.Delete(ctx, obj, opts...)
			},
		}).Build()

	enforcer := CreateBackUpCronJobCountSpecEnforcer(
		ctx.KubegresContext{
			Kubegres: kubegres,
			Ctx:      context.Background(),
			Client:   kubeClient,
			Log:      log.LogWrapper{Kubegres: kubegres, Recorder: record.NewFakeRecorder(10)},
		},
		states2.ResourcesStates{
			BackUp: states2.BackUpStates{IsCronJobDeployed: true, DeployedCronJob: cronJob},
			Config: states2.ConfigStates{
				BaseConfigName:  "base-config-pg",
				ConfigLocations: states2.ConfigLocations{BackUpScript: ctx.BaseConfigMapVolumeName},
			},
		},
		template.ResourcesCreatorFromTemplate{},
	)
	return enforcer, &deleted
}

func TestBackUpEnforceSpecKeepsEquivalentEphemeralStorageWhenPvcIsMissing(t *testing.T) {
	kubegres, cronJob := newEphemeralBackUpFixture()
	kubegres.Spec.Backup.Size = "1024Mi"
	cronJob.Spec.JobTemplate.Spec.Template.Spec.Volumes[0].Ephemeral.VolumeClaimTemplate = &core.PersistentVolumeClaimTemplate{
		Spec: core.PersistentVolumeClaimSpec{Resources: core.VolumeResourceRequirements{
			Requests: core.ResourceList{core.ResourceStorage: resource.MustParse("1Gi")},
		}},
	}
	enforcer, deleted := newBackUpEnforcerForTest(t, kubegres, cronJob, nil)

	panicked := false
	func() {
		defer func() { panicked = recover() != nil }() // Base behavior deletes before attempting to recreate the CronJob.
		_ = enforcer.EnforceSpec()
	}()
	if *deleted {
		t.Fatal("backup CronJob using equivalent ephemeral storage was deleted although the PVC still does not exist")
	}
	if panicked {
		t.Fatal("EnforceSpec() panicked while keeping the backup CronJob")
	}
}

func TestBackUpEnforceSpecDetectsChangedEphemeralStorageWhenPvcIsMissing(t *testing.T) {
	kubegres, cronJob := newEphemeralBackUpFixture()
	kubegres.Spec.Backup.Size = "2Gi"
	cronJob.Spec.JobTemplate.Spec.Template.Spec.Volumes[0].Ephemeral.VolumeClaimTemplate = &core.PersistentVolumeClaimTemplate{
		Spec: core.PersistentVolumeClaimSpec{Resources: core.VolumeResourceRequirements{
			Requests: core.ResourceList{core.ResourceStorage: resource.MustParse("1Gi")},
		}},
	}
	enforcer, _ := newBackUpEnforcerForTest(t, kubegres, cronJob, nil)

	hasChanged, err := enforcer.hasSpecChanged()
	if err != nil {
		t.Fatalf("hasSpecChanged() error = %v", err)
	}
	if !hasChanged {
		t.Fatal("backup CronJob with a different ephemeral storage request was not marked changed")
	}
}

func TestBackUpEnforceSpecKeepsCronJobWhenPvcLookupFails(t *testing.T) {
	kubegres, cronJob := newEphemeralBackUpFixture()
	wantErr := errors.New("api server unavailable")
	enforcer, deleted := newBackUpEnforcerForTest(t, kubegres, cronJob, wantErr)

	if err := enforcer.EnforceSpec(); !errors.Is(err, wantErr) {
		t.Fatalf("EnforceSpec() error = %v, want %v", err, wantErr)
	}
	if *deleted {
		t.Fatal("backup CronJob was deleted although the PVC lookup failed")
	}
}
