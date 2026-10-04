package pvccleanup

import (
	"context"
	"testing"
	"time"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	postgresv1 "reactive-tech.io/kubegres/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPrepareForStatefulSetDeletionAfterAnnotatesOwnedPVC(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := apps.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := core.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	ss := &apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "postgres-1", Namespace: "default", UID: "ss-uid"},
		Spec:       apps.StatefulSetSpec{VolumeClaimTemplates: []core.PersistentVolumeClaim{{ObjectMeta: metav1.ObjectMeta{Name: "postgres-db"}}}},
	}
	pvc := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Name: "postgres-db-postgres-1-0", Namespace: "default",
		OwnerReferences: []metav1.OwnerReference{{Kind: "StatefulSet", Name: ss.Name}},
	}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	volume := postgresv1.Volume{PvcCleanupPolicy: postgresv1.PvcCleanupPolicyAfter, PvcCleanupDuration: &metav1.Duration{Duration: 24 * time.Hour}}
	if err := PrepareForStatefulSetDeletion(context.Background(), c, ss, volume, now); err != nil {
		t.Fatal(err)
	}
	if got := ss.Spec.PersistentVolumeClaimRetentionPolicy.WhenDeleted; got != apps.RetainPersistentVolumeClaimRetentionPolicyType {
		t.Fatalf("WhenDeleted = %q, want Retain", got)
	}
	var updated core.PersistentVolumeClaim
	if err := c.Get(context.Background(), clientKey(pvc), &updated); err != nil {
		t.Fatal(err)
	}
	if got := updated.Annotations[CleanupAtAnnotation]; got != "2026-01-02T00:00:00Z" {
		t.Fatalf("cleanup annotation = %q", got)
	}
}

func TestCleanupExpiredPVCsDeletesDueAndReportsNext(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := core.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	due := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "due", Namespace: "default", Annotations: map[string]string{CleanupAtAnnotation: "2026-01-01T00:00:00Z"}}}
	future := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "future", Namespace: "default", Annotations: map[string]string{CleanupAtAnnotation: "2026-01-01T02:00:00Z"}}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(due, future).Build()
	now := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	next, err := CleanupExpiredPVCs(context.Background(), c, "default", now)
	if err != nil {
		t.Fatal(err)
	}
	if next != time.Hour {
		t.Fatalf("next cleanup = %s, want 1h", next)
	}
	if err := c.Get(context.Background(), clientKey(due), &core.PersistentVolumeClaim{}); err == nil {
		t.Fatal("due PVC still exists")
	}
}

func clientKey(object client.Object) client.ObjectKey {
	return client.ObjectKeyFromObject(object)
}
