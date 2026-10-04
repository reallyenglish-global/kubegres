package pvccleanup

import (
	"context"
	"fmt"
	"strings"
	"time"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	postgresv1 "reactive-tech.io/kubegres/api/v1"
)

const CleanupAtAnnotation = "kubegres.reactive-tech.io/pvc-cleanup-at"

func StatefulSetRetentionPolicy(volume postgresv1.Volume) *apps.StatefulSetPersistentVolumeClaimRetentionPolicy {
	policy := volume.PvcCleanupPolicy
	whenDeleted := apps.RetainPersistentVolumeClaimRetentionPolicyType
	if policy == postgresv1.PvcCleanupPolicyDelete {
		whenDeleted = apps.DeletePersistentVolumeClaimRetentionPolicyType
	}
	return &apps.StatefulSetPersistentVolumeClaimRetentionPolicy{
		WhenDeleted: whenDeleted,
		WhenScaled:  apps.RetainPersistentVolumeClaimRetentionPolicyType,
	}
}

// PrepareForStatefulSetDeletion applies the configured PVC retention behavior before
// the StatefulSet is deleted. For delayed cleanup, PVCs are annotated with an absolute
// cleanup time so the controller can safely resume after a restart.
func PrepareForStatefulSetDeletion(ctx context.Context, c client.Client, statefulSet *apps.StatefulSet, volume postgresv1.Volume, now time.Time) error {
	policy := volume.PvcCleanupPolicy
	if policy == "" {
		policy = postgresv1.PvcCleanupPolicyRetain
	}

	retention := apps.StatefulSetPersistentVolumeClaimRetentionPolicy{WhenScaled: apps.RetainPersistentVolumeClaimRetentionPolicyType}
	if policy == postgresv1.PvcCleanupPolicyDelete {
		retention.WhenDeleted = apps.DeletePersistentVolumeClaimRetentionPolicyType
	} else {
		retention.WhenDeleted = apps.RetainPersistentVolumeClaimRetentionPolicyType
	}
	statefulSet.Spec.PersistentVolumeClaimRetentionPolicy = &retention

	if policy != postgresv1.PvcCleanupPolicyAfter {
		return nil
	}
	if volume.PvcCleanupDuration == nil || volume.PvcCleanupDuration.Duration < 0 {
		return fmt.Errorf("volume.pvcCleanupDuration is required and must be non-negative when volume.pvcCleanupPolicy is After")
	}

	pvcs := &core.PersistentVolumeClaimList{}
	if err := c.List(ctx, pvcs, client.InNamespace(statefulSet.Namespace)); err != nil {
		return err
	}
	cleanupAt := now.Add(volume.PvcCleanupDuration.Duration).UTC().Format(time.RFC3339)
	for i := range pvcs.Items {
		pvc := &pvcs.Items[i]
		if !belongsToStatefulSet(pvc, statefulSet) {
			continue
		}
		if pvc.Annotations == nil {
			pvc.Annotations = map[string]string{}
		}
		pvc.Annotations[CleanupAtAnnotation] = cleanupAt
		if err := c.Update(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func CleanupExpiredPVCs(ctx context.Context, c client.Client, namespace string, now time.Time) (time.Duration, error) {
	pvcs := &core.PersistentVolumeClaimList{}
	if err := c.List(ctx, pvcs, client.InNamespace(namespace)); err != nil {
		return 0, err
	}
	var next time.Duration
	for i := range pvcs.Items {
		pvc := &pvcs.Items[i]
		value := pvc.Annotations[CleanupAtAnnotation]
		if value == "" {
			continue
		}
		cleanupAt, err := time.Parse(time.RFC3339, value)
		if err != nil {
			continue
		}
		if !cleanupAt.After(now) {
			if err := c.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
				return 0, err
			}
			continue
		}
		remaining := cleanupAt.Sub(now)
		if next == 0 || remaining < next {
			next = remaining
		}
	}
	return next, nil
}

func belongsToStatefulSet(pvc *core.PersistentVolumeClaim, statefulSet *apps.StatefulSet) bool {
	for _, owner := range pvc.OwnerReferences {
		if owner.Kind == "StatefulSet" && owner.Name == statefulSet.Name {
			return true
		}
	}
	for _, template := range statefulSet.Spec.VolumeClaimTemplates {
		if strings.HasPrefix(pvc.Name, template.Name+"-"+statefulSet.Name+"-") {
			return true
		}
	}
	return false
}
