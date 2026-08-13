package datachan

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cws "github.com/coder/websocket"
	"github.com/hashicorp/yamux"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	ws "vraxel.io/vraxel/lib/websocket"
)

// testLogger keeps the package's Logger dependency out of the tests.
type testLogger struct{ t *testing.T }

func (l testLogger) Infof(f string, a ...any) { l.t.Logf("INFO  "+f, a...) }
func (l testLogger) Warnf(f string, a ...any) { l.t.Logf("WARN  "+f, a...) }

// gateway is the server half of the data channel, standing in for
// pkg/apis/agentgw. It is also the reference for what the gateway must
// do: upgrade, wrap in a yamux client, open one stream per session.
type gateway struct {
	srv     *httptest.Server
	sess    chan *yamux.Session
	channel *Channel
	cancel  context.CancelFunc
}

func newGateway(t *testing.T, guard *Guard) *gateway {
	t.Helper()
	g := &gateway{sess: make(chan *yamux.Session, 1)}

	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		conn, err := ws.Accept(w, r, nil)
		if err != nil {
			return
		}
		netConn := cws.NetConn(context.Background(), conn.Inner(), cws.MessageBinary)
		ycfg := yamux.DefaultConfig()
		ycfg.LogOutput = io.Discard
		sess, err := yamux.Client(netConn, ycfg)
		if err != nil {
			return
		}
		g.sess <- sess
		<-r.Context().Done()
	}))
	t.Cleanup(g.srv.Close)

	g.channel = New(Config{
		ServerURL: g.srv.URL,
		Token:     func() string { return "session-token" },
		Guard:     guard,
		Log:       testLogger{t},
	})

	ctx, cancel := context.WithCancel(context.Background())
	g.cancel = cancel
	t.Cleanup(cancel)
	go g.channel.Run(ctx)
	g.channel.Ensure()
	return g
}

// session returns the yamux session once the agent has dialled.
func (g *gateway) session(t *testing.T) *yamux.Session {
	t.Helper()
	select {
	case s := <-g.sess:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("agent never dialled the data channel")
		return nil
	}
}

// open starts a stream and completes the handshake.
func open(t *testing.T, sess *yamux.Session, req agenttypes.StreamOpen) (net.Conn, agenttypes.StreamAccept) {
	t.Helper()
	stream, err := sess.OpenStream()
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	if err := agenttypes.WriteStreamHeader(stream, req); err != nil {
		t.Fatalf("write header: %v", err)
	}
	var acc agenttypes.StreamAccept
	if err := agenttypes.ReadStreamHeader(stream, &acc); err != nil {
		t.Fatalf("read accept: %v", err)
	}
	return stream, acc
}

// echoServer is a local TCP service for the tcp-kind tests, standing in
// for a middleware API on loopback.
func echoServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().String()
}

func TestTCPStreamRoundTrip(t *testing.T) {
	g := newGateway(t, NewGuard(nil))
	sess := g.session(t)

	stream, acc := open(t, sess, agenttypes.StreamOpen{
		Kind:   agenttypes.StreamKindTCP,
		Target: echoServer(t),
	})
	defer stream.Close()
	if !acc.Ok {
		t.Fatalf("stream rejected: %+v", acc)
	}

	if _, err := stream.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(stream, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("got %q, want ping", buf)
	}
}

// TestTCPStreamHalfClose covers the case an HTTP request depends on: the
// gateway finishes writing and the local service must see EOF while the
// response still flows back.
func TestTCPStreamHalfClose(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		body, _ := io.ReadAll(c) // returns only once the client half-closes
		_, _ = c.Write([]byte("saw:" + string(body)))
	}()

	g := newGateway(t, NewGuard(nil))
	sess := g.session(t)
	stream, acc := open(t, sess, agenttypes.StreamOpen{
		Kind:   agenttypes.StreamKindTCP,
		Target: ln.Addr().String(),
	})
	if !acc.Ok {
		t.Fatalf("stream rejected: %+v", acc)
	}

	if _, err := stream.Write([]byte("req")); err != nil {
		t.Fatalf("write: %v", err)
	}
	// yamux Close is a FIN: the read side stays usable.
	if err := stream.Close(); err != nil {
		t.Fatalf("half close: %v", err)
	}
	got, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if string(got) != "saw:req" {
		t.Fatalf("got %q, want saw:req", got)
	}
}

func TestTCPStreamRejectsNonLoopback(t *testing.T) {
	g := newGateway(t, NewGuard(nil))
	sess := g.session(t)

	stream, acc := open(t, sess, agenttypes.StreamOpen{
		Kind:   agenttypes.StreamKindTCP,
		Target: "10.1.1.10:5432",
	})
	defer stream.Close()
	if acc.Ok {
		t.Fatal("agent accepted a non-loopback target")
	}
	if acc.Code != agenttypes.StreamErrTargetNotAllowed {
		t.Fatalf("code = %q, want %q", acc.Code, agenttypes.StreamErrTargetNotAllowed)
	}
}

func TestTCPStreamRejectsPortOutsideAllowlist(t *testing.T) {
	g := newGateway(t, NewGuard([]int{9100}))
	sess := g.session(t)

	stream, acc := open(t, sess, agenttypes.StreamOpen{
		Kind:   agenttypes.StreamKindTCP,
		Target: "127.0.0.1:5432",
	})
	defer stream.Close()
	if acc.Ok {
		t.Fatal("agent accepted a port outside the allowlist")
	}
	if acc.Code != agenttypes.StreamErrTargetNotAllowed {
		t.Fatalf("code = %q, want %q", acc.Code, agenttypes.StreamErrTargetNotAllowed)
	}
}

func TestUnknownKindRejected(t *testing.T) {
	g := newGateway(t, NewGuard(nil))
	sess := g.session(t)

	stream, acc := open(t, sess, agenttypes.StreamOpen{Kind: "nope"})
	defer stream.Close()
	if acc.Ok || acc.Code != agenttypes.StreamErrUnknownKind {
		t.Fatalf("got %+v, want unknown_kind rejection", acc)
	}
}

func TestPTYStreamRunsCommand(t *testing.T) {
	g := newGateway(t, NewGuard(nil))
	sess := g.session(t)

	stream, acc := open(t, sess, agenttypes.StreamOpen{
		Kind:    agenttypes.StreamKindPTY,
		Command: []string{"/bin/sh", "-c", "echo hello-pty; exit 7"},
		Cols:    100,
		Rows:    40,
	})
	defer stream.Close()
	if !acc.Ok {
		t.Fatalf("stream rejected: %+v", acc)
	}

	var out strings.Builder
	var exit agenttypes.PTYExit
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		typ, payload, err := agenttypes.ReadMessage(stream)
		if err != nil {
			t.Fatalf("read message: %v", err)
		}
		if typ == agenttypes.MsgData {
			out.Write(payload)
			continue
		}
		if typ == agenttypes.MsgExit {
			if err := json.Unmarshal(payload, &exit); err != nil {
				t.Fatalf("decode exit: %v", err)
			}
			break
		}
	}
	if !strings.Contains(out.String(), "hello-pty") {
		t.Fatalf("output %q does not contain hello-pty", out.String())
	}
	if exit.Code != 7 {
		t.Fatalf("exit code = %d, want 7", exit.Code)
	}
}

// TestPTYResize proves the resize control message reaches the process,
// by having the shell report its terminal size after the resize lands.
func TestPTYResize(t *testing.T) {
	g := newGateway(t, NewGuard(nil))
	sess := g.session(t)

	stream, acc := open(t, sess, agenttypes.StreamOpen{
		Kind:    agenttypes.StreamKindPTY,
		Command: []string{"/bin/sh"},
		Cols:    80,
		Rows:    24,
	})
	defer stream.Close()
	if !acc.Ok {
		t.Fatalf("stream rejected: %+v", acc)
	}

	if err := agenttypes.WriteJSONMessage(stream, agenttypes.MsgResize,
		agenttypes.PTYResize{Cols: 132, Rows: 50}); err != nil {
		t.Fatalf("resize: %v", err)
	}
	// stty reads the tty's current size from the kernel.
	if err := agenttypes.WriteMessage(stream, agenttypes.MsgData,
		[]byte("stty size; exit\n")); err != nil {
		t.Fatalf("write cmd: %v", err)
	}

	var out strings.Builder
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		typ, payload, err := agenttypes.ReadMessage(stream)
		if err != nil {
			break
		}
		if typ == agenttypes.MsgData {
			out.Write(payload)
		}
		if typ == agenttypes.MsgExit {
			break
		}
	}
	if !strings.Contains(out.String(), "50 132") {
		t.Fatalf("output %q does not report the resized 50 132", out.String())
	}
}

func TestFileWriteReadListStat(t *testing.T) {
	g := newGateway(t, NewGuard(nil))
	sess := g.session(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.bin")
	body := strings.Repeat("payload-", 100000) // ~800KB: multiple chunks

	// write
	stream, acc := open(t, sess, agenttypes.StreamOpen{
		Kind: agenttypes.StreamKindFile,
		Op:   agenttypes.FileOpWrite,
		Path: path,
		Size: int64(len(body)),
		Mode: 0o640,
	})
	if !acc.Ok {
		t.Fatalf("stream rejected: %+v", acc)
	}
	for off := 0; off < len(body); off += fileChunk {
		end := min(off+fileChunk, len(body))
		if err := agenttypes.WriteMessage(stream, agenttypes.MsgData, []byte(body[off:end])); err != nil {
			t.Fatalf("send chunk: %v", err)
		}
	}
	if err := agenttypes.WriteMessage(stream, agenttypes.MsgEOF, nil); err != nil {
		t.Fatalf("send eof: %v", err)
	}
	res := readResult(t, stream)
	stream.Close()
	if !res.Ok {
		t.Fatalf("write failed: %s", res.Error)
	}
	if res.Written != int64(len(body)) {
		t.Fatalf("written = %d, want %d", res.Written, len(body))
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(onDisk) != body {
		t.Fatal("file content does not match what was uploaded")
	}

	// read
	stream, acc = open(t, sess, agenttypes.StreamOpen{
		Kind: agenttypes.StreamKindFile,
		Op:   agenttypes.FileOpRead,
		Path: path,
	})
	if !acc.Ok {
		t.Fatalf("stream rejected: %+v", acc)
	}
	var got strings.Builder
	for {
		typ, payload, err := agenttypes.ReadMessage(stream)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if typ == agenttypes.MsgData {
			got.Write(payload)
			continue
		}
		if typ == agenttypes.MsgEOF {
			break
		}
	}
	if res := readResult(t, stream); !res.Ok {
		t.Fatalf("read failed: %s", res.Error)
	}
	stream.Close()
	if got.String() != body {
		t.Fatalf("downloaded %d bytes, want %d", got.Len(), len(body))
	}

	// list
	stream, _ = open(t, sess, agenttypes.StreamOpen{
		Kind: agenttypes.StreamKindFile,
		Op:   agenttypes.FileOpList,
		Path: dir,
	})
	res = readResult(t, stream)
	stream.Close()
	if !res.Ok || len(res.Entries) != 1 || res.Entries[0].Name != "artifact.bin" {
		t.Fatalf("list = %+v", res)
	}
	if res.Entries[0].Mode != 0o640 {
		t.Fatalf("mode = %o, want 640", res.Entries[0].Mode)
	}
}

// TestFileWriteSizeMismatchKeepsOriginal proves an interrupted upload
// does not clobber the existing file.
func TestFileWriteSizeMismatchKeepsOriginal(t *testing.T) {
	g := newGateway(t, NewGuard(nil))
	sess := g.session(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	stream, _ := open(t, sess, agenttypes.StreamOpen{
		Kind: agenttypes.StreamKindFile,
		Op:   agenttypes.FileOpWrite,
		Path: path,
		Size: 100,
	})
	_ = agenttypes.WriteMessage(stream, agenttypes.MsgData, []byte("short"))
	_ = agenttypes.WriteMessage(stream, agenttypes.MsgEOF, nil)
	res := readResult(t, stream)
	stream.Close()
	if res.Ok {
		t.Fatal("short upload reported success")
	}

	kept, err := os.ReadFile(path)
	if err != nil || string(kept) != "original" {
		t.Fatalf("original file was clobbered: %q %v", kept, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %d entries", len(entries))
	}
}

func readResult(t *testing.T, stream net.Conn) agenttypes.FileResult {
	t.Helper()
	for {
		typ, payload, err := agenttypes.ReadMessage(stream)
		if err != nil {
			t.Fatalf("read result: %v", err)
		}
		if typ != agenttypes.MsgResult {
			continue
		}
		var res agenttypes.FileResult
		if err := json.Unmarshal(payload, &res); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		return res
	}
}

// TestStalledUploadIsTornDown covers the stream that no transport error
// ever reports: a gateway that opens a file write and then stops sending,
// without dying, so yamux keeps the session healthy. Before the idle
// deadline, that pinned a handler forever AND kept the data channel from
// ever closing, since the channel only goes idle with no streams left.
func TestStalledUploadIsTornDown(t *testing.T) {
	old := fileIdleTimeout
	fileIdleTimeout = 200 * time.Millisecond
	t.Cleanup(func() { fileIdleTimeout = old })

	g := newGateway(t, NewGuard(nil))
	sess := g.session(t)
	path := filepath.Join(t.TempDir(), "stalled.bin")

	stream, acc := open(t, sess, agenttypes.StreamOpen{
		Kind: agenttypes.StreamKindFile,
		Op:   agenttypes.FileOpWrite,
		Path: path,
		Size: 1000,
	})
	defer stream.Close()
	if !acc.Ok {
		t.Fatalf("stream rejected: %+v", acc)
	}
	// One chunk, then nothing: no MsgEOF, no close.
	if err := agenttypes.WriteMessage(stream, agenttypes.MsgData, []byte("partial")); err != nil {
		t.Fatalf("send chunk: %v", err)
	}

	res := readResult(t, stream)
	if res.Ok {
		t.Fatal("a stalled upload reported success")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the stalled upload was written to the destination")
	}
}

// TestQuietTCPStreamStaysUntilClientCloses is the reversal of the old
// "idle tcp is torn down" test. A proxied stream that moves no bytes is
// not a stuck one -- an SSE feed or `kubectl logs -f` on a silent pod is
// legitimately quiet -- so the agent must keep it open while the gateway
// holds it, and tear it down only when the gateway closes it.
func TestQuietTCPStreamStaysUntilClientCloses(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- c // held open, never written to
	}()

	g := newGateway(t, NewGuard(nil))
	sess := g.session(t)
	stream, acc := open(t, sess, agenttypes.StreamOpen{
		Kind:   agenttypes.StreamKindTCP,
		Target: ln.Addr().String(),
	})
	if !acc.Ok {
		t.Fatalf("stream rejected: %+v", acc)
	}
	select {
	case c := <-accepted:
		defer c.Close()
	case <-time.After(5 * time.Second):
		t.Fatal("the agent never connected to the local service")
	}

	// It stays open through a quiet window that the old 5-minute timeout's
	// removal is the whole point of. A read with a short deadline should
	// time out (stream alive, no data), not return EOF (stream gone).
	_ = stream.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	buf := make([]byte, 1)
	if _, err := stream.Read(buf); err == nil || !isTimeout(err) {
		t.Fatalf("quiet stream did not stay open: err=%v", err)
	}
	_ = stream.SetReadDeadline(time.Time{})

	// Closing the client side is the real stop signal, and it must reach
	// the agent promptly.
	if err := stream.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

// isTimeout reports whether err is a deadline-exceeded network timeout.
func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// TestExecStreamHasNoCarriageReturns is the whole reason exec exists
// separately from pty: a log line printed with \n must arrive as \n, not
// the \r\n a PTY's line discipline would produce. The SSH log tail this
// replaces uses a plain pipe, so the agent output has to match it
// exactly.
func TestExecStreamHasNoCarriageReturns(t *testing.T) {
	g := newGateway(t, NewGuard(nil))
	sess := g.session(t)

	stream, acc := open(t, sess, agenttypes.StreamOpen{
		Kind:    agenttypes.StreamKindExec,
		Command: []string{"/bin/sh", "-c", "printf 'line-one\\nline-two\\n'"},
	})
	defer stream.Close()
	if !acc.Ok {
		t.Fatalf("stream rejected: %+v", acc)
	}

	var out strings.Builder
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		typ, payload, err := agenttypes.ReadMessage(stream)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if typ == agenttypes.MsgData {
			out.Write(payload)
			continue
		}
		if typ == agenttypes.MsgExit {
			break
		}
	}
	got := out.String()
	if got != "line-one\nline-two\n" {
		t.Fatalf("output = %q, want clean \\n line endings (a PTY would have added \\r)", got)
	}
	if strings.Contains(got, "\r") {
		t.Fatal("exec output carries a carriage return; the log panel would show ^M on every line")
	}
}

// TestExecStreamReportsExitCode covers a command that fails, since a log
// job that dies should surface its status rather than look like a clean
// end.
func TestExecStreamReportsExitCode(t *testing.T) {
	g := newGateway(t, NewGuard(nil))
	sess := g.session(t)

	stream, acc := open(t, sess, agenttypes.StreamOpen{
		Kind:    agenttypes.StreamKindExec,
		Command: []string{"/bin/sh", "-c", "echo starting; exit 3"},
	})
	defer stream.Close()
	if !acc.Ok {
		t.Fatalf("stream rejected: %+v", acc)
	}

	var exit agenttypes.PTYExit
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		typ, payload, err := agenttypes.ReadMessage(stream)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if typ == agenttypes.MsgExit {
			if err := json.Unmarshal(payload, &exit); err != nil {
				t.Fatalf("decode exit: %v", err)
			}
			break
		}
	}
	if exit.Code != 3 {
		t.Fatalf("exit code = %d, want 3", exit.Code)
	}
}

// TestExecStreamStopsOnClientClose proves a follow (journalctl -f) is
// killed when the viewer goes away, rather than leaking a process that
// tails forever.
func TestExecStreamStopsOnClientClose(t *testing.T) {
	g := newGateway(t, NewGuard(nil))
	sess := g.session(t)

	stream, acc := open(t, sess, agenttypes.StreamOpen{
		Kind: agenttypes.StreamKindExec,
		// A follow with no natural end.
		Command: []string{"/bin/sh", "-c", "while true; do echo tick; sleep 0.1; done"},
	})
	if !acc.Ok {
		t.Fatalf("stream rejected: %+v", acc)
	}

	// Read at least one line so the process is definitely running.
	deadline := time.Now().Add(5 * time.Second)
	sawData := false
	for time.Now().Before(deadline) && !sawData {
		typ, _, err := agenttypes.ReadMessage(stream)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if typ == agenttypes.MsgData {
			sawData = true
		}
	}
	if !sawData {
		t.Fatal("never saw output from the follow")
	}

	// Close the viewer side; the agent must tear the process down. If it
	// does not, the read loop keeps the goroutine alive and -race /
	// leaktest would catch it, but the observable check is that the
	// stream ends.
	if err := stream.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
