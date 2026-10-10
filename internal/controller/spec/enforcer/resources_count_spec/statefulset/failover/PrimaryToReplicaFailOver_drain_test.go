package failover

import (
	"context"
	"errors"
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	v1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
	"reactive-tech.io/kubegres/internal/controller/ctx/log"
	"reactive-tech.io/kubegres/internal/controller/ctx/status"
	operation2 "reactive-tech.io/kubegres/internal/controller/operation"
	"reactive-tech.io/kubegres/internal/controller/states"
	"reactive-tech.io/kubegres/internal/controller/states/statefulset"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIsVoluntaryDisruption(t *testing.T) {
	deletion := metav1.Now()
	cases := []struct {
		name     string
		pod      core.Pod
		expected bool
	}{
		{"ordinary pod", core.Pod{}, false},
		{"deleted without disruption condition", core.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletion}}, false},
		{"eviction", core.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletion}, Status: core.PodStatus{Conditions: []core.PodCondition{{Type: core.DisruptionTarget, Status: core.ConditionTrue, Reason: "EvictionByEvictionAPI"}}}}, true},
		{"preemption", core.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletion}, Status: core.PodStatus{Conditions: []core.PodCondition{{Type: core.DisruptionTarget, Status: core.ConditionTrue, Reason: "PreemptionByScheduler"}}}}, true},
		{"condition false", core.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletion}, Status: core.PodStatus{Conditions: []core.PodCondition{{Type: core.DisruptionTarget, Status: core.ConditionFalse, Reason: "EvictionByEvictionAPI"}}}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isVoluntaryDisruption(tc.pod); got != tc.expected {
				t.Fatalf("isVoluntaryDisruption() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestSelectReplicaToPromoteSkipsDisruptedReadyReplica(t *testing.T) {
	var replicas statefulset.StatefulSetWrappers
	replicas.Add(statefulset.StatefulSetWrapper{
		InstanceIndex: 1,
		IsReady:       true,
		Pod: statefulset.PodWrapper{
			IsReady:                     true,
			IsBeingVoluntarilyDisrupted: true,
		},
	})
	replicas.Add(statefulset.StatefulSetWrapper{
		InstanceIndex: 2,
		IsReady:       true,
		Pod:           statefulset.PodWrapper{IsReady: true},
	})

	failover := PrimaryToReplicaFailOver{
		kubegresContext: ctx.KubegresContext{Kubegres: &v1.Kubegres{}},
		resourcesStates: states.ResourcesStates{
			StatefulSets: statefulset.StatefulSetsStates{
				Replicas: statefulset.Replicas{All: replicas},
			},
		},
	}

	selected, err := failover.selectReplicaToPromote()
	if err != nil {
		t.Fatalf("selectReplicaToPromote() returned error: %v", err)
	}
	if selected.InstanceIndex != 2 {
		t.Fatalf("selectReplicaToPromote() selected instance %d, want 2", selected.InstanceIndex)
	}
}

type deleteErrorClient struct {
	client.Client
	err error
}

func (c deleteErrorClient) Delete(context.Context, client.Object, ...client.DeleteOption) error {
	return c.err
}

func TestDeletePrimaryStatefulSetReturnsDeleteError(t *testing.T) {
	wantErr := errors.New("delete failed")
	failover := PrimaryToReplicaFailOver{
		kubegresContext: ctx.KubegresContext{
			Ctx:    context.Background(),
			Client: deleteErrorClient{err: wantErr},
		},
		resourcesStates: states.ResourcesStates{
			StatefulSets: statefulset.StatefulSetsStates{
				Primary: statefulset.StatefulSetWrapper{
					StatefulSet: appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "primary"}},
				},
			},
		},
	}

	if err := failover.deletePrimaryStatefulSet(); !errors.Is(err, wantErr) {
		t.Fatalf("deletePrimaryStatefulSet() error = %v, want %v", err, wantErr)
	}
}

func TestDeletePrimaryStatefulSetTreatsMissingPrimaryAsAlreadyDeleted(t *testing.T) {
	failover := PrimaryToReplicaFailOver{
		resourcesStates: states.ResourcesStates{
			StatefulSets: statefulset.StatefulSetsStates{},
		},
	}

	if err := failover.deletePrimaryStatefulSet(); err != nil {
		t.Fatalf("deletePrimaryStatefulSet() error = %v, want nil", err)
	}
}

func TestPromoteReplicaToPrimaryAppliesPrimaryResources(t *testing.T) {
	for _, mode := range []string{"override", "common fallback", "empty fallback"} {
		t.Run(mode, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := appsv1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}

			kubegres := &v1.Kubegres{
				ObjectMeta: metav1.ObjectMeta{Name: "pg", Namespace: "default"},
				Spec: v1.KubegresSpec{
					Primary: v1.KubegresRoleSpec{Resources: core.ResourceRequirements{
						Requests: core.ResourceList{
							core.ResourceCPU:    resource.MustParse("500m"),
							core.ResourceMemory: resource.MustParse("4Gi"),
						},
						Limits: core.ResourceList{
							core.ResourceCPU:    resource.MustParse("1"),
							core.ResourceMemory: resource.MustParse("8Gi"),
						},
					}},
					Replica: v1.KubegresRoleSpec{Resources: core.ResourceRequirements{
						Requests: core.ResourceList{
							core.ResourceCPU:    resource.MustParse("100m"),
							core.ResourceMemory: resource.MustParse("512Mi"),
						},
						Limits: core.ResourceList{
							core.ResourceCPU:    resource.MustParse("250m"),
							core.ResourceMemory: resource.MustParse("1Gi"),
						},
					}},
				},
			}
			if mode != "override" {
				kubegres.Spec.Primary.Resources = core.ResourceRequirements{}
			}
			if mode == "common fallback" {
				kubegres.Spec.Resources = core.ResourceRequirements{Requests: core.ResourceList{core.ResourceMemory: resource.MustParse("2Gi")}}
			}
			statefulSet := appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "pg-1", Namespace: "default", Labels: map[string]string{"replicationRole": "replica"}},
				Spec: appsv1.StatefulSetSpec{Template: core.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"replicationRole": "replica"}},
					Spec:       core.PodSpec{Containers: []core.Container{{Name: "postgres", Resources: kubegres.Spec.ResourcesForReplica()}}},
				}},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&statefulSet).Build()
			kubegresContext := ctx.KubegresContext{
				Kubegres: kubegres,
				Ctx:      context.Background(),
				Client:   fakeClient,
				Log:      log.LogWrapper{Kubegres: kubegres, Recorder: record.NewFakeRecorder(10)},
				Status:   &status.KubegresStatusWrapper{Kubegres: kubegres},
			}
			blockingOperation := operation2.CreateBlockingOperation(kubegresContext)
			failover := CreatePrimaryToReplicaFailOver(kubegresContext, states.ResourcesStates{}, blockingOperation)
			blockingOperation.AddConfig(failover.CreateOperationConfigForFailingOver())

			if err := failover.promoteReplicaToPrimaryFor(
				statefulset.StatefulSetWrapper{InstanceIndex: 1, StatefulSet: statefulSet},
				operation2.OperationIdPrimaryDbCountSpecEnforcement,
				operation2.OperationStepIdPrimaryDbFailingOver,
			); err != nil {
				t.Fatalf("promoteReplicaToPrimaryFor() error = %v", err)
			}

			var promoted appsv1.StatefulSet
			if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: "pg-1", Namespace: "default"}, &promoted); err != nil {
				t.Fatalf("failed to read promoted StatefulSet: %v", err)
			}
			got := promoted.Spec.Template.Spec.Containers[0].Resources
			want := kubegres.Spec.ResourcesForPrimary()
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("promoted resources = %#v, want primary resources %#v", got, want)
			}
			if promoted.Labels["replicationRole"] != "primary" || promoted.Spec.Template.Labels["replicationRole"] != "primary" {
				t.Fatal("promotion did not update both role labels")
			}
		})
	}
}

type updateErrorClient struct {
	client.Client
	err error
}

func (c updateErrorClient) Update(context.Context, client.Object, ...client.UpdateOption) error {
	return c.err
}

func TestPromoteReplicaToPrimaryReturnsUpdateError(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	kubegres := &v1.Kubegres{ObjectMeta: metav1.ObjectMeta{Name: "pg", Namespace: "default"}}
	statefulSet := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "pg-1", Namespace: "default", Labels: map[string]string{"replicationRole": "replica"}},
		Spec: appsv1.StatefulSetSpec{Template: core.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"replicationRole": "replica"}},
			Spec:       core.PodSpec{Containers: []core.Container{{Name: "postgres"}}},
		}},
	}
	wantErr := errors.New("update failed")
	kubegresContext := ctx.KubegresContext{
		Kubegres: kubegres,
		Ctx:      context.Background(),
		Client:   updateErrorClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&statefulSet).Build(), err: wantErr},
		Log:      log.LogWrapper{Kubegres: kubegres, Recorder: record.NewFakeRecorder(10)},
		Status:   &status.KubegresStatusWrapper{Kubegres: kubegres},
	}
	blockingOperation := operation2.CreateBlockingOperation(kubegresContext)
	failover := CreatePrimaryToReplicaFailOver(kubegresContext, states.ResourcesStates{}, blockingOperation)
	blockingOperation.AddConfig(failover.CreateOperationConfigForFailingOver())

	err := failover.promoteReplicaToPrimaryFor(
		statefulset.StatefulSetWrapper{InstanceIndex: 1, StatefulSet: statefulSet},
		operation2.OperationIdPrimaryDbCountSpecEnforcement,
		operation2.OperationStepIdPrimaryDbFailingOver,
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("promoteReplicaToPrimaryFor() error = %v, want %v", err, wantErr)
	}
	if active := blockingOperation.GetActiveOperation(); active.OperationId != "" {
		t.Fatalf("active blocking operation = %q, want none after a failed promotion", active.OperationId)
	}
}
