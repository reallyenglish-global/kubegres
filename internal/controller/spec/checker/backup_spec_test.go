package checker

import (
	"testing"

	postgresV1 "reactive-tech.io/kubegres/api/v1"
)

// A scheduled backup needs somewhere to write: either an existing PVC or a
// size for a generic ephemeral PVC. Requiring pvcName unconditionally made the
// ephemeral backup storage feature unreachable through the reconcile loop.
func TestBackupSpecErrorAcceptsEphemeralSize(t *testing.T) {
	for _, tc := range []struct {
		name        string
		backup      postgresV1.KubegresBackUp
		pvcDeployed bool
		wantError   bool
	}{
		{"no schedule", postgresV1.KubegresBackUp{}, false, false},
		{"pvc deployed", postgresV1.KubegresBackUp{Schedule: "0 2 * * *", VolumeMount: "/backups", PvcName: "backups"}, true, false},
		{"pvc missing without size", postgresV1.KubegresBackUp{Schedule: "0 2 * * *", VolumeMount: "/backups", PvcName: "backups"}, false, true},
		{"pvc missing with size falls back to ephemeral", postgresV1.KubegresBackUp{Schedule: "0 2 * * *", VolumeMount: "/backups", PvcName: "backups", Size: "1Gi"}, false, false},
		{"ephemeral size only", postgresV1.KubegresBackUp{Schedule: "0 2 * * *", VolumeMount: "/backups", Size: "1Gi"}, false, false},
		{"neither pvc nor size", postgresV1.KubegresBackUp{Schedule: "0 2 * * *", VolumeMount: "/backups"}, false, true},
		{"missing volume mount", postgresV1.KubegresBackUp{Schedule: "0 2 * * *", Size: "1Gi"}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := SpecChecker{}
			r.resourcesStates.BackUp.IsPvcDeployed = tc.pvcDeployed
			spec := postgresV1.KubegresSpec{Backup: tc.backup}
			got := r.backupSpecError(&spec)
			if (got != "") != tc.wantError {
				t.Fatalf("backupSpecError = %q, wantError=%v", got, tc.wantError)
			}
		})
	}
}
