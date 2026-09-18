package maintenance

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubegresv1 "reactive-tech.io/kubegres/api/v1"
)

type Classification string

const (
	Scale                   Classification = "scale"
	PodDelete               Classification = "pod_delete"
	NodeUpgrade             Classification = "node_upgrade"
	NodeMaintenance         Classification = "node_maintenance"
	PrimaryFailure          Classification = "primary_failure"
	PlannedVoluntaryUnknown Classification = "planned_voluntary_unknown"
	Unknown                 Classification = "unknown"
)

type Input struct {
	Now                  time.Time
	KubegresGeneration   int64
	ObservedGeneration   int64
	PodDeletionTimestamp *metav1.Time
	DisruptionReason     string
	PodNodeUID           string
	Primary              bool
	NodeUnreachable      bool
	Operation            *kubegresv1.MaintenanceOperation
}

type Result struct {
	Class  Classification
	Source string
}

func ClassifyPodLoss(input Input) Result {
	if input.KubegresGeneration != 0 && input.ObservedGeneration != 0 && input.KubegresGeneration != input.ObservedGeneration {
		return Result{Class: Scale, Source: "spec_change"}
	}

	planned := input.DisruptionReason == "EvictionByEvictionAPI" || input.DisruptionReason == "PreemptionByScheduler"
	if input.Primary && input.NodeUnreachable && planned {
		return Result{Class: Unknown, Source: "contradictory_observation"}
	}
	if input.Operation != nil && IsActive(input.Operation) && operationMatches(input) {
		switch input.Operation.Spec.Purpose {
		case kubegresv1.NodeUpgrade:
			return Result{Class: NodeUpgrade, Source: "maintenance_operation"}
		case kubegresv1.NodeMaintenance, kubegresv1.NodeRotation:
			return Result{Class: NodeMaintenance, Source: "maintenance_operation"}
		}
	}
	if planned {
		return Result{Class: PlannedVoluntaryUnknown, Source: "pod_disruption"}
	}
	if input.PodDeletionTimestamp != nil {
		return Result{Class: PodDelete, Source: "pod_watch"}
	}
	if input.Primary && input.NodeUnreachable {
		return Result{Class: PrimaryFailure, Source: "node_watch"}
	}
	return Result{Class: Unknown, Source: "insufficient_evidence"}
}

func operationMatches(input Input) bool {
	op := input.Operation
	if op.Spec.TargetNode.UID == "" || op.Spec.TargetNode.UID != input.PodNodeUID {
		return false
	}
	if !op.Spec.ExpiresAt.IsZero() && !input.Now.IsZero() && !input.Now.Before(op.Spec.ExpiresAt.Time) {
		return false
	}
	return input.DisruptionReason == "EvictionByEvictionAPI" || input.DisruptionReason == "PreemptionByScheduler"
}

func IsActive(op *kubegresv1.MaintenanceOperation) bool {
	if op == nil {
		return false
	}
	switch op.Status.Phase {
	case kubegresv1.MaintenancePhaseCompleted, kubegresv1.MaintenancePhaseAborted, kubegresv1.MaintenancePhaseClosed:
		return false
	default:
		return true
	}
}
