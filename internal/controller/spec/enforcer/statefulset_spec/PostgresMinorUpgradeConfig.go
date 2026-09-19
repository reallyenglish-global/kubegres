package statefulset_spec

import (
	operation2 "reactive-tech.io/kubegres/internal/controller/operation"
)

func (r *AllStatefulSetsSpecEnforcer) CreateOperationConfigForPostgresMinorUpgradeReplica() operation2.BlockingOperationConfig {
	return operation2.BlockingOperationConfig{
		OperationId:                         operation2.OperationIdPostgresMinorVersionUpgrade,
		StepId:                              operation2.OperationStepIdPostgresUpgradeReplica,
		TimeOutInSeconds:                    600,
		CompletionChecker:                   r.isPostgresMinorUpgradeReplicaOperationComplete,
		AfterCompletionMoveToTransitionStep: true,
	}
}

func (r *AllStatefulSetsSpecEnforcer) CreateOperationConfigForPostgresMinorUpgradeWaiting() operation2.BlockingOperationConfig {
	return operation2.BlockingOperationConfig{
		OperationId:                         operation2.OperationIdPostgresMinorVersionUpgrade,
		StepId:                              operation2.OperationStepIdPostgresUpgradeWaitingBeforeFailover,
		TimeOutInSeconds:                    30,
		AfterCompletionMoveToTransitionStep: true,
	}
}
