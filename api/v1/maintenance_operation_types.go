package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// MaintenancePurpose identifies the approved reason for a planned node operation.
type MaintenancePurpose string

const (
	NodeUpgrade     MaintenancePurpose = "node-upgrade"
	NodeMaintenance MaintenancePurpose = "node-maintenance"
	NodeRotation    MaintenancePurpose = "node-rotation"
)

type MaintenancePhase string

const (
	MaintenancePhasePending              MaintenancePhase = "Pending"
	MaintenancePhaseRelocatingReplica    MaintenancePhase = "RelocatingReplica"
	MaintenancePhaseAwaitingPrimaryDrain MaintenancePhase = "AwaitingPrimaryDrain"
	MaintenancePhaseFencing              MaintenancePhase = "Fencing"
	MaintenancePhasePromoting            MaintenancePhase = "Promoting"
	MaintenancePhaseRejoining            MaintenancePhase = "Rejoining"
	MaintenancePhaseCompleted            MaintenancePhase = "Completed"
	MaintenancePhaseAborted              MaintenancePhase = "Aborted"
	MaintenancePhaseClosed               MaintenancePhase = "Closed"
	MaintenancePhaseManualIntervention   MaintenancePhase = "ManualIntervention"
)

type MaintenanceNodeReference struct {
	Name string `json:"name,omitempty"`
	// UID is immutable and is the authoritative target identity.
	//+kubebuilder:validation:MinLength=1
	UID string `json:"uid"`
}

type MaintenanceKubegresReference struct {
	Name string `json:"name"`
}

type MaintenanceSafetyPolicy struct {
	MaxReplayLagSeconds       int32 `json:"maxReplayLagSeconds,omitempty"`
	StableForSeconds          int32 `json:"stableForSeconds,omitempty"`
	RelocationDeadlineSeconds int32 `json:"relocationDeadlineSeconds,omitempty"`
}

// MaintenanceOperationSpec declares an approved, expiring planned operation.
type MaintenanceOperationSpec struct {
	KubegresRef MaintenanceKubegresReference `json:"kubegresRef"`
	//+kubebuilder:validation:Enum=node-upgrade;node-maintenance;node-rotation
	Purpose    MaintenancePurpose       `json:"purpose"`
	TargetNode MaintenanceNodeReference `json:"targetNode"`
	ExpiresAt  metav1.Time              `json:"expiresAt"`
	Safety     MaintenanceSafetyPolicy  `json:"safety,omitempty"`
	// RequesterIdentity preserves the human or workflow identity behind the
	// approved service-account request.
	RequesterIdentity string `json:"requesterIdentity,omitempty"`
	ApprovalReference string `json:"approvalReference,omitempty"`
}

type MaintenanceOperationHistoryEntry struct {
	At       metav1.Time       `json:"at"`
	Phase    MaintenancePhase  `json:"phase"`
	Message  string            `json:"message,omitempty"`
	Evidence map[string]string `json:"evidence,omitempty"`
}

type MaintenanceOperationStatus struct {
	Phase                      MaintenancePhase                   `json:"phase,omitempty"`
	OperationID                string                             `json:"operationID,omitempty"`
	Classification             string                             `json:"classification,omitempty"`
	ObservedKubegresGeneration int64                              `json:"observedKubegresGeneration,omitempty"`
	DesiredReplicas            int32                              `json:"desiredReplicas,omitempty"`
	Conditions                 []metav1.Condition                 `json:"conditions,omitempty"`
	History                    []MaintenanceOperationHistoryEntry `json:"history,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=maintop
// +kubebuilder:printcolumn:name="Kubegres",type="string",JSONPath=".spec.kubegresRef.name"
// +kubebuilder:printcolumn:name="Purpose",type="string",JSONPath=".spec.purpose"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Expires",type="date",JSONPath=".spec.expiresAt"
type MaintenanceOperation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MaintenanceOperationSpec   `json:"spec,omitempty"`
	Status            MaintenanceOperationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type MaintenanceOperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MaintenanceOperation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MaintenanceOperation{}, &MaintenanceOperationList{})
}
