package maintenance

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubegresv1 "reactive-tech.io/kubegres/api/v1"
)

func TestClassifyPodLoss(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	deleted := metav1.NewTime(now)
	nodeUID := "node-uid"

	tests := []struct {
		name  string
		input Input
		want  Classification
	}{
		{name: "scale generation change", input: Input{KubegresGeneration: 8, ObservedGeneration: 7}, want: Scale},
		{name: "direct delete", input: Input{PodDeletionTimestamp: &deleted}, want: PodDelete},
		{name: "planned upgrade", input: Input{Now: now, Operation: &kubegresv1.MaintenanceOperation{Spec: kubegresv1.MaintenanceOperationSpec{Purpose: kubegresv1.NodeUpgrade, TargetNode: kubegresv1.MaintenanceNodeReference{UID: nodeUID}, ExpiresAt: metav1.NewTime(now.Add(time.Hour))}, Status: kubegresv1.MaintenanceOperationStatus{Phase: kubegresv1.MaintenancePhasePending}}, PodNodeUID: nodeUID, DisruptionReason: "EvictionByEvictionAPI"}, want: NodeUpgrade},
		{name: "voluntary without intent", input: Input{DisruptionReason: "EvictionByEvictionAPI"}, want: PlannedVoluntaryUnknown},
		{name: "primary failure", input: Input{Primary: true, NodeUnreachable: true}, want: PrimaryFailure},
		{name: "contradictory evidence", input: Input{Primary: true, NodeUnreachable: true, DisruptionReason: "EvictionByEvictionAPI"}, want: Unknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyPodLoss(tt.input)
			if got.Class != tt.want {
				t.Fatalf("class = %q, want %q (source=%q)", got.Class, tt.want, got.Source)
			}
		})
	}
}

func TestMaintenanceOperationIsActive(t *testing.T) {
	for _, phase := range []kubegresv1.MaintenancePhase{kubegresv1.MaintenancePhasePending, kubegresv1.MaintenancePhaseRelocatingReplica, kubegresv1.MaintenancePhaseManualIntervention} {
		op := kubegresv1.MaintenanceOperation{Status: kubegresv1.MaintenanceOperationStatus{Phase: phase}}
		if !IsActive(&op) {
			t.Errorf("phase %q was not active", phase)
		}
	}
	for _, phase := range []kubegresv1.MaintenancePhase{kubegresv1.MaintenancePhaseCompleted, kubegresv1.MaintenancePhaseAborted} {
		op := kubegresv1.MaintenanceOperation{Status: kubegresv1.MaintenanceOperationStatus{Phase: phase}}
		if IsActive(&op) {
			t.Errorf("phase %q was active", phase)
		}
	}
}
