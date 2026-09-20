package maintenance

import (
	"fmt"
	"time"

	kubegresv1 "reactive-tech.io/kubegres/api/v1"
)

// ValidateOperation checks the fail-closed contract before any controller
// workflow is allowed to mutate database topology.
func ValidateOperation(op *kubegresv1.MaintenanceOperation, now time.Time) error {
	if op == nil {
		return fmt.Errorf("maintenance operation is required")
	}
	if op.Spec.KubegresRef.Name == "" {
		return fmt.Errorf("kubegresRef.name is required")
	}
	if op.Spec.TargetNode.UID == "" {
		return fmt.Errorf("targetNode.uid is required")
	}
	switch op.Spec.Purpose {
	case kubegresv1.NodeUpgrade, kubegresv1.NodeMaintenance, kubegresv1.NodeRotation:
	default:
		return fmt.Errorf("unsupported maintenance purpose %q", op.Spec.Purpose)
	}
	if op.Spec.ExpiresAt.IsZero() {
		return fmt.Errorf("expiresAt is required")
	}
	if !now.IsZero() && !now.Before(op.Spec.ExpiresAt.Time) {
		return fmt.Errorf("maintenance operation has expired")
	}
	safety := op.Spec.Safety
	if safety.MaxReplayLagSeconds < 0 || safety.StableForSeconds < 0 || safety.RelocationDeadlineSeconds < 0 {
		return fmt.Errorf("safety thresholds must not be negative")
	}
	return nil
}

// ScaleDeferred reports whether normal replica-count reconciliation must yield
// to an active maintenance operation for the referenced Kubegres generation.
func ScaleDeferred(op *kubegresv1.MaintenanceOperation, kubegresGeneration int64) bool {
	return IsActive(op) && op.Status.ObservedKubegresGeneration != 0 && op.Status.ObservedKubegresGeneration == kubegresGeneration
}

// Approved safety thresholds. They are also declared as CRD defaults so a
// stored object carries them; EffectiveSafety covers objects built in-process.
const (
	DefaultMaxReplayLagSeconds       int32 = 5
	DefaultStableForSeconds          int32 = 60
	DefaultRelocationDeadlineSeconds int32 = 900
)

// EffectiveSafety returns the safety policy with approved defaults applied to
// any unset threshold.
func EffectiveSafety(policy kubegresv1.MaintenanceSafetyPolicy) kubegresv1.MaintenanceSafetyPolicy {
	if policy.MaxReplayLagSeconds == 0 {
		policy.MaxReplayLagSeconds = DefaultMaxReplayLagSeconds
	}
	if policy.StableForSeconds == 0 {
		policy.StableForSeconds = DefaultStableForSeconds
	}
	if policy.RelocationDeadlineSeconds == 0 {
		policy.RelocationDeadlineSeconds = DefaultRelocationDeadlineSeconds
	}
	return policy
}
