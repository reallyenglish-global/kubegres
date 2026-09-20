package template

import (
	"context"
	"testing"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubegresv1 "reactive-tech.io/kubegres/api/v1"
	ctx "reactive-tech.io/kubegres/internal/controller/ctx"
)

func TestCreateBackUpCronJobUsesEphemeralVolume(t *testing.T) {
	replicas := int32(1)
	kubegres := &kubegresv1.Kubegres{
		ObjectMeta: metav1.ObjectMeta{Name: "postgres", Namespace: "default"},
		Spec: kubegresv1.KubegresSpec{
			Replicas: &replicas,
			Image:    "postgres:17",
			Backup: kubegresv1.KubegresBackUp{
				Schedule:    "0 1 * * *",
				VolumeMount: "/backup",
				Size:        "20Gi",
			},
		},
	}
	creator := CreateResourcesCreatorFromTemplate(ctx.KubegresContext{
		Kubegres: kubegres,
		Ctx:      context.Background(),
	}, CustomConfigSpecHelper{}, ResourceTemplateLoader{})

	cronJob, err := creator.CreateBackUpCronJob("base-kubegres-config", true)
	if err != nil {
		t.Fatal(err)
	}

	volume := cronJob.Spec.JobTemplate.Spec.Template.Spec.Volumes[0]
	if volume.PersistentVolumeClaim != nil {
		t.Fatal("expected no direct PVC reference for an ephemeral backup volume")
	}
	if volume.Ephemeral == nil || volume.Ephemeral.VolumeClaimTemplate == nil {
		t.Fatal("expected an ephemeral PVC template")
	}
	storage := volume.Ephemeral.VolumeClaimTemplate.Spec.Resources.Requests[core.ResourceStorage]
	if got := storage.String(); got != "20Gi" {
		t.Fatalf("expected 20Gi backup volume, got %s", got)
	}
	if cronJob.Spec.JobTemplate.Spec.TTLSecondsAfterFinished == nil || *cronJob.Spec.JobTemplate.Spec.TTLSecondsAfterFinished != 0 {
		t.Fatal("expected ephemeral backup jobs to be deleted after completion")
	}
}

func TestCreateBackUpCronJobUsesConfiguredPVC(t *testing.T) {
	replicas := int32(1)
	kubegres := &kubegresv1.Kubegres{
		ObjectMeta: metav1.ObjectMeta{Name: "postgres", Namespace: "default"},
		Spec: kubegresv1.KubegresSpec{
			Replicas: &replicas,
			Image:    "postgres:17",
			Backup: kubegresv1.KubegresBackUp{
				Schedule:    "0 1 * * *",
				PvcName:     "backup-pvc",
				VolumeMount: "/backup",
			},
		},
	}
	creator := CreateResourcesCreatorFromTemplate(ctx.KubegresContext{
		Kubegres: kubegres,
		Ctx:      context.Background(),
	}, CustomConfigSpecHelper{}, ResourceTemplateLoader{})

	cronJob, err := creator.CreateBackUpCronJob("base-kubegres-config", false)
	if err != nil {
		t.Fatal(err)
	}

	volume := cronJob.Spec.JobTemplate.Spec.Template.Spec.Volumes[0]
	if volume.PersistentVolumeClaim == nil || volume.PersistentVolumeClaim.ClaimName != "backup-pvc" {
		t.Fatalf("expected configured PVC, got %#v", volume)
	}
	if volume.Ephemeral != nil {
		t.Fatal("did not expect an ephemeral volume for a configured PVC")
	}
	if cronJob.Spec.JobTemplate.Spec.TTLSecondsAfterFinished != nil {
		t.Fatal("did not expect a TTL override for a durable backup PVC")
	}
}
