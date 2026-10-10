package statefulset

import (
	"context"
	"errors"
	"testing"

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
