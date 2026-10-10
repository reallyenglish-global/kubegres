package v1

import (
	"encoding/json"
	"reflect"
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

func TestDecodedRoleResourcesEmptyFallbackAndReplacement(t *testing.T) {
	common := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
		Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
	}
	for _, role := range []string{"primary", "replica"} {
		for _, override := range []string{`{}`, `{"requests":{}}`, `{"limits":{}}`, `{"requests":{},"limits":{}}`, `{"requests":null,"limits":null}`, `{"requests":{"cpu":"100m"},"limits":{}}`, `{"requests":{},"limits":{"cpu":"2"}}`} {
			t.Run(role+"/"+override, func(t *testing.T) {
				var spec KubegresSpec
				if err := json.Unmarshal([]byte(`{"`+role+`":{"resources":`+override+`}}`), &spec); err != nil {
					t.Fatal(err)
				}
				spec.Resources = common
				var decoded corev1.ResourceRequirements
				if err := json.Unmarshal([]byte(override), &decoded); err != nil {
					t.Fatal(err)
				}
				want := decoded
				if len(decoded.Requests) == 0 && len(decoded.Limits) == 0 {
					want = common
				}
				got := spec.ResourcesForPrimary()
				if role == "replica" {
					got = spec.ResourcesForReplica()
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("resources = %#v, want %#v (replace, not merge)", got, want)
				}
			})
		}
	}
}

func assertMemoryRequest(t *testing.T, requirements corev1.ResourceRequirements, want string) {
	t.Helper()
	got := requirements.Requests[corev1.ResourceMemory]
	if got.Cmp(resource.MustParse(want)) != 0 {
		t.Fatalf("memory request = %s, want %s", got.String(), want)
	}
}
