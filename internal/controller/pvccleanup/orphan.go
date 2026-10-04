package pvccleanup

import (
	"context"
	"strings"
	"time"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	postgresv1 "reactive-tech.io/kubegres/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MarkOrphanedPVCs schedules PVCs whose StatefulSet was deleted outside the operator.
// This closes the gap between an external kubectl delete and the normal operator delete path.
func MarkOrphanedPVCs(ctx context.Context, c client.Client, namespace, kubegresName string, volume postgresv1.Volume, now time.Time) (time.Duration, error) {
	if volume.PvcCleanupPolicy != postgresv1.PvcCleanupPolicyAfter || volume.PvcCleanupDuration == nil {
		return 0, nil
	}
	statefulSets := &apps.StatefulSetList{}
	if err := c.List(ctx, statefulSets, client.InNamespace(namespace)); err != nil {
		return 0, err
	}
	active := make(map[string]struct{}, len(statefulSets.Items))
	for _, statefulSet := range statefulSets.Items {
		active[statefulSet.Name] = struct{}{}
	}
	pvcs := &core.PersistentVolumeClaimList{}
	if err := c.List(ctx, pvcs, client.InNamespace(namespace)); err != nil {
		return 0, err
	}
	cleanupAt := now.Add(volume.PvcCleanupDuration.Duration).UTC().Format(time.RFC3339)
	var next time.Duration
	for i := range pvcs.Items {
		pvc := &pvcs.Items[i]
		ownerName, owned := statefulSetOwnerName(pvc)
		if !owned || !strings.HasPrefix(ownerName, kubegresName+"-") {
			continue
		}
		if _, exists := active[ownerName]; exists {
			continue
		}
		if pvc.Annotations == nil {
			pvc.Annotations = map[string]string{}
		}
		if _, alreadyScheduled := pvc.Annotations[CleanupAtAnnotation]; !alreadyScheduled {
			pvc.Annotations[CleanupAtAnnotation] = cleanupAt
			if err := c.Update(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
				return 0, err
			}
		}
		next = volume.PvcCleanupDuration.Duration
	}
	return next, nil
}

func statefulSetOwnerName(pvc *core.PersistentVolumeClaim) (string, bool) {
	for _, owner := range pvc.OwnerReferences {
		if owner.Kind == "StatefulSet" {
			return owner.Name, true
		}
	}
	return "", false
}
