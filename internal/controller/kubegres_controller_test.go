package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubegresv1 "reactive-tech.io/kubegres/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func newReconcilerForGetTest(t *testing.T, getErr error) *KubegresReconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := kubegresv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	builder := fake.NewClientBuilder().WithScheme(scheme)
	if getErr != nil {
		builder = builder.WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return getErr
			},
		})
	}
	return &KubegresReconciler{Client: builder.Build(), Logger: logr.Discard(), Scheme: scheme}
}

func TestReconcileReturnsTransientGetError(t *testing.T) {
	wantErr := errors.New("api server unavailable")
	r := newReconcilerForGetTest(t, wantErr)

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "db"}})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Reconcile() error = %v, want %v so the request is retried", err, wantErr)
	}
}

func TestReconcileIgnoresMissingKubegres(t *testing.T) {
	r := newReconcilerForGetTest(t, nil)

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "db"}})
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil for a deleted Kubegres", err)
	}
	if result != (ctrl.Result{}) {
		t.Fatalf("Reconcile() result = %+v, want empty result", result)
	}
}
