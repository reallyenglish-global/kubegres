package resources_count_spec

import (
	"context"
	"errors"
	"testing"

	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
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

func TestBackUpEnforceSpecKeepsCronJobWhenPvcLookupFails(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batch.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := core.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

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
	// The CronJob was created with an ephemeral volume because the backup PVC
	// did not exist yet.
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

	wantErr := errors.New("api server unavailable")
	deleted := false
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cronJob).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, isPvc := obj.(*core.PersistentVolumeClaim); isPvc {
					return wantErr
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

	if err := enforcer.EnforceSpec(); !errors.Is(err, wantErr) {
		t.Fatalf("EnforceSpec() error = %v, want %v", err, wantErr)
	}
	if deleted {
		t.Fatal("backup CronJob was deleted although the PVC lookup failed")
	}
}
