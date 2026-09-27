/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ----------------------- SPEC -------------------------------------------

type KubegresDatabase struct {
	Size             string  `json:"size,omitempty"`
	VolumeMount      string  `json:"volumeMount,omitempty"`
	StorageClassName *string `json:"storageClassName,omitempty"`
}

type KubegresBackUp struct {
	Schedule       string `json:"schedule,omitempty"`
	VolumeMount    string `json:"volumeMount,omitempty"`
	PvcName        string `json:"pvcName,omitempty"`
	ArchiveCommand string `json:"archiveCommand,omitempty"`
	RestoreCommand string `json:"restoreCommand,omitempty"`
	// Size requests a generic ephemeral PVC for the backup Pod when PvcName is
	// empty or names a PVC that does not exist. The ephemeral PVC is deleted
	// with the Pod, so backup scripts must copy data to durable storage.
	//+kubebuilder:validation:Pattern=`^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$`
	Size string `json:"size,omitempty"`
	// Image is the container image used by the backup CronJob. When empty,
	// the database image is used for backwards compatibility.
	Image string `json:"image,omitempty"`
	// TimeZone is the IANA time zone (for example "America/New_York") passed
	// to the CronJob's spec.timeZone. When unset, the CronJob controller
	// default applies.
	TimeZone *string `json:"timeZone,omitempty"`
}

// KubegresCronTaskScript mounts one ConfigMap key as a file in a CronTask container.
type KubegresCronTaskScript struct {
	// ConfigMapName is the ConfigMap holding the script.
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:MinLength=1
	ConfigMapName string `json:"configMapName"`
	// Key is the ConfigMap key whose content is mounted.
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:MinLength=1
	Key string `json:"key"`
	// MountPath is the file path the script is mounted at in the container.
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:MinLength=1
	MountPath string `json:"mountPath"`
}

// KubegresCronTask defines an independent, Kubegres-owned CronJob. It is
// intentionally separate from Backup so existing backup behaviour is unchanged.
type KubegresCronTask struct {
	// Name identifies the task and forms the CronJob name. It must be a
	// DNS-1123 label and unique within cronTasks.
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:MinLength=1
	//+kubebuilder:validation:MaxLength=63
	//+kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`
	// Schedule is the cron expression passed to the CronJob.
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:MinLength=1
	Schedule string `json:"schedule"`
	// Image is the container image the task runs.
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// Command overrides the image entrypoint.
	Command []string `json:"command,omitempty"`
	// Args are passed to the entrypoint.
	Args []string `json:"args,omitempty"`
	// Script mounts one ConfigMap key as a file in the task container.
	Script *KubegresCronTaskScript `json:"script,omitempty"`
	// Env is added to the task container.
	Env []v1.EnvVar `json:"env,omitempty"`
	// Volumes are added to the task Pod.
	Volumes []v1.Volume `json:"volumes,omitempty"`
	// VolumeMounts are added to the task container.
	VolumeMounts []v1.VolumeMount `json:"volumeMounts,omitempty"`
	// ConcurrencyPolicy is the CronJob concurrency policy.
	//+kubebuilder:validation:Enum=Allow;Forbid;Replace
	ConcurrencyPolicy string `json:"concurrencyPolicy,omitempty"`
	// SuccessfulJobsHistoryLimit is passed through to the CronJob.
	//+kubebuilder:validation:Minimum=0
	SuccessfulJobsHistoryLimit *int32 `json:"successfulJobsHistoryLimit,omitempty"`
	// FailedJobsHistoryLimit is passed through to the CronJob.
	//+kubebuilder:validation:Minimum=0
	FailedJobsHistoryLimit *int32 `json:"failedJobsHistoryLimit,omitempty"`
	// TimeZone is the IANA time zone (for example "America/New_York") passed
	// to the CronJob's spec.timeZone. When unset, the CronJob controller
	// default applies.
	TimeZone *string `json:"timeZone,omitempty"`
}

type KubegresFailover struct {
	IsDisabled bool   `json:"isDisabled,omitempty"`
	PromotePod string `json:"promotePod,omitempty"`
	// OnPrimaryPodDrain promotes a Ready replica when Kubernetes marks the primary
	// for voluntary disruption (for example, a node drain eviction).
	OnPrimaryPodDrain bool `json:"onPrimaryPodDrain,omitempty"`
}

// KubegresPodDisruptionBudget controls the PodDisruptionBudget Kubegres
// manages for the primary. It is also created when
// spec.failover.onPrimaryPodDrain is true.
type KubegresPodDisruptionBudget struct {
	// Enabled creates the PodDisruptionBudget even when drain failover is off.
	Enabled bool `json:"enabled,omitempty"`
	// MinAvailable is the PodDisruptionBudget minAvailable value.
	//+kubebuilder:validation:Minimum=0
	MinAvailable *int32 `json:"minAvailable,omitempty"`
	// UnhealthyPodEvictionPolicy is the PodDisruptionBudget unhealthyPodEvictionPolicy value.
	// AlwaysAllow lets a drain evict an already-unhealthy primary so drain-aware
	// failover can proceed. When unset, the Kubernetes API server default applies.
	//+kubebuilder:validation:Enum=IfHealthyBudget;AlwaysAllow
	UnhealthyPodEvictionPolicy *string `json:"unhealthyPodEvictionPolicy,omitempty"`
}

type KubegresScheduler struct {
	Affinity    *v1.Affinity    `json:"affinity,omitempty"`
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
}

// KubegresRoleSpec contains configuration that applies to one PostgreSQL role.
// An empty role-specific resources value falls back to KubegresSpec.Resources.
type KubegresRoleSpec struct {
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

type VolumeClaimTemplate struct {
	Name string                       `json:"name,omitempty"`
	Spec v1.PersistentVolumeClaimSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

type Volume struct {
	VolumeMounts         []v1.VolumeMount      `json:"volumeMounts,omitempty"`
	Volumes              []v1.Volume           `json:"volumes,omitempty"`
	VolumeClaimTemplates []VolumeClaimTemplate `json:"volumeClaimTemplates,omitempty"`
}

type Probe struct {
	LivenessProbe  *v1.Probe `json:"livenessProbe,omitempty"`
	ReadinessProbe *v1.Probe `json:"readinessProbe,omitempty"`
	// StartupProbe replaces the startup probe Kubegres sets on the
	// PostgreSQL container.
	StartupProbe *v1.Probe `json:"startupProbe,omitempty"`
}

// Lifecycle overrides container lifecycle hooks on the PostgreSQL container.
type Lifecycle struct {
	// PreStop replaces the preStop hook Kubegres sets on the PostgreSQL
	// container.
	PreStop *v1.LifecycleHandler `json:"preStop,omitempty"`
}

type KubegresSpec struct {
	Replicas            *int32                      `json:"replicas,omitempty"`
	Image               string                      `json:"image,omitempty"`
	Port                int32                       `json:"port,omitempty"`
	ImagePullSecrets    []v1.LocalObjectReference   `json:"imagePullSecrets,omitempty"`
	CustomConfig        string                      `json:"customConfig,omitempty"`
	Database            KubegresDatabase            `json:"database,omitempty"`
	Failover            KubegresFailover            `json:"failover,omitempty"`
	PodDisruptionBudget KubegresPodDisruptionBudget `json:"podDisruptionBudget,omitempty"`
	Backup              KubegresBackUp              `json:"backup,omitempty"`
	//+listType=map
	//+listMapKey=name
	CronTasks                []KubegresCronTask      `json:"cronTasks,omitempty"`
	Env                      []v1.EnvVar             `json:"env,omitempty"`
	Scheduler                KubegresScheduler       `json:"scheduler,omitempty"`
	Resources                v1.ResourceRequirements `json:"resources,omitempty"`
	Primary                  KubegresRoleSpec        `json:"primary,omitempty"`
	Replica                  KubegresRoleSpec        `json:"replica,omitempty"`
	Volume                   Volume                  `json:"volume,omitempty"`
	SecurityContext          *v1.PodSecurityContext  `json:"securityContext,omitempty"`
	ContainerSecurityContext *v1.SecurityContext     `json:"containerSecurityContext,omitempty"`
	Probe                    Probe                   `json:"probe,omitempty"`
	Lifecycle                Lifecycle               `json:"lifecycle,omitempty"`
	ServiceAccountName       string                  `json:"serviceAccountName,omitempty"`
}

// ResourcesForPrimary returns the primary-specific requirements when set, or
// the backwards-compatible common requirements otherwise.
func (s KubegresSpec) ResourcesForPrimary() v1.ResourceRequirements {
	if s.Primary.Resources.Requests != nil || s.Primary.Resources.Limits != nil {
		return s.Primary.Resources
	}
	return s.Resources
}

// ResourcesForReplica returns the replica-specific requirements when set, or
// the backwards-compatible common requirements otherwise.
func (s KubegresSpec) ResourcesForReplica() v1.ResourceRequirements {
	if s.Replica.Resources.Requests != nil || s.Replica.Resources.Limits != nil {
		return s.Replica.Resources
	}
	return s.Resources
}

// ----------------------- STATUS -----------------------------------------

type KubegresStatefulSetOperation struct {
	InstanceIndex int32  `json:"instanceIndex,omitempty"`
	Name          string `json:"name,omitempty"`
}

type KubegresStatefulSetSpecUpdateOperation struct {
	SpecDifferences string `json:"specDifferences,omitempty"`
}

type KubegresBlockingOperation struct {
	OperationId          string `json:"operationId,omitempty"`
	StepId               string `json:"stepId,omitempty"`
	TimeOutEpocInSeconds int64  `json:"timeOutEpocInSeconds,omitempty"`
	HasTimedOut          bool   `json:"hasTimedOut,omitempty"`

	// Custom operation fields
	StatefulSetOperation           KubegresStatefulSetOperation           `json:"statefulSetOperation,omitempty"`
	StatefulSetSpecUpdateOperation KubegresStatefulSetSpecUpdateOperation `json:"statefulSetSpecUpdateOperation,omitempty"`
}

type KubegresStatus struct {
	LastCreatedInstanceIndex  int32                     `json:"lastCreatedInstanceIndex,omitempty"`
	BlockingOperation         KubegresBlockingOperation `json:"blockingOperation,omitempty"`
	PreviousBlockingOperation KubegresBlockingOperation `json:"previousBlockingOperation,omitempty"`
	EnforcedReplicas          int32                     `json:"enforcedReplicas,omitempty"`
}

// ----------------------- RESOURCE ---------------------------------------

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.replicas"
//+kubebuilder:printcolumn:name="Image",type="string",JSONPath=".spec.image"
//+kubebuilder:printcolumn:name="Operation",type="string",JSONPath=".status.blockingOperation.operationId"
//+kubebuilder:printcolumn:name="Step",type="string",JSONPath=".status.blockingOperation.stepId"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Kubegres is the Schema for the kubegres API
type Kubegres struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubegresSpec   `json:"spec,omitempty"`
	Status KubegresStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KubegresList contains a list of Kubegres
type KubegresList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Kubegres `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Kubegres{}, &KubegresList{})
}
