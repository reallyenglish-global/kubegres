package template

import (
	"reflect"
	"testing"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
)

func TestCreateStatefulSetRoleResources(t *testing.T) {
	common := core.ResourceRequirements{Requests: core.ResourceList{core.ResourceMemory: resource.MustParse("1Gi")}}
	primary := core.ResourceRequirements{Limits: core.ResourceList{core.ResourceCPU: resource.MustParse("2")}}
	replica := core.ResourceRequirements{Requests: core.ResourceList{core.ResourceCPU: resource.MustParse("100m")}}
	for _, overrides := range []bool{false, true} {
		for _, role := range []string{"primary", "replica"} {
			name := role + "/common"
			if overrides {
				name = role + "/override"
			}
			t.Run(name, func(t *testing.T) {
				kg := &v1.Kubegres{ObjectMeta: metav1.ObjectMeta{Name: "pg", Namespace: "test"}, Spec: v1.KubegresSpec{
					Image: "postgres:17", Port: 5432, Database: v1.KubegresDatabase{Size: "1Gi", VolumeMount: "/var/lib/postgresql/data"}, Resources: common,
				}}
				want := common
				if overrides {
					kg.Spec.Primary.Resources = primary
					kg.Spec.Replica.Resources = replica
					if role == "primary" {
						want = primary
					} else {
						want = replica
					}
				}
				creator := CreateResourcesCreatorFromTemplate(ctx.KubegresContext{Kubegres: kg}, CustomConfigSpecHelper{}, ResourceTemplateLoader{})
				create := creator.CreatePrimaryStatefulSet
				if role == "replica" {
					create = creator.CreateReplicaStatefulSet
				}
				sts, err := create(1)
				if err != nil {
					t.Fatal(err)
				}
				if sts.Labels["replicationRole"] != role || sts.Spec.Template.Labels["replicationRole"] != role {
					t.Fatalf("incorrect role labels: %#v", sts)
				}
				if got := sts.Spec.Template.Spec.Containers[0].Resources; !reflect.DeepEqual(got, want) {
					t.Fatalf("resources = %#v, want %#v", got, want)
				}
			})
		}
	}
}
