package domain

import "time"

type AuditEvent struct {
	EventID        string        `json:"eventId"`
	DossierID      string        `json:"dossierId"`
	Sequence       int64         `json:"sequence"`
	EventType      string        `json:"eventType"`
	Actor          string        `json:"actor"`
	Reason         string        `json:"reason"`
	BeforeStatus   DossierStatus `json:"beforeStatus"`
	AfterStatus    DossierStatus `json:"afterStatus"`
	OccurredAt     time.Time     `json:"occurredAt"`
	PreviousDigest string        `json:"previousDigest"`
	EventDigest    string        `json:"eventDigest"`
}
