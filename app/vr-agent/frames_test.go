package main

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"vraxel.io/vraxel/lib/agent/datachan"
	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// newTestAgent wires an agent the way main does, and hands back the buffer
// its log lands in. The data channel is real but never Run, so Ensure is
// the no-op buffered send it is designed to be.
func newTestAgent() (*agent, *bytes.Buffer) {
	var buf bytes.Buffer
	lg := stdLogger{log.New(&buf, "", 0)}
	a := &agent{log: lg}
	a.data = datachan.New(datachan.Config{
		ServerURL: "http://127.0.0.1:1",
		Token:     a.sessionToken,
		Guard:     datachan.NewGuard(nil),
		Log:       lg,
	})
	return a, &buf
}

// TestSessionTokenFrameIsKept pins the reason this routing exists. The
// server has always pushed session.token; an agent that logs the frame and
// drops the value looks perfectly healthy -- control channel up, heartbeats
// fine -- and then cannot dial the data channel at all, because the token
// is the only credential that endpoint accepts.
func TestSessionTokenFrameIsKept(t *testing.T) {
	a, _ := newTestAgent()

	if got := a.sessionToken(); got != "" {
		t.Fatalf("token before any frame = %q, want empty", got)
	}

	a.onFrame(context.Background(), agenttypes.Frame{
		Type:  agenttypes.FrameTypeSessionToken,
		Token: "tok-1",
	}, nil)
	if got := a.sessionToken(); got != "tok-1" {
		t.Fatalf("token after the first frame = %q, want tok-1", got)
	}

	// The server renews for as long as the channel lives, so the newest
	// value must win: keeping the first would work until the first expiry
	// and then fail for the rest of the connection.
	a.onFrame(context.Background(), agenttypes.Frame{
		Type:  agenttypes.FrameTypeSessionToken,
		Token: "tok-2",
	}, nil)
	if got := a.sessionToken(); got != "tok-2" {
		t.Fatalf("token after renewal = %q, want tok-2", got)
	}
}

// TestChannelOpenIsRouted checks the frame reaches the data channel rather
// than falling through to the default branch. Asserted on the log line the
// branch writes, which is the only effect Ensure has that is visible from
// outside the package.
func TestChannelOpenIsRouted(t *testing.T) {
	a, buf := newTestAgent()

	a.onFrame(context.Background(), agenttypes.Frame{
		Type: agenttypes.FrameTypeChannelOpen,
	}, nil)

	if !strings.Contains(buf.String(), "server asked for the data channel") {
		t.Fatalf("channel.open did not reach the data channel; log was %q", buf.String())
	}
}

// TestUnknownFrameIsLogged keeps the default branch honest: a frame type
// this slice does not consume yet must still be visible, because that log
// line is how the next slice's wiring gets confirmed on a real host.
func TestUnknownFrameIsLogged(t *testing.T) {
	a, buf := newTestAgent()

	a.onFrame(context.Background(), agenttypes.Frame{
		Type: agenttypes.FrameTypeJobDispatch,
	}, nil)

	if !strings.Contains(buf.String(), agenttypes.FrameTypeJobDispatch) {
		t.Fatalf("unknown frame was not logged; log was %q", buf.String())
	}
}
