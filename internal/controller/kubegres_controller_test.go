package controller

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubegresv1 "reactive-tech.io/kubegres/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// The reconciler derives the next StatefulSet index and the active blocking
// operation from Kubegres status, so it must read its own latest status write.
// The informer cache can lag behind that write; the API reader cannot.
func TestGetDeployedKubegresResourcePrefersUncachedReader(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := kubegresv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	key := types.NamespacedName{Namespace: "ns", Name: "db"}
	stale := &kubegresv1.Kubegres{ObjectMeta: metav1.ObjectMeta{Namespace: key.Namespace, Name: key.Name}, Status: kubegresv1.KubegresStatus{LastCreatedInstanceIndex: 1}}
	fresh := &kubegresv1.Kubegres{ObjectMeta: metav1.ObjectMeta{Namespace: key.Namespace, Name: key.Name}, Status: kubegresv1.KubegresStatus{LastCreatedInstanceIndex: 2}}
	cached := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(stale).WithObjects(stale).Build()
	direct := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(fresh).WithObjects(fresh).Build()

	r := &KubegresReconciler{Client: cached, APIReader: direct}
	got, err := r.getDeployedKubegresResource(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.LastCreatedInstanceIndex != 2 {
		t.Fatalf("read stale status %d from the cache instead of the API reader", got.Status.LastCreatedInstanceIndex)
	}

	r = &KubegresReconciler{Client: cached}
	if got, err := r.getDeployedKubegresResource(context.Background(), ctrl.Request{NamespacedName: key}); err != nil || got.Status.LastCreatedInstanceIndex != 1 {
		t.Fatalf("without an API reader the cached client must be used: got %v err %v", got.Status.LastCreatedInstanceIndex, err)
	}

	empty := fake.NewClientBuilder().WithScheme(scheme).Build()
	r = &KubegresReconciler{Client: cached, APIReader: empty}
	if _, err := r.getDeployedKubegresResource(context.Background(), ctrl.Request{NamespacedName: key}); !errors.IsNotFound(err) {
		t.Fatalf("expected not found from the API reader, got %v", err)
	}
}
