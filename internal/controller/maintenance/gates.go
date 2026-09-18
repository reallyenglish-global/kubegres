package maintenance

// PreflightInput contains observations required before a planned promotion.
// The controller must populate these from Kubernetes and PostgreSQL probes; the
// gate deliberately does not infer database health from Pod Ready alone.
type PreflightInput struct {
	PrimaryAndReplicaOnDistinctNodes bool
	ReplicaReady                     bool
	Streaming                        bool
	ReplayLagSeconds                 int32
	MaxReplayLagSeconds              int32
	StableObservationSeconds         int32
	RequiredStableSeconds            int32
	WithinDeadline                   bool
}

type GateResult struct {
	Allowed bool
	Reason  string
}

func EvaluatePreflight(input PreflightInput) GateResult {
	switch {
	case !input.PrimaryAndReplicaOnDistinctNodes:
		return GateResult{Reason: "replica_not_on_distinct_failure_domain"}
	case !input.ReplicaReady:
		return GateResult{Reason: "replica_not_ready"}
	case !input.Streaming:
		return GateResult{Reason: "replication_not_streaming"}
	case input.ReplayLagSeconds > input.MaxReplayLagSeconds:
		return GateResult{Reason: "replay_lag_exceeds_threshold"}
	case input.StableObservationSeconds < input.RequiredStableSeconds:
		return GateResult{Reason: "stability_window_incomplete"}
	case !input.WithinDeadline:
		return GateResult{Reason: "relocation_deadline_exceeded"}
	default:
		return GateResult{Allowed: true, Reason: "preflight_verified"}
	}
}

// FencingInput is intentionally explicit. A promoted primary is unsafe unless
// both old-primary isolation and writable endpoint removal are observed.
type FencingInput struct {
	OldPrimaryTerminatedOrIsolated         bool
	OldPrimaryRemovedFromWritableEndpoints bool
}

func EvaluateFencing(input FencingInput) GateResult {
	if !input.OldPrimaryTerminatedOrIsolated {
		return GateResult{Reason: "old_primary_not_fenced"}
	}
	if !input.OldPrimaryRemovedFromWritableEndpoints {
		return GateResult{Reason: "old_primary_writable_endpoint_present"}
	}
	return GateResult{Allowed: true, Reason: "fencing_verified"}
}
