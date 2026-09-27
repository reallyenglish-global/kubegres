package statefulset_spec

import (
	"testing"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
)

func TestTerminationGracePeriodSecondsSpecEnforcerUsesConfiguredValue(t *testing.T) {
	configured := int64(60)
	enforcer := CreateTerminationGracePeriodSecondsSpecEnforcer(ctx.KubegresContext{
		Kubegres: &v1.Kubegres{Spec: v1.KubegresSpec{TerminationGracePeriodSeconds: &configured}},
	})
	statefulSet := testStatefulSetWithTerminationGracePeriod(10)

	if difference := enforcer.CheckForSpecDifference(&statefulSet); !difference.IsThereDifference() {
		t.Fatal("expected configured value to differ from the template default")
	}
	if _, err := enforcer.EnforceSpec(&statefulSet); err != nil {
		t.Fatalf("enforce termination grace period: %v", err)
	}
	if got := *statefulSet.Spec.Template.Spec.TerminationGracePeriodSeconds; got != configured {
		t.Fatalf("termination grace period = %d, want %d", got, configured)
	}
}

func TestTerminationGracePeriodSecondsSpecEnforcerPreservesDefault(t *testing.T) {
	enforcer := CreateTerminationGracePeriodSecondsSpecEnforcer(ctx.KubegresContext{
		Kubegres: &v1.Kubegres{},
	})
	statefulSet := testStatefulSetWithTerminationGracePeriod(10)

	if difference := enforcer.CheckForSpecDifference(&statefulSet); difference.IsThereDifference() {
		t.Fatalf("default template unexpectedly differs: %+v", difference)
	}
}

func testStatefulSetWithTerminationGracePeriod(value int64) apps.StatefulSet {
	return apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "postgres-0"},
		Spec: apps.StatefulSetSpec{Template: core.PodTemplateSpec{Spec: core.PodSpec{
			TerminationGracePeriodSeconds: &value,
		}}},
	}
}
