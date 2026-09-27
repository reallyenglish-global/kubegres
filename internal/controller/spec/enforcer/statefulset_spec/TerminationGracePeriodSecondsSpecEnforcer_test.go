package statefulset_spec

import (
	"testing"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
	"reactive-tech.io/kubegres/internal/controller/spec/template"
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

// An explicit value equal to the default must not be reported as a difference,
// otherwise writing the default in the YAML would roll every Pod.
func TestTerminationGracePeriodSecondsSpecEnforcerAcceptsExplicitDefault(t *testing.T) {
	configured := defaultTerminationGracePeriodSeconds
	enforcer := CreateTerminationGracePeriodSecondsSpecEnforcer(ctx.KubegresContext{
		Kubegres: &v1.Kubegres{Spec: v1.KubegresSpec{TerminationGracePeriodSeconds: &configured}},
	})
	statefulSet := testStatefulSetWithTerminationGracePeriod(defaultTerminationGracePeriodSeconds)

	if difference := enforcer.CheckForSpecDifference(&statefulSet); difference.IsThereDifference() {
		t.Fatalf("explicit default unexpectedly differs: %+v", difference)
	}
}

// A StatefulSet without the field, for example one edited by hand, is brought
// back to the expected value instead of inheriting the Kubernetes default of 30.
func TestTerminationGracePeriodSecondsSpecEnforcerSetsUndefinedValue(t *testing.T) {
	enforcer := CreateTerminationGracePeriodSecondsSpecEnforcer(ctx.KubegresContext{
		Kubegres: &v1.Kubegres{},
	})
	statefulSet := apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "postgres-0"},
		Spec:       apps.StatefulSetSpec{Template: core.PodTemplateSpec{Spec: core.PodSpec{}}},
	}

	difference := enforcer.CheckForSpecDifference(&statefulSet)
	if !difference.IsThereDifference() {
		t.Fatal("expected an undefined termination grace period to be reported as a difference")
	}
	if difference.Current != "<nil>" || difference.Expected != "10" {
		t.Fatalf("difference = %+v, want current '<nil>' and expected '10'", difference)
	}

	if _, err := enforcer.EnforceSpec(&statefulSet); err != nil {
		t.Fatalf("enforce termination grace period: %v", err)
	}
	if got := statefulSet.Spec.Template.Spec.TerminationGracePeriodSeconds; got == nil || *got != defaultTerminationGracePeriodSeconds {
		t.Fatalf("termination grace period = %v, want %d", got, defaultTerminationGracePeriodSeconds)
	}
}

// The default duplicates the value baked into the StatefulSet templates. If the
// two ever diverge, every freshly created cluster would be rolled by its first
// reconciliation, so they are checked against each other here.
func TestTerminationGracePeriodSecondsDefaultMatchesStatefulSetTemplates(t *testing.T) {
	loader := template.ResourceTemplateLoader{}
	templates := map[string]func() (apps.StatefulSet, error){
		"Primary": loader.LoadPrimaryStatefulSet,
		"Replica": loader.LoadReplicaStatefulSet,
	}
	for name, load := range templates {
		t.Run(name, func(t *testing.T) {
			statefulSet, err := load()
			if err != nil {
				t.Fatal(err)
			}
			got := statefulSet.Spec.Template.Spec.TerminationGracePeriodSeconds
			if got == nil || *got != defaultTerminationGracePeriodSeconds {
				t.Fatalf("template termination grace period = %v, want %d", got, defaultTerminationGracePeriodSeconds)
			}
		})
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
