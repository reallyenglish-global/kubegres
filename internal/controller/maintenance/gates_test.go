package maintenance

import "testing"

func TestEvaluatePreflightFailsClosed(t *testing.T) {
	base := PreflightInput{PrimaryAndReplicaOnDistinctNodes: true, ReplicaReady: true, Streaming: true, ReplayLagSeconds: 5, MaxReplayLagSeconds: 5, StableObservationSeconds: 60, RequiredStableSeconds: 60, WithinDeadline: true}
	checks := []struct {
		name   string
		mutate func(*PreflightInput)
		reason string
	}{
		{"same node", func(v *PreflightInput) { v.PrimaryAndReplicaOnDistinctNodes = false }, "replica_not_on_distinct_failure_domain"},
		{"not ready", func(v *PreflightInput) { v.ReplicaReady = false }, "replica_not_ready"},
		{"not streaming", func(v *PreflightInput) { v.Streaming = false }, "replication_not_streaming"},
		{"lag", func(v *PreflightInput) { v.ReplayLagSeconds = 6 }, "replay_lag_exceeds_threshold"},
		{"stability", func(v *PreflightInput) { v.StableObservationSeconds = 59 }, "stability_window_incomplete"},
		{"deadline", func(v *PreflightInput) { v.WithinDeadline = false }, "relocation_deadline_exceeded"},
	}
	for _, tt := range checks {
		t.Run(tt.name, func(t *testing.T) {
			in := base
			tt.mutate(&in)
			got := EvaluatePreflight(in)
			if got.Allowed || got.Reason != tt.reason {
				t.Fatalf("got %#v", got)
			}
		})
	}
	if got := EvaluatePreflight(base); !got.Allowed {
		t.Fatalf("valid preflight rejected: %#v", got)
	}
}

func TestEvaluateFencingRequiresBothProofs(t *testing.T) {
	for _, in := range []FencingInput{{}, {OldPrimaryTerminatedOrIsolated: true}, {OldPrimaryRemovedFromWritableEndpoints: true}} {
		if got := EvaluateFencing(in); got.Allowed {
			t.Fatalf("unsafe fencing accepted: %#v", in)
		}
	}
	if got := EvaluateFencing(FencingInput{OldPrimaryTerminatedOrIsolated: true, OldPrimaryRemovedFromWritableEndpoints: true}); !got.Allowed {
		t.Fatalf("valid fencing rejected: %#v", got)
	}
}
