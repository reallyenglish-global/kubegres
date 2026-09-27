package statefulset_spec

import (
	"testing"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	postgresV1 "reactive-tech.io/kubegres/api/v1"
)

func TestResourcesForStatefulSetUsesRoleSpecificRequirements(t *testing.T) {
	spec := postgresV1.KubegresSpec{
		Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		}},
		Primary: postgresV1.KubegresRoleSpec{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		}}},
		Replica: postgresV1.KubegresRoleSpec{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		}}},
	}

	cases := []struct {
		name string
		role string
		want string
	}{
		{name: "primary", role: "primary", want: "4Gi"},
		{name: "replica", role: "replica", want: "512Mi"},
		{name: "unknown falls back to common", role: "other", want: "1Gi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			statefulSet := &apps.StatefulSet{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"replicationRole": tc.role}}}
			got := resourcesForStatefulSet(spec, statefulSet).Requests[corev1.ResourceMemory]
			if got.Cmp(resource.MustParse(tc.want)) != 0 {
				t.Fatalf("memory request = %s, want %s", got.String(), tc.want)
			}
		})
	}
}
