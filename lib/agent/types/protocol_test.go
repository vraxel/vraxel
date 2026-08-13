package types

import (
	"strings"
	"testing"
)

// TestFrameJobDispatchRoundtrip pins the job.dispatch wire shape: a
// frame carrying a full JobDispatch survives encode/decode bit-exact.
func TestFrameJobDispatchRoundtrip(t *testing.T) {
	in := Frame{
		Type: FrameTypeJobDispatch,
		ID:   "f-1",
		Job: &JobDispatch{
			ID:              1024,
			Kind:            "play",
			Entry:           "node_exporter/install.yml",
			BundleHash:      "sha256:abc",
			BundleURL:       BundlePath("sha256:abc"),
			VarsURL:         JobVarsPath(1024),
			HostName:        "test4",
			PlayIndex:       2,
			IsRunOnceTarget: true,
			TimeoutSec:      1800,
		},
	}
	data, err := EncodeFrame(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := DecodeFrame(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Job == nil || *out.Job != *in.Job {
		t.Fatalf("job roundtrip mismatch: got %+v want %+v", out.Job, in.Job)
	}
}

// TestFrameForwardCompat asserts an agent decoding a frame with unknown
// fields neither errors nor loses the fields it knows -- the multi-version
// coexistence contract (design §2.5).
func TestFrameForwardCompat(t *testing.T) {
	raw := []byte(`{"type":"job.cancel","id":"f-2","jobId":7,"futureField":{"x":1}}`)
	f, err := DecodeFrame(raw)
	if err != nil {
		t.Fatalf("decode with unknown field: %v", err)
	}
	if f.Type != FrameTypeJobCancel || f.JobID != 7 {
		t.Fatalf("known fields lost: %+v", f)
	}
}

func TestJobPathBuilders(t *testing.T) {
	cases := map[string]string{
		BundlePath("sha256:ab"): "/api/agent/v1/bundles/sha256:ab",
		JobVarsPath(9):          "/api/agent/v1/jobs/9/vars",
		JobEventsPath(9):        "/api/agent/v1/jobs/9/events",
		JobResultPath(9):        "/api/agent/v1/jobs/9/result",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
	}
	for _, p := range []string{BundlePath("h"), JobVarsPath(1)} {
		if !strings.HasPrefix(p, ProtocolPathPrefix) {
			t.Fatalf("path %q not under ProtocolPathPrefix", p)
		}
	}
}
