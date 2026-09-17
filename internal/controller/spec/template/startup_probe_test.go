package template

import (
	apps "k8s.io/api/apps/v1"
	"os"
	"reflect"
	"testing"
)

func TestStartupProbeCadenceAndBudget(t *testing.T) {
	loader := ResourceTemplateLoader{}
	for name, load := range map[string]func() (apps.StatefulSet, error){"Primary": loader.LoadPrimaryStatefulSet, "Replica": loader.LoadReplicaStatefulSet} {
		t.Run(name, func(t *testing.T) {
			sts, err := load()
			if err != nil {
				t.Fatal(err)
			}
			probe := sts.Spec.Template.Spec.Containers[0].StartupProbe
			if probe.PeriodSeconds != 5 || probe.FailureThreshold != 180 || probe.InitialDelaySeconds != 10 {
				t.Fatalf("unexpected startup cadence: %+v", probe)
			}
			if probe.PeriodSeconds*probe.FailureThreshold != 900 {
				t.Fatal("startup budget changed")
			}
			if probe.Exec == nil || !reflect.DeepEqual(probe.Exec.Command, []string{"sh", "-c", "exec pg_isready -U postgres -h $POD_IP"}) {
				t.Fatal("startup command changed")
			}
			source, err := os.ReadFile("yaml/" + name + "StatefulSetTemplate.yaml")
			if err != nil {
				t.Fatal(err)
			}
			sourceSTS, err := loader.loadStatefulSet(string(source))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(sts, sourceSTS) {
				t.Fatal("generated template differs from source")
			}
		})
	}
}
