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

func TestClassifyPodLossInvoluntaryDisruptionIsNotDirectDelete(t *testing.T) {
	deleted := metav1.NewTime(time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC))
	for _, reason := range []string{"DeletionByTaintManager", "TerminationByKubelet"} {
		t.Run("primary "+reason, func(t *testing.T) {
			got := ClassifyPodLoss(Input{Primary: true, NodeUnreachable: true, PodDeletionTimestamp: &deleted, DisruptionReason: reason})
			if got.Class != PrimaryFailure {
				t.Fatalf("class = %q, want %q (source=%q)", got.Class, PrimaryFailure, got.Source)
			}
		})
		t.Run("replica "+reason, func(t *testing.T) {
			got := ClassifyPodLoss(Input{PodDeletionTimestamp: &deleted, DisruptionReason: reason})
			if got.Class == PodDelete {
				t.Fatalf("involuntary disruption %q was classified as direct delete", reason)
			}
		})
	}
	t.Run("primary on unreachable node with deletion timestamp", func(t *testing.T) {
		got := ClassifyPodLoss(Input{Primary: true, NodeUnreachable: true, PodDeletionTimestamp: &deleted})
		if got.Class != PrimaryFailure {
			t.Fatalf("class = %q, want %q (source=%q)", got.Class, PrimaryFailure, got.Source)
		}
	})
}

func TestClassifyPodLossCorrelatesInvoluntaryDisruptionWithActiveOperation(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	deleted := metav1.NewTime(now)
	op := &kubegresv1.MaintenanceOperation{Spec: kubegresv1.MaintenanceOperationSpec{Purpose: kubegresv1.NodeUpgrade, TargetNode: kubegresv1.MaintenanceNodeReference{UID: "node-uid"}, ExpiresAt: metav1.NewTime(now.Add(time.Hour))}}
	got := ClassifyPodLoss(Input{Now: now, Operation: op, PodNodeUID: "node-uid", PodDeletionTimestamp: &deleted, DisruptionReason: "DeletionByTaintManager"})
	if got.Class != NodeUpgrade {
		t.Fatalf("class = %q, want %q (source=%q)", got.Class, NodeUpgrade, got.Source)
	}
}

func TestClassifyPodLossRecordsEvidence(t *testing.T) {
	got := ClassifyPodLoss(Input{Primary: true, NodeUnreachable: true, PodNodeUID: "node-uid", DisruptionReason: "DeletionByTaintManager"})
	for key, want := range map[string]string{"disruption_reason": "DeletionByTaintManager", "node_uid": "node-uid", "primary": "true", "node_unreachable": "true"} {
		if got.Evidence[key] != want {
			t.Errorf("evidence[%q] = %q, want %q", key, got.Evidence[key], want)
		}
	}
}

func TestClassifyPodLossIgnoresExpiredOperation(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	op := &kubegresv1.MaintenanceOperation{Spec: kubegresv1.MaintenanceOperationSpec{Purpose: kubegresv1.NodeUpgrade, TargetNode: kubegresv1.MaintenanceNodeReference{UID: "node-uid"}, ExpiresAt: metav1.NewTime(now)}}
	got := ClassifyPodLoss(Input{Now: now, Operation: op, PodNodeUID: "node-uid", DisruptionReason: ReasonEvictionByEvictionAPI})
	if got.Class != PlannedVoluntaryUnknown {
		t.Fatalf("expired operation still matched: class=%q source=%q", got.Class, got.Source)
	}
}

func TestMaintenanceOperationClosedIsInactive(t *testing.T) {
	op := kubegresv1.MaintenanceOperation{Status: kubegresv1.MaintenanceOperationStatus{Phase: kubegresv1.MaintenancePhaseClosed}}
	if IsActive(&op) {
		t.Fatal("Closed operation reported active")
	}
}
