package models

import "time"

// TranscriptEntry contains only fields needed in a session export.
type TranscriptEntry struct {
	CreatedAt        time.Time
	SenderRole       string
	ContentType      string
	DeliveryStatus   string
	SentByOperator   bool
	ImpersonatedRole string
	Content          string
}
