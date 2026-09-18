package statefulset_spec

import (
	"fmt"

	v1 "reactive-tech.io/kubegres/api/v1"
	operation2 "reactive-tech.io/kubegres/internal/controller/operation"
	"reactive-tech.io/kubegres/internal/controller/spec/enforcer/resources_count_spec/statefulset/failover"
)

// enforcePostgresMinorUpgrade owns image changes once a database is already
// running. A normal StatefulSet rolling update is unsafe here: it can restart
// the primary before a replica has been proven healthy with the new image.
func (r *AllStatefulSetsSpecEnforcer) enforcePostgresMinorUpgrade() (bool, error) {
	desired := r.kubegresContext.Kubegres.Spec.Image
	if desired == "" {
		return false, nil
	}

	current := r.getAllReverseSortedByInstanceIndex()
	if len(current) == 0 {
		return false, nil
	}
	for _, wrapper := range current {
		if wrapper.StatefulSet.Name == "" {
			continue
		}
		if wrapper.StatefulSet.Spec.Template.Spec.Containers[0].Image == desired {
			continue
		}
		minor, err := IsPostgresMinorUpgrade(wrapper.StatefulSet.Spec.Template.Spec.Containers[0].Image, desired)
		if err != nil {
			r.kubegresContext.Log.ErrorEvent("PostgresImageUpgradeRejected", err,
				"PostgreSQL image changes must be a tagged minor-version upgrade; the requested image was not applied.",
				"Current image", wrapper.StatefulSet.Spec.Template.Spec.Containers[0].Image,
				"Requested image", desired)
			return true, err
		}
		if !minor {
			continue
		}
	}

	// No image change is pending. An active operation may still be waiting for
	// the final primary/replica-count observation.
	if !r.hasPendingPostgresUpgrade(desired) {
		return false, nil
	}
	if r.blockingOperation.IsActiveOperationIdDifferentOf(operation2.OperationIdPostgresMinorVersionUpgrade) {
		return true, nil
	}

	active := r.blockingOperation.GetActiveOperation()
	if active.OperationId == "" {
		return true, r.startPostgresMinorUpgrade(desired)
	}
	if r.blockingOperation.IsActiveOperationInTransition(operation2.OperationIdPostgresMinorVersionUpgrade) {
		switch r.blockingOperation.GetPreviouslyActiveOperation().StepId {
		case operation2.OperationStepIdPostgresUpgradeReplica:
			f := failover.CreatePrimaryToReplicaFailOver(r.kubegresContext, r.resourcesStates, r.blockingOperation)
			return true, f.BeginPostgresMinorUpgradeFailover()
		case operation2.OperationStepIdPostgresUpgradeWaitingBeforeFailover:
			f := failover.CreatePrimaryToReplicaFailOver(r.kubegresContext, r.resourcesStates, r.blockingOperation)
			return true, f.FailOverForPostgresMinorUpgrade()
		case operation2.OperationStepIdPostgresUpgradeFailingOver:
			// The failover completion checker has already observed a healthy new
			// primary. Let the replica-count enforcer recreate the old primary.
			r.blockingOperation.RemoveActiveOperation()
			return true, nil
		}
	}

	return true, nil
}

func (r *AllStatefulSetsSpecEnforcer) hasPendingPostgresUpgrade(desired string) bool {
	if r.blockingOperation.GetActiveOperation().OperationId == operation2.OperationIdPostgresMinorVersionUpgrade {
		return true
	}
	for _, wrapper := range r.resourcesStates.StatefulSets.Replicas.All.GetAllSortedByInstanceIndex() {
		if wrapper.StatefulSet.Name != "" && wrapper.StatefulSet.Spec.Template.Spec.Containers[0].Image != desired {
			return true
		}
	}
	return false
}

func (r *AllStatefulSetsSpecEnforcer) startPostgresMinorUpgrade(desired string) error {
	for _, replica := range r.resourcesStates.StatefulSets.Replicas.All.GetAllSortedByInstanceIndex() {
		if replica.StatefulSet.Name == "" || replica.StatefulSet.Spec.Template.Spec.Containers[0].Image == desired {
			continue
		}
		if !replica.IsReadyForFailover() {
			return nil
		}
		if err := r.blockingOperation.ActivateOperationOnStatefulSet(
			operation2.OperationIdPostgresMinorVersionUpgrade,
			operation2.OperationStepIdPostgresUpgradeReplica,
			replica.InstanceIndex); err != nil {
			return err
		}
		replica.StatefulSet.Spec.Template.Spec.Containers[0].Image = desired
		if len(replica.StatefulSet.Spec.Template.Spec.InitContainers) > 0 {
			replica.StatefulSet.Spec.Template.Spec.InitContainers[0].Image = desired
		}
		if err := r.kubegresContext.Client.Update(r.kubegresContext.Ctx, &replica.StatefulSet); err != nil {
			r.blockingOperation.RemoveActiveOperation()
			return fmt.Errorf("update PostgreSQL replica %s for minor upgrade: %w", replica.StatefulSet.Name, err)
		}
		return nil
	}
	return nil
}

func (r *AllStatefulSetsSpecEnforcer) isPostgresMinorUpgradeReplicaReady(operation v1.KubegresBlockingOperation) bool {
	replica, err := r.resourcesStates.StatefulSets.Replicas.All.GetByInstanceIndex(operation.StatefulSetOperation.InstanceIndex)
	return err == nil && replica.StatefulSet.Spec.Template.Spec.Containers[0].Image == r.kubegresContext.Kubegres.Spec.Image && replica.IsReadyForFailover()
}

// isPostgresMinorUpgradeReplicaOperationComplete is registered with the
// blocking-operation manager and is evaluated after each reconciliation.
func (r *AllStatefulSetsSpecEnforcer) isPostgresMinorUpgradeReplicaOperationComplete(operation v1.KubegresBlockingOperation) bool {
	return r.isPostgresMinorUpgradeReplicaReady(operation)
}
