package statefulset

import (
	"context"
	"errors"
	"testing"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	v1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
	"reactive-tech.io/kubegres/internal/controller/ctx/log"
	"reactive-tech.io/kubegres/internal/controller/ctx/status"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func newPrimaryEnforcerForPvcTest(t *testing.T, kubegres *v1.Kubegres, getErr error, objects ...client.Object) PrimaryDbCountSpecEnforcer {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := core.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...)
	if getErr != nil {
		builder = builder.WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return getErr
			},
		})
	}
	return PrimaryDbCountSpecEnforcer{kubegresContext: ctx.KubegresContext{
		Kubegres: kubegres,
		Ctx:      context.Background(),
		Client:   builder.Build(),
		Log:      log.LogWrapper{Kubegres: kubegres, Recorder: record.NewFakeRecorder(10)},
		Status:   &status.KubegresStatusWrapper{Kubegres: kubegres},
	}}
}

func newKubegresWithOnePrimaryDeployed() *v1.Kubegres {
	replicas := int32(1)
	return &v1.Kubegres{
		ObjectMeta: metav1.ObjectMeta{Name: "pg", Namespace: "default"},
		Spec:       v1.KubegresSpec{Replicas: &replicas},
		Status:     v1.KubegresStatus{LastCreatedInstanceIndex: 1},
	}
}

func TestPrimaryRecreationReturnsPvcLookupError(t *testing.T) {
	wantErr := errors.New("api server unavailable")
	enforcer := newPrimaryEnforcerForPvcTest(t, newKubegresWithOnePrimaryDeployed(), wantErr)

	_, err := enforcer.getInstanceIndexIfPrimaryNeedsToBeRecreatedAndThereIsNoReplicaSetUp()
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v instead of treating the PVC as absent", err, wantErr)
	}
}

func TestPrimaryRecreationReusesExistingPvc(t *testing.T) {
	pvc := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "postgres-db-pg-1-0", Namespace: "default"}}
	enforcer := newPrimaryEnforcerForPvcTest(t, newKubegresWithOnePrimaryDeployed(), nil, pvc)

	instanceIndex, err := enforcer.getInstanceIndexIfPrimaryNeedsToBeRecreatedAndThereIsNoReplicaSetUp()
	if err != nil || instanceIndex != 1 {
		t.Fatalf("got (%d, %v), want (1, nil) to reuse the existing PVC", instanceIndex, err)
	}
}

func TestPrimaryRecreationUsesNewPvcWhenMissing(t *testing.T) {
	enforcer := newPrimaryEnforcerForPvcTest(t, newKubegresWithOnePrimaryDeployed(), nil)

	instanceIndex, err := enforcer.getInstanceIndexIfPrimaryNeedsToBeRecreatedAndThereIsNoReplicaSetUp()
	if err != nil || instanceIndex != 0 {
		t.Fatalf("got (%d, %v), want (0, nil) when the PVC does not exist", instanceIndex, err)
	}
}

func TestReservedBootstrapReturnsPvcLookupError(t *testing.T) {
	wantErr := errors.New("api server unavailable")
	kubegres := newKubegresWithOnePrimaryDeployed()
	kubegres.Spec.Bootstrap = &v1.KubegresBootstrap{}
	kubegres.Status.BootstrapState = "reserved"
	kubegres.Status.BootstrapPVCName = "postgres-db-pg-1-0"
	enforcer := newPrimaryEnforcerForPvcTest(t, kubegres, wantErr)

	shouldDeploy, err := enforcer.shouldWeDeployNewPrimaryDb()
	if shouldDeploy || !errors.Is(err, wantErr) {
		t.Fatalf("got (%v, %v), want (false, %v)", shouldDeploy, err, wantErr)
	}
}
