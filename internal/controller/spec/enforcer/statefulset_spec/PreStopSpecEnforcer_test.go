package statefulset_spec

import (
	"reflect"
	"testing"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	postgresV1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
)

// defaultPreStopHandler mirrors the built-in preStop hook baked into the
// StatefulSet templates (see PrimaryStatefulSetTemplate.yaml), so tests can
// simulate a StatefulSet that was rendered from the default template.
func defaultPreStopHandler() *core.LifecycleHandler {
	return &core.LifecycleHandler{
		Exec: &core.ExecAction{
			Command: []string{"sh", "-c", "pg_ctl -D $PGDATA stop -m fast"},
		},
	}
}

func statefulSetWithPreStopHandler(handler *core.LifecycleHandler) *apps.StatefulSet {
	return &apps.StatefulSet{
		Spec: apps.StatefulSetSpec{
			Template: core.PodTemplateSpec{
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:      "postgres",
							Lifecycle: &core.Lifecycle{PreStop: handler},
						},
					},
				},
			},
		},
	}
}

func TestPreStopSpecEnforcer_DifferenceDetectedAndEnforced(t *testing.T) {
	expected := &core.LifecycleHandler{
		Exec: &core.ExecAction{Command: []string{"sh", "-c", "/scripts/custom-prestop.sh"}},
	}

	enforcer := CreatePreStopSpecEnforcer(ctx.KubegresContext{
		Kubegres: &postgresV1.Kubegres{
			Spec: postgresV1.KubegresSpec{Lifecycle: postgresV1.Lifecycle{PreStop: expected}},
		},
	})

	statefulSet := statefulSetWithPreStopHandler(defaultPreStopHandler())

	diff := enforcer.CheckForSpecDifference(statefulSet)
	if !diff.IsThereDifference() {
		t.Fatal("expected a difference to be reported when the Kubegres spec's preStop handler differs from the StatefulSet's")
	}
	if diff.SpecName != "PreStop" {
		t.Fatalf("unexpected SpecName: %q", diff.SpecName)
	}

	wasUpdated, err := enforcer.EnforceSpec(statefulSet)
	if err != nil || !wasUpdated {
		t.Fatalf("EnforceSpec() = %v, %v; want true, nil", wasUpdated, err)
	}
	if !reflect.DeepEqual(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop, expected) {
		t.Fatalf("EnforceSpec did not apply the expected preStop handler to the postgres container, got %+v",
			statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop)
	}

	// Once enforced, checking again should report no further difference.
	if diff := enforcer.CheckForSpecDifference(statefulSet); diff.IsThereDifference() {
		t.Fatalf("expected no difference after enforcement, got %+v", diff)
	}
}

func TestPreStopSpecEnforcer_NoDifferenceWhenUnset(t *testing.T) {
	enforcer := CreatePreStopSpecEnforcer(ctx.KubegresContext{
		Kubegres: &postgresV1.Kubegres{Spec: postgresV1.KubegresSpec{}},
	})

	original := defaultPreStopHandler()
	statefulSet := statefulSetWithPreStopHandler(original)

	diff := enforcer.CheckForSpecDifference(statefulSet)
	if diff.IsThereDifference() {
		t.Fatalf("expected no difference when spec.lifecycle.preStop is unset, got %+v", diff)
	}
	if !reflect.DeepEqual(statefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop, original) {
		t.Fatal("the StatefulSet's default preStop handler must be preserved when spec.lifecycle.preStop is unset")
	}
}

func TestPreStopSpecEnforcer_NoDifferenceWhenIdentical(t *testing.T) {
	enforcer := CreatePreStopSpecEnforcer(ctx.KubegresContext{
		Kubegres: &postgresV1.Kubegres{
			Spec: postgresV1.KubegresSpec{Lifecycle: postgresV1.Lifecycle{PreStop: defaultPreStopHandler()}},
		},
	})

	// A distinct pointer holding an equal value: the enforcer must compare by
	// value, not by identity.
	statefulSet := statefulSetWithPreStopHandler(defaultPreStopHandler())

	diff := enforcer.CheckForSpecDifference(statefulSet)
	if diff.IsThereDifference() {
		t.Fatalf("expected no difference for identical preStop handlers, got %+v", diff)
	}
}
