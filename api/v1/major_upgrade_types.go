package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// MajorUpgradeStrategy selects the user-controlled data migration mechanism.
type MajorUpgradeStrategy string

const (
	MajorUpgradeLogicalReplication MajorUpgradeStrategy = "logical-replication"
	MajorUpgradeDumpRestore        MajorUpgradeStrategy = "dump-restore"
)

// MajorUpgradePhase describes the controller-visible lifecycle of a planned upgrade.
type MajorUpgradePhase string

const (
	MajorUpgradePending      MajorUpgradePhase = "Pending"
	MajorUpgradeRunning      MajorUpgradePhase = "Running"
	MajorUpgradeCutoverReady MajorUpgradePhase = "CutoverReady"
	MajorUpgradeCompleted    MajorUpgradePhase = "Completed"
	MajorUpgradeAborted      MajorUpgradePhase = "Aborted"
)

// MajorUpgradeHook is deliberately a small, transparent execution contract.
// Kubegres does not invent pg_upgrade, logical-replication, or application
// cutover commands; the owner supplies those commands or a ConfigMap script.
type MajorUpgradeHook struct {
	// Image is the image in which the hook runs.
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// Command and Args are passed to the hook container.
	Command []string `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	// Script mounts one ConfigMap key as a file. This is the recommended way to
	// keep deployment-specific credentials, endpoints, and policy outside the CR.
	Script *KubegresCronTaskScript `json:"script,omitempty"`
}

// MajorUpgradeSpec describes a separate destination cluster and an explicit
// operator-controlled migration. Changing Kubegres.spec.image remains a minor
// upgrade operation and cannot start this workflow.
type MajorUpgradeSpec struct {
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:MinLength=1
	SourceKubegres string `json:"sourceKubegres"`
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:MinLength=1
	SourceImage string `json:"sourceImage"`
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:MinLength=1
	TargetImage string `json:"targetImage"`
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=logical-replication;dump-restore
	Strategy MajorUpgradeStrategy `json:"strategy"`
	// Hooks run in order. The operator records their lifecycle but does not
	// replace their commands or silently mutate the source cluster.
	//+kubebuilder:validation:Required
	Preflight MajorUpgradeHook `json:"preflight"`
	//+kubebuilder:validation:Required
	Bootstrap MajorUpgradeHook `json:"bootstrap"`
	//+kubebuilder:validation:Required
	Copy      MajorUpgradeHook `json:"copy"`
	//+kubebuilder:validation:Required
	Validate  MajorUpgradeHook `json:"validate"`
	//+kubebuilder:validation:Required
	Cutover   MajorUpgradeHook `json:"cutover"`
	Cleanup   *MajorUpgradeHook `json:"cleanup,omitempty"`
}

type MajorUpgradeStatus struct {
	Phase             MajorUpgradePhase `json:"phase,omitempty"`
	CurrentHook       string            `json:"currentHook,omitempty"`
	ObservedGeneration int64            `json:"observedGeneration,omitempty"`
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=pgmajor
// +kubebuilder:printcolumn:name="Source",type="string",JSONPath=".spec.sourceKubegres"
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.targetImage"
// +kubebuilder:printcolumn:name="Strategy",type="string",JSONPath=".spec.strategy"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
type MajorUpgrade struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec   MajorUpgradeSpec   `json:"spec"`
	Status MajorUpgradeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type MajorUpgradeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items []MajorUpgrade `json:"items"`
}

func init() { SchemeBuilder.Register(&MajorUpgrade{}, &MajorUpgradeList{}) }
