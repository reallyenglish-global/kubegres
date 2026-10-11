package statefulset

import (
	"context"
	"errors"
	"testing"

	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	v1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
	"reactive-tech.io/kubegres/internal/controller/ctx/log"
	"reactive-tech.io/kubegres/internal/controller/ctx/status"
	operation2 "reactive-tech.io/kubegres/internal/controller/operation"
	"reactive-tech.io/kubegres/internal/controller/spec/template"
	"reactive-tech.io/kubegres/internal/controller/states"
	statefulsetstates "reactive-tech.io/kubegres/internal/controller/states/statefulset"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestReplicaEnforceReturnsManualFailoverResetError(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	replicas := int32(1)
	kubegres := &v1.Kubegres{
		ObjectMeta: metav1.ObjectMeta{Name: "pg", Namespace: "default"},
		Spec:       v1.KubegresSpec{Replicas: &replicas, Failover: v1.KubegresFailover{PromotePod: "pg-2-0"}},
	}
	wantErr := errors.New("update conflict")
	kubegresClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kubegres).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
				return wantErr
			},
		}).Build()
	kubegresContext := ctx.KubegresContext{
		Kubegres: kubegres,
		Ctx:      context.Background(),
		Client:   kubegresClient,
		Log:      log.LogWrapper{Kubegres: kubegres, Recorder: record.NewFakeRecorder(10)},
		Status:   &status.KubegresStatusWrapper{Kubegres: kubegres},
	}
	resourcesStates := states.ResourcesStates{StatefulSets: statefulsetstates.StatefulSetsStates{
		Primary: statefulsetstates.StatefulSetWrapper{IsDeployed: true, IsReady: true},
	}}
	enforcer := CreateReplicaDbCountSpecEnforcer(kubegresContext, resourcesStates, template.ResourcesCreatorFromTemplate{}, operation2.CreateBlockingOperation(kubegresContext))

	if err := enforcer.Enforce(); !errors.Is(err, wantErr) {
		t.Fatalf("Enforce() error = %v, want %v", err, wantErr)
	}
}

func TestReplicaEnforceContinuesWhenManualFailoverResetFails(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	replicas := int32(2)
	kubegres := &v1.Kubegres{
		ObjectMeta: metav1.ObjectMeta{Name: "pg", Namespace: "default"},
		Spec:       v1.KubegresSpec{Replicas: &replicas, Failover: v1.KubegresFailover{PromotePod: "pg-2-0"}},
		Status:     v1.KubegresStatus{EnforcedReplicas: 2},
	}
	wantErr := errors.New("update rejected")
	deletedReplica := ""
	kubegresClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kubegres).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
				return wantErr
			},
			Delete: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.DeleteOption) error {
				deletedReplica = obj.GetName()
				return nil
			},
		}).Build()
	kubegresContext := ctx.KubegresContext{
		Kubegres: kubegres,
		Ctx:      context.Background(),
		Client:   kubegresClient,
		Log:      log.LogWrapper{Kubegres: kubegres, Recorder: record.NewFakeRecorder(10)},
		Status:   &status.KubegresStatusWrapper{Kubegres: kubegres},
	}
	var deployedReplicas statefulsetstates.StatefulSetWrappers
	deployedReplicas.Add(statefulsetstates.StatefulSetWrapper{
		IsDeployed:    true,
		InstanceIndex: 2,
		StatefulSet:   apps.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "pg-2", Namespace: "default"}},
	})
	resourcesStates := states.ResourcesStates{StatefulSets: statefulsetstates.StatefulSetsStates{
		SpecExpectedNbreToDeploy: 2,
		Primary:                  statefulsetstates.StatefulSetWrapper{IsDeployed: true, IsReady: true},
		Replicas:                 statefulsetstates.Replicas{All: deployedReplicas, NbreDeployed: 1},
	}}
	blockingOperation := operation2.CreateBlockingOperation(kubegresContext)
	enforcer := CreateReplicaDbCountSpecEnforcer(kubegresContext, resourcesStates, template.ResourcesCreatorFromTemplate{}, blockingOperation)
	blockingOperation.AddConfig(enforcer.CreateOperationConfigForReplicaDbUndeploying())

	if err := enforcer.Enforce(); !errors.Is(err, wantErr) {
		t.Fatalf("Enforce() error = %v, want %v", err, wantErr)
	}
	if deletedReplica != "pg-2" {
		t.Fatalf("deleted replica = %q, want the unready replica pg-2 to be undeployed despite the reset failure", deletedReplica)
	}
}
