package statefulset

import (
	"testing"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsPodReadyExcludesVoluntaryDisruption(t *testing.T) {
	deletion := metav1.Now()
	pod := core.Pod{
		ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletion},
		Status: core.PodStatus{
			ContainerStatuses: []core.ContainerStatus{{Ready: true}},
			Conditions: []core.PodCondition{{
				Type:   core.DisruptionTarget,
				Status: core.ConditionTrue,
				Reason: "EvictionByEvictionAPI",
			}},
		},
	}

	if got := (&PodStates{}).isPodReady(pod); got {
		t.Fatal("isPodReady() = true for a Pod being voluntarily disrupted")
	}
}

func TestIsPodBeingVoluntarilyDisrupted(t *testing.T) {
	deletion := metav1.Now()
	tests := []struct {
		name string
		pod  core.Pod
		want bool
	}{
		{name: "ordinary pod", pod: core.Pod{}, want: false},
		{name: "deleted without disruption condition", pod: core.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletion}}, want: false},
		{name: "eviction", pod: core.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletion}, Status: core.PodStatus{Conditions: []core.PodCondition{{Type: core.DisruptionTarget, Status: core.ConditionTrue, Reason: "EvictionByEvictionAPI"}}}}, want: true},
		{name: "preemption", pod: core.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletion}, Status: core.PodStatus{Conditions: []core.PodCondition{{Type: core.DisruptionTarget, Status: core.ConditionTrue, Reason: "PreemptionByScheduler"}}}}, want: true},
		{name: "condition false", pod: core.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletion}, Status: core.PodStatus{Conditions: []core.PodCondition{{Type: core.DisruptionTarget, Status: core.ConditionFalse, Reason: "EvictionByEvictionAPI"}}}}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsPodBeingVoluntarilyDisrupted(test.pod); got != test.want {
				t.Fatalf("IsPodBeingVoluntarilyDisrupted() = %v, want %v", got, test.want)
			}
		})
	}
}
