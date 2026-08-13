package client

import (
	"testing"
	"time"
)

// TestJitteredWithinHalfToFull asserts full jitter never returns a wait
// outside [d/2, d] -- below d/2 would defeat the backoff, above d would
// exceed the intended cap.
func TestJitteredWithinHalfToFull(t *testing.T) {
	for _, d := range []time.Duration{reconnectMin, 4 * time.Second, reconnectMax} {
		for i := 0; i < 1000; i++ {
			got := jittered(d)
			if got < d/2 || got > d {
				t.Fatalf("jittered(%s) = %s, want within [%s, %s]", d, got, d/2, d)
			}
		}
	}
}
