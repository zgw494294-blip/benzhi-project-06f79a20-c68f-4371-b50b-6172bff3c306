package concurrent_audit_clock_test

import (
	"testing"
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/audit"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
)

type auditResult struct {
	event        domain.AuditEvent
	lastOccurred time.Time
	clockOK      bool
	err          error
}

func TestConcurrentDossiersSynchronizeAuditClockState(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	fixed := time.Date(2026, 8, 27, 13, 0, 0, 0, time.UTC)
	builder := audit.NewBuilder(func() time.Time {
		entered <- struct{}{}
		<-release
		return fixed
	})
	start := make(chan struct{})
	results := make(chan auditResult, 2)

	for _, dossierID := range []string{"dossier-audit-a", "dossier-audit-b"} {
		dossierID := dossierID
		go func() {
			<-start
			event, err := builder.Next(nil, audit.Change{
				EventID:     "event-" + dossierID,
				DossierID:   dossierID,
				EventType:   audit.EventDossierCreated,
				Actor:       "访谈员",
				Reason:      "并行建立不同档案",
				AfterStatus: domain.StatusDraft,
			})
			lastOccurred, clockOK := builder.LastOccurred(dossierID)
			results <- auditResult{event: event, lastOccurred: lastOccurred, clockOK: clockOK, err: err}
		}()
	}

	close(start)
	<-entered
	<-entered
	close(release)
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatalf("build audit event: %v", result.err)
		}
		if result.event.OccurredAt.IsZero() {
			t.Fatal("audit event lost occurrence time")
		}
		if !result.clockOK || !result.lastOccurred.Equal(result.event.OccurredAt) {
			t.Fatal("audit clock state does not match emitted event")
		}
	}
}
