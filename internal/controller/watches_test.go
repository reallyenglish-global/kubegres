package controller

import (
	"context"
	"testing"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestMapKubegresChildByAppLabel(t *testing.T) {
	pod := &core.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "db-1-0", Labels: map[string]string{"app": "db"}}}
	got := mapKubegresChildByAppLabel(context.Background(), pod)
	if len(got) != 1 || got[0].Namespace != "ns" || got[0].Name != "db" {
		t.Fatalf("requests = %v, want one request for ns/db", got)
	}
	if got := mapKubegresChildByAppLabel(context.Background(), &core.Pod{}); len(got) != 0 {
		t.Fatalf("object without app label produced %v", got)
	}
}

func TestPodChangePredicateOnlyEnqueuesMeaningfulUpdates(t *testing.T) {
	base := func() *core.Pod {
		return &core.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "db-1-0", Labels: map[string]string{"app": "db"}},
			Status:     core.PodStatus{Phase: core.PodRunning, Conditions: []core.PodCondition{{Type: core.PodReady, Status: core.ConditionTrue}}},
		}
	}
	p := podChangePredicate()
	unchanged := base()
	unchanged.ResourceVersion = "2"
	if p.Update(event.UpdateEvent{ObjectOld: base(), ObjectNew: unchanged}) {
		t.Fatal("resourceVersion-only update enqueued")
	}
	disrupted := base()
	disrupted.Status.Conditions = append(disrupted.Status.Conditions, core.PodCondition{Type: core.DisruptionTarget, Status: core.ConditionTrue, Reason: "EvictionByEvictionAPI"})
	if !p.Update(event.UpdateEvent{ObjectOld: base(), ObjectNew: disrupted}) {
		t.Fatal("DisruptionTarget condition did not enqueue")
	}
	notReady := base()
	notReady.Status.Conditions[0].Status = core.ConditionFalse
	if !p.Update(event.UpdateEvent{ObjectOld: base(), ObjectNew: notReady}) {
		t.Fatal("Ready transition did not enqueue")
	}
	deleting := base()
	now := metav1.Now()
	deleting.DeletionTimestamp = &now
	if !p.Update(event.UpdateEvent{ObjectOld: base(), ObjectNew: deleting}) {
		t.Fatal("deletionTimestamp did not enqueue")
	}
	if !p.Delete(event.DeleteEvent{Object: base()}) {
		t.Fatal("pod deletion did not enqueue")
	}
	if p.Create(event.CreateEvent{Object: base()}) {
		t.Fatal("pod creation enqueued; StatefulSet status already covers it")
	}
}

func TestPvcChangePredicateEnqueuesCapacityChanges(t *testing.T) {
	pvc := func(capacity string) *core.PersistentVolumeClaim {
		return &core.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "postgres-db-db-1-0", Labels: map[string]string{"app": "db"}},
			Status:     core.PersistentVolumeClaimStatus{Capacity: core.ResourceList{core.ResourceStorage: resource.MustParse(capacity)}},
		}
	}
	p := pvcChangePredicate()
	if p.Update(event.UpdateEvent{ObjectOld: pvc("1Gi"), ObjectNew: pvc("1Gi")}) {
		t.Fatal("unchanged PVC enqueued")
	}
	if !p.Update(event.UpdateEvent{ObjectOld: pvc("1Gi"), ObjectNew: pvc("2Gi")}) {
		t.Fatal("capacity change did not enqueue")
	}
	resizing := pvc("1Gi")
	resizing.Status.Conditions = []core.PersistentVolumeClaimCondition{{Type: core.PersistentVolumeClaimResizing, Status: core.ConditionTrue}}
	if !p.Update(event.UpdateEvent{ObjectOld: pvc("1Gi"), ObjectNew: resizing}) {
		t.Fatal("resize condition change did not enqueue")
	}
}
