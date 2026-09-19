package failover

import (
	operation2 "reactive-tech.io/kubegres/internal/controller/operation"
)

// BeginPostgresMinorUpgradeFailover removes the old primary and starts the
// short safety window before promotion. It is intentionally separate from the
// normal automatic failover path so an image change cannot be mistaken for a
// failed database.
func (r *PrimaryToReplicaFailOver) BeginPostgresMinorUpgradeFailover() error {
	newPrimary, err := r.selectReplicaToPromote()
	if err != nil {
		return err
	}
	return r.waitBeforePromotingReplicaToPrimaryFor(
		newPrimary,
		operation2.OperationIdPostgresMinorVersionUpgrade,
		operation2.OperationStepIdPostgresUpgradeWaitingBeforeFailover,
	)
}

func (r *PrimaryToReplicaFailOver) CreateOperationConfigForPostgresMinorUpgradeFailover() operation2.BlockingOperationConfig {
	return operation2.BlockingOperationConfig{
		OperationId:                         operation2.OperationIdPostgresMinorVersionUpgrade,
		StepId:                              operation2.OperationStepIdPostgresUpgradeFailingOver,
		TimeOutInSeconds:                    600,
		CompletionChecker:                   r.isFailOverCompleted,
		AfterCompletionMoveToTransitionStep: true,
	}
}
