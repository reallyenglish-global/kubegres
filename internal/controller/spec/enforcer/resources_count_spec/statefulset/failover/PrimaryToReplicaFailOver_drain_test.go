package failover

import (
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestIsVoluntaryDisruption(t *testing.T) {
	deletion := metav1.Now()
	cases := []struct {
		name     string
		pod      core.Pod
		expected bool
	}{
		{"ordinary pod", core.Pod{}, false},
		{"deleted without disruption condition", core.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletion}}, false},
		{"eviction", core.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletion}, Status: core.PodStatus{Conditions: []core.PodCondition{{Type: core.DisruptionTarget, Status: core.ConditionTrue, Reason: "EvictionByEvictionAPI"}}}}, true},
		{"preemption", core.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletion}, Status: core.PodStatus{Conditions: []core.PodCondition{{Type: core.DisruptionTarget, Status: core.ConditionTrue, Reason: "PreemptionByScheduler"}}}}, true},
		{"condition false", core.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletion}, Status: core.PodStatus{Conditions: []core.PodCondition{{Type: core.DisruptionTarget, Status: core.ConditionFalse, Reason: "EvictionByEvictionAPI"}}}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isVoluntaryDisruption(tc.pod); got != tc.expected {
				t.Fatalf("isVoluntaryDisruption() = %v, want %v", got, tc.expected)
			}
		})
	}
}
