package models

import (
	"fmt"
	"strings"
)

// TraceEvent models a log entry.
type TraceEvent struct {
	Timestamp int64
	PID       int
	TID       int
	Proc      string
	EventType string
	Event     string
	KV        map[string]string
}

// ToStr converts a tracevent to string.
func (t TraceEvent) ToStr() string {
	var kvStr strings.Builder
	for k, v := range t.KV {
		fmt.Fprintf(&kvStr, "%s=%s****", k, v)
	}

	return fmt.Sprintf(
		"%d####%s####%s#### %s",
		t.Timestamp,
		t.Proc,
		t.Event,
		kvStr.String(),
	)
}
