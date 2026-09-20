package maintenance

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubegresv1 "reactive-tech.io/kubegres/api/v1"
)

func TestValidateOperation(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	valid := &kubegresv1.MaintenanceOperation{Spec: kubegresv1.MaintenanceOperationSpec{
		KubegresRef: kubegresv1.MaintenanceKubegresReference{Name: "db"}, Purpose: kubegresv1.NodeUpgrade,
		TargetNode: kubegresv1.MaintenanceNodeReference{UID: "uid"}, ExpiresAt: metav1.NewTime(now.Add(time.Hour)),
	}}
	if err := ValidateOperation(valid, now); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*kubegresv1.MaintenanceOperation){
		"missing target UID": func(v *kubegresv1.MaintenanceOperation) { v.Spec.TargetNode.UID = "" },
		"missing Kubegres":   func(v *kubegresv1.MaintenanceOperation) { v.Spec.KubegresRef.Name = "" },
		"expired":            func(v *kubegresv1.MaintenanceOperation) { v.Spec.ExpiresAt = metav1.NewTime(now) },
		"missing expiry":     func(v *kubegresv1.MaintenanceOperation) { v.Spec.ExpiresAt = metav1.Time{} },
		"missing purpose":    func(v *kubegresv1.MaintenanceOperation) { v.Spec.Purpose = "" },
		"unknown purpose":    func(v *kubegresv1.MaintenanceOperation) { v.Spec.Purpose = "reboot" },
	} {
		t.Run(name, func(t *testing.T) {
			v := valid.DeepCopy()
			mutate(v)
			if err := ValidateOperation(v, now); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestRecordHistoryIsBounded(t *testing.T) {
	op := &kubegresv1.MaintenanceOperation{}
	at := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	for i := 0; i < maxHistoryEntries+3; i++ {
		RecordHistory(op, at, kubegresv1.MaintenancePhasePending, "event", nil)
	}
	if len(op.Status.History) != maxHistoryEntries {
		t.Fatalf("history length = %d", len(op.Status.History))
	}
}

func TestScaleDeferredRequiresMatchingObservedGeneration(t *testing.T) {
	op := &kubegresv1.MaintenanceOperation{Status: kubegresv1.MaintenanceOperationStatus{Phase: kubegresv1.MaintenancePhaseRelocatingReplica, ObservedKubegresGeneration: 7}}
	if !ScaleDeferred(op, 7) {
		t.Fatal("expected scale to be deferred")
	}
	if ScaleDeferred(op, 8) {
		t.Fatal("generation change must not be silently deferred")
	}
}

func TestEffectiveSafetyAppliesApprovedDefaults(t *testing.T) {
	got := EffectiveSafety(kubegresv1.MaintenanceSafetyPolicy{})
	want := kubegresv1.MaintenanceSafetyPolicy{MaxReplayLagSeconds: 5, StableForSeconds: 60, RelocationDeadlineSeconds: 900}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	explicit := kubegresv1.MaintenanceSafetyPolicy{MaxReplayLagSeconds: 2, StableForSeconds: 30, RelocationDeadlineSeconds: 120}
	if got := EffectiveSafety(explicit); got != explicit {
		t.Fatalf("explicit policy overridden: got %+v", got)
	}
}

func TestValidateOperationRejectsNegativeSafetyThresholds(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	op := &kubegresv1.MaintenanceOperation{Spec: kubegresv1.MaintenanceOperationSpec{
		KubegresRef: kubegresv1.MaintenanceKubegresReference{Name: "db"}, Purpose: kubegresv1.NodeUpgrade,
		TargetNode: kubegresv1.MaintenanceNodeReference{UID: "uid"}, ExpiresAt: metav1.NewTime(now.Add(time.Hour)),
		Safety: kubegresv1.MaintenanceSafetyPolicy{StableForSeconds: -1},
	}}
	if err := ValidateOperation(op, now); err == nil {
		t.Fatal("expected validation error for negative stableForSeconds")
	}
}
