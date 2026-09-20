package maintenance

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubegresv1 "reactive-tech.io/kubegres/api/v1"
)

const maxHistoryEntries = 20

// RecordHistory appends a bounded, structured audit entry. It does not perform
// a status write; the reconciler must persist the returned operation.
func RecordHistory(op *kubegresv1.MaintenanceOperation, at time.Time, phase kubegresv1.MaintenancePhase, message string, evidence map[string]string) {
	if op == nil {
		return
	}
	op.Status.History = append(op.Status.History, kubegresv1.MaintenanceOperationHistoryEntry{
		At: metav1.NewTime(at), Phase: phase, Message: message, Evidence: evidence,
	})
	if len(op.Status.History) > maxHistoryEntries {
		op.Status.History = op.Status.History[len(op.Status.History)-maxHistoryEntries:]
	}
}
