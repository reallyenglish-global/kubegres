package maintenance

import (
	"strconv"
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

// DisruptionTarget condition reasons published by Kubernetes. Planned reasons
// describe voluntary disruption; involuntary reasons describe the node or
// kubelet removing a Pod and must never be treated as a direct deletion.
const (
	ReasonEvictionByEvictionAPI  = "EvictionByEvictionAPI"
	ReasonPreemptionByScheduler  = "PreemptionByScheduler"
	ReasonDeletionByTaintManager = "DeletionByTaintManager"
	ReasonTerminationByKubelet   = "TerminationByKubelet"
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

// Result carries the evidence-backed classification. Evidence records the
// observations the decision was made from so status and events can cite them.
type Result struct {
	Class    Classification
	Source   string
	Evidence map[string]string
}

func ClassifyPodLoss(input Input) Result {
	evidence := evidenceFor(input)
	result := func(class Classification, source string) Result {
		return Result{Class: class, Source: source, Evidence: evidence}
	}

	if input.KubegresGeneration != 0 && input.ObservedGeneration != 0 && input.KubegresGeneration != input.ObservedGeneration {
		return result(Scale, "spec_change")
	}

	planned := isPlannedDisruption(input.DisruptionReason)
	involuntary := isInvoluntaryDisruption(input.DisruptionReason)

	if input.Primary && input.NodeUnreachable && planned {
		return result(Unknown, "contradictory_observation")
	}
	if input.Operation != nil && IsActive(input.Operation) && operationMatches(input) {
		switch input.Operation.Spec.Purpose {
		case kubegresv1.NodeUpgrade:
			return result(NodeUpgrade, "maintenance_operation")
		case kubegresv1.NodeMaintenance, kubegresv1.NodeRotation:
			return result(NodeMaintenance, "maintenance_operation")
		}
	}
	if planned {
		return result(PlannedVoluntaryUnknown, "pod_disruption")
	}
	// Involuntary evidence must be checked before the deletion timestamp: a
	// primary on an unreachable node is usually deleted by the taint manager,
	// which also sets deletionTimestamp.
	if input.Primary && input.NodeUnreachable {
		return result(PrimaryFailure, "node_watch")
	}
	if input.Primary && involuntary {
		return result(PrimaryFailure, "pod_disruption")
	}
	if involuntary {
		return result(Unknown, "involuntary_disruption")
	}
	if input.PodDeletionTimestamp != nil {
		return result(PodDelete, "pod_watch")
	}
	return result(Unknown, "insufficient_evidence")
}

func isPlannedDisruption(reason string) bool {
	return reason == ReasonEvictionByEvictionAPI || reason == ReasonPreemptionByScheduler
}

func isInvoluntaryDisruption(reason string) bool {
	return reason == ReasonDeletionByTaintManager || reason == ReasonTerminationByKubelet
}

func operationMatches(input Input) bool {
	op := input.Operation
	if op.Spec.TargetNode.UID == "" || op.Spec.TargetNode.UID != input.PodNodeUID {
		return false
	}
	if !op.Spec.ExpiresAt.IsZero() && !input.Now.IsZero() && !input.Now.Before(op.Spec.ExpiresAt.Time) {
		return false
	}
	return isPlannedDisruption(input.DisruptionReason) || isInvoluntaryDisruption(input.DisruptionReason)
}

func evidenceFor(input Input) map[string]string {
	evidence := map[string]string{
		"primary":          strconv.FormatBool(input.Primary),
		"node_unreachable": strconv.FormatBool(input.NodeUnreachable),
	}
	if input.DisruptionReason != "" {
		evidence["disruption_reason"] = input.DisruptionReason
	}
	if input.PodNodeUID != "" {
		evidence["node_uid"] = input.PodNodeUID
	}
	if input.PodDeletionTimestamp != nil {
		evidence["pod_deletion_timestamp"] = input.PodDeletionTimestamp.UTC().Format(time.RFC3339)
	}
	if input.KubegresGeneration != 0 || input.ObservedGeneration != 0 {
		evidence["kubegres_generation"] = strconv.FormatInt(input.KubegresGeneration, 10)
		evidence["observed_generation"] = strconv.FormatInt(input.ObservedGeneration, 10)
	}
	if input.Operation != nil {
		evidence["maintenance_operation"] = input.Operation.Name
		evidence["maintenance_operation_phase"] = string(input.Operation.Status.Phase)
	}
	return evidence
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
