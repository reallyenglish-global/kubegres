package statefulset

import (
	"strconv"
	"testing"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newIndexedStatefulSetWrapper(instanceIndex int32) StatefulSetWrapper {
	statefulSet := apps.StatefulSet{Spec: apps.StatefulSetSpec{Template: core.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"index": strconv.Itoa(int(instanceIndex))}},
	}}}
	return StatefulSetWrapper{InstanceIndex: instanceIndex, StatefulSet: statefulSet}
}

func TestGetAllSortedByInstanceIndexReturnsACopy(t *testing.T) {
	var wrappers StatefulSetWrappers
	wrappers.Add(newIndexedStatefulSetWrapper(1))
	wrappers.Add(newIndexedStatefulSetWrapper(2))

	sorted := wrappers.GetAllSortedByInstanceIndex()
	sorted[0].InstanceIndex = 99
	reverseSorted := wrappers.GetAllReverseSortedByInstanceIndex()
	reverseSorted[0].InstanceIndex = 99

	if got := wrappers.GetAllSortedByInstanceIndex()[0].InstanceIndex; got != 1 {
		t.Fatalf("sorted internal state changed through returned slice: first instance index = %d, want 1", got)
	}
	if got := wrappers.GetAllReverseSortedByInstanceIndex()[0].InstanceIndex; got != 2 {
		t.Fatalf("reverse sorted internal state changed through returned slice: first instance index = %d, want 2", got)
	}
}
