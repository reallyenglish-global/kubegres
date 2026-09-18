package template

import (
	"testing"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	postgresv1 "reactive-tech.io/kubegres/api/v1"
	controllerctx "reactive-tech.io/kubegres/internal/controller/ctx"
)

func TestConfigureArchiveAddsPostgresArchiveArguments(t *testing.T) {
	reconciler := ResourcesCreatorFromTemplate{
		kubegresContext: controllerctx.KubegresContext{
			Kubegres: &postgresv1.Kubegres{Spec: postgresv1.KubegresSpec{
				Backup: postgresv1.KubegresBackUp{
					ArchiveCommand:     "/usr/local/bin/archive %p %f",
					ServiceAccountName: "wal-archiver",
				},
			}},
		},
	}
	statefulSet := apps.StatefulSet{Spec: apps.StatefulSetSpec{Template: corePodTemplate([]string{"postgres"})}}

	reconciler.configureArchive(&statefulSet)

	args := statefulSet.Spec.Template.Spec.Containers[0].Args
	if !containsPair(args, "-c", "archive_mode=on") ||
		!containsPair(args, "-c", "archive_command=/usr/local/bin/archive %p %f") {
		t.Fatalf("archive arguments = %v", args)
	}
	if got := statefulSet.Spec.Template.Spec.ServiceAccountName; got != "wal-archiver" {
		t.Fatalf("service account = %q, want wal-archiver", got)
	}
}

func TestConfigureArchivePreservesExplicitDatabaseServiceAccount(t *testing.T) {
	reconciler := ResourcesCreatorFromTemplate{
		kubegresContext: controllerctx.KubegresContext{
			Kubegres: &postgresv1.Kubegres{Spec: postgresv1.KubegresSpec{
				ServiceAccountName: "database-runtime",
				Backup:             postgresv1.KubegresBackUp{ServiceAccountName: "wal-archiver"},
			}},
		},
	}
	statefulSet := apps.StatefulSet{Spec: apps.StatefulSetSpec{Template: corePodTemplate(nil)}}

	reconciler.configureArchive(&statefulSet)

	if got := statefulSet.Spec.Template.Spec.ServiceAccountName; got != "" {
		t.Fatalf("service account = %q, want empty template value", got)
	}
}

func corePodTemplate(args []string) core.PodTemplateSpec {
	return core.PodTemplateSpec{Spec: core.PodSpec{Containers: []core.Container{{Name: "postgres", Args: args}}}}
}

func containsPair(values []string, first, second string) bool {
	for i := 0; i+1 < len(values); i++ {
		if values[i] == first && values[i+1] == second {
			return true
		}
	}
	return false
}
