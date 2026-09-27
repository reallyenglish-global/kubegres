package v1

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestKubegresSpecResourcesByRole(t *testing.T) {
	common := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
	}
	primary := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
	}
	replica := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("512Mi")},
	}

	spec := KubegresSpec{
		Resources: common,
		Primary:   KubegresRoleSpec{Resources: primary},
		Replica:   KubegresRoleSpec{Resources: replica},
	}

	assertMemoryRequest(t, spec.ResourcesForPrimary(), "4Gi")
	assertMemoryRequest(t, spec.ResourcesForReplica(), "512Mi")
}

func TestKubegresSpecResourcesFallbackToCommon(t *testing.T) {
	common := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
	}
	spec := KubegresSpec{Resources: common}

	assertMemoryRequest(t, spec.ResourcesForPrimary(), "1Gi")
	assertMemoryRequest(t, spec.ResourcesForReplica(), "1Gi")
}

func assertMemoryRequest(t *testing.T, requirements corev1.ResourceRequirements, want string) {
	t.Helper()
	got := requirements.Requests[corev1.ResourceMemory]
	if got.Cmp(resource.MustParse(want)) != 0 {
		t.Fatalf("memory request = %s, want %s", got.String(), want)
	}
}
