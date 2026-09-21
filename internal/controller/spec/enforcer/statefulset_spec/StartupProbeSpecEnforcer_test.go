package statefulset_spec

import (
	"reflect"
	"testing"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	postgresV1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
)

// defaultStartupProbe mirrors the built-in startup probe cadence baked into
// the StatefulSet templates (see template/startup_probe_test.go), so tests
// can simulate a StatefulSet that was rendered from the default template.
func defaultStartupProbe() *core.Probe {
	return &core.Probe{
		PeriodSeconds:       5,
		FailureThreshold:    180,
		InitialDelaySeconds: 10,
		ProbeHandler: core.ProbeHandler{
			Exec: &core.ExecAction{
				Command: []string{"sh", "-c", "exec pg_isready -U postgres -h $POD_IP"},
			},
		},
	}
}

func statefulSetWithStartupProbe(probe *core.Probe) *apps.StatefulSet {
	return &apps.StatefulSet{
		Spec: apps.StatefulSetSpec{
			Template: core.PodTemplateSpec{
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:         "postgres",
							StartupProbe: probe,
						},
					},
				},
			},
		},
	}
}

func TestStartupProbeSpecEnforcer_OverrideDetectedAndEnforced(t *testing.T) {
	expected := &core.Probe{
		PeriodSeconds:       15,
		FailureThreshold:    20,
		InitialDelaySeconds: 30,
		ProbeHandler: core.ProbeHandler{
			Exec: &core.ExecAction{Command: []string{"sh", "-c", "pg_isready -U postgres"}},
		},
	}

	enforcer := CreateStartupProbeSpecEnforcer(ctx.KubegresContext{
		Kubegres: &postgresV1.Kubegres{
			Spec: postgresV1.KubegresSpec{Probe: postgresV1.Probe{StartupProbe: expected}},
		},
	})

	statefulSet := statefulSetWithStartupProbe(defaultStartupProbe())

	diff := enforcer.CheckForSpecDifference(statefulSet)
	if !diff.IsThereDifference() {
		t.Fatal("expected a difference to be reported when the user-supplied startupProbe override differs from the StatefulSet's")
	}
	if diff.SpecName != "StartupProbe" {
		t.Fatalf("unexpected SpecName: %q", diff.SpecName)
	}

	wasUpdated, err := enforcer.EnforceSpec(statefulSet)
	if err != nil || !wasUpdated {
		t.Fatalf("EnforceSpec() = %v, %v; want true, nil", wasUpdated, err)
	}
	if !reflect.DeepEqual(statefulSet.Spec.Template.Spec.Containers[0].StartupProbe, expected) {
		t.Fatalf("EnforceSpec did not apply the expected startupProbe override to the postgres container, got %+v",
			statefulSet.Spec.Template.Spec.Containers[0].StartupProbe)
	}

	if diff := enforcer.CheckForSpecDifference(statefulSet); diff.IsThereDifference() {
		t.Fatalf("expected no difference after enforcement, got %+v", diff)
	}
}

func TestStartupProbeSpecEnforcer_NoDifferenceWhenUnset(t *testing.T) {
	enforcer := CreateStartupProbeSpecEnforcer(ctx.KubegresContext{
		Kubegres: &postgresV1.Kubegres{Spec: postgresV1.KubegresSpec{}},
	})

	original := defaultStartupProbe()
	statefulSet := statefulSetWithStartupProbe(original)

	diff := enforcer.CheckForSpecDifference(statefulSet)
	if diff.IsThereDifference() {
		t.Fatalf("expected no difference when spec.probe.startupProbe is unset, got %+v", diff)
	}
	if !reflect.DeepEqual(statefulSet.Spec.Template.Spec.Containers[0].StartupProbe, original) {
		t.Fatal("the StatefulSet's default startup cadence must be preserved when spec.probe.startupProbe is unset")
	}
}

func TestStartupProbeSpecEnforcer_NoDifferenceWhenIdentical(t *testing.T) {
	enforcer := CreateStartupProbeSpecEnforcer(ctx.KubegresContext{
		Kubegres: &postgresV1.Kubegres{
			Spec: postgresV1.KubegresSpec{Probe: postgresV1.Probe{StartupProbe: defaultStartupProbe()}},
		},
	})

	// A distinct pointer holding an equal value: the enforcer must compare by
	// value, not by identity.
	statefulSet := statefulSetWithStartupProbe(defaultStartupProbe())

	diff := enforcer.CheckForSpecDifference(statefulSet)
	if diff.IsThereDifference() {
		t.Fatalf("expected no difference for identical startup probes, got %+v", diff)
	}
}
