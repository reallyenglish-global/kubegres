package statefulset_spec

import (
	"encoding/json"
	"reactive-tech.io/kubegres/internal/controller/ctx"
	"reflect"
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
		name         string
		role         string
		metadataRole string
		want         string
	}{
		{name: "primary", role: "primary", metadataRole: "primary", want: "4Gi"},
		{name: "replica", role: "replica", metadataRole: "replica", want: "512Mi"},
		{name: "primary with stale replica metadata", role: "primary", metadataRole: "replica", want: "4Gi"},
		{name: "replica with stale primary metadata", role: "replica", metadataRole: "primary", want: "512Mi"},
		{name: "primary without metadata role", role: "primary", want: "4Gi"},
		{name: "replica without metadata role", role: "replica", want: "512Mi"},
		{name: "unknown falls back to common", role: "other", metadataRole: "primary", want: "1Gi"},
		{name: "missing template role falls back to common", metadataRole: "primary", want: "1Gi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			statefulSet := &apps.StatefulSet{Spec: apps.StatefulSetSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"replicationRole": tc.role}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{}}}}}}
			if tc.metadataRole != "" {
				statefulSet.Labels = map[string]string{"replicationRole": tc.metadataRole}
			}
			got := resourcesForStatefulSet(spec, statefulSet).Requests[corev1.ResourceMemory]
			if got.Cmp(resource.MustParse(tc.want)) != 0 {
				t.Fatalf("memory request = %s, want %s", got.String(), tc.want)
			}
			statefulSet.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse(tc.want)}}
			enforcer := CreateResourcesSpecEnforcer(ctx.KubegresContext{Kubegres: &postgresV1.Kubegres{Spec: spec}})
			if diff := enforcer.CheckForSpecDifference(statefulSet); diff.SpecName != "" {
				t.Fatalf("metadata drift must not trigger resource rollout: %#v", diff)
			}
		})
	}
}

func TestResourcesEnforcementTracksRoleChangesAndOverrideRemoval(t *testing.T) {
	common := corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")}}
	kg := &postgresV1.Kubegres{Spec: postgresV1.KubegresSpec{Resources: common,
		Primary: postgresV1.KubegresRoleSpec{Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")}}},
		Replica: postgresV1.KubegresRoleSpec{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")}}},
	}}
	enforcer := CreateResourcesSpecEnforcer(ctx.KubegresContext{Kubegres: kg})
	sts := &apps.StatefulSet{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"replicationRole": "replica"}}, Spec: apps.StatefulSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Resources: common}}}}}}
	for _, role := range []string{"replica", "primary", "primary"} {
		sts.Spec.Template.Labels = map[string]string{"replicationRole": role}
		if reflect.DeepEqual(sts.Spec.Template.Spec.Containers[0].Resources, kg.Spec.Primary.Resources) {
			kg.Spec.Primary.Resources = corev1.ResourceRequirements{}
		}
		if diff := enforcer.CheckForSpecDifference(sts); diff.SpecName != "Resources" {
			t.Fatalf("missing resource difference for %s: %#v", role, diff)
		}
		updated, err := enforcer.EnforceSpec(sts)
		if err != nil || !updated {
			t.Fatalf("EnforceSpec = %v, %v", updated, err)
		}
		want := kg.Spec.ResourcesForReplica()
		if role == "primary" {
			want = kg.Spec.ResourcesForPrimary()
		}
		if !reflect.DeepEqual(sts.Spec.Template.Spec.Containers[0].Resources, want) {
			t.Fatalf("resources not enforced for %s", role)
		}
		if diff := enforcer.CheckForSpecDifference(sts); diff.SpecName != "" {
			t.Fatalf("enforcement not idempotent: %#v", diff)
		}
	}
}

func TestDecodedEmptyOverridesDoNotTriggerResourceRollout(t *testing.T) {
	for _, role := range []string{"primary", "replica"} {
		t.Run(role, func(t *testing.T) {
			var spec postgresV1.KubegresSpec
			if err := json.Unmarshal([]byte(`{"resources":{"requests":{"cpu":"1"}},"`+role+`":{"resources":{"requests":{},"limits":{}}}}`), &spec); err != nil {
				t.Fatal(err)
			}
			enforcer := CreateResourcesSpecEnforcer(ctx.KubegresContext{Kubegres: &postgresV1.Kubegres{Spec: spec}})
			sts := &apps.StatefulSet{Spec: apps.StatefulSetSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"replicationRole": role}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1000m")}}}}}}}}
			if diff := enforcer.CheckForSpecDifference(sts); diff.SpecName != "" {
				t.Fatalf("empty override must retain common resources without rollout: %#v", diff)
			}
		})
	}
}

func TestResourcesComparisonUsesQuantityValue(t *testing.T) {
	enforcer := ResourcesSpecEnforcer{}
	current := corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1000m")}}
	expected := corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")}}
	if !enforcer.compareResources(current, expected) {
		t.Fatal("equivalent quantities must not cause repeated rollouts")
	}
}
