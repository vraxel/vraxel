package websocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cws "github.com/coder/websocket"
)

func TestNewConn(t *testing.T) {
	// Verify NewConn wraps an underlying connection and Inner returns it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := cws.Accept(w, r, &cws.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Logf("server accept error: %v", err)
			return
		}
		conn := NewConn(c)
		if conn.Inner() != c {
			t.Error("Inner() did not return the underlying connection")
		}
		conn.Close(StatusNormalClosure, "done")
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := cws.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer c.CloseNow()

	// Read close frame to let server finish
	_, _, _ = c.Read(ctx)
}

func TestConnWriteAndReadMessage(t *testing.T) {
	// Echo server: reads one message, writes it back.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := cws.Accept(w, r, &cws.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		conn := NewConn(c)
		defer conn.Close(StatusNormalClosure, "done")

		ctx := r.Context()
		msgType, data, err := conn.ReadMessage(ctx)
		if err != nil {
			t.Logf("server read error: %v", err)
			return
		}
		if err := conn.WriteMessage(ctx, msgType, data); err != nil {
			t.Logf("server write error: %v", err)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := cws.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	conn := NewConn(c)
	defer conn.Close(StatusNormalClosure, "bye")

	// Send a text message
	want := []byte("hello websocket")
	if err := conn.WriteMessage(ctx, cws.MessageText, want); err != nil {
		t.Fatalf("write error: %v", err)
	}

	// Read the echoed message
	gotType, gotData, err := conn.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if gotType != cws.MessageText {
		t.Fatalf("expected message type %v, got %v", cws.MessageText, gotType)
	}
	if string(gotData) != string(want) {
		t.Fatalf("expected %q, got %q", want, gotData)
	}
}

func TestConnWriteBinary(t *testing.T) {
	// Server reads a binary message.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := cws.Accept(w, r, &cws.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		conn := NewConn(c)
		defer conn.Close(StatusNormalClosure, "done")

		ctx := r.Context()
		msgType, data, err := conn.ReadMessage(ctx)
		if err != nil {
			t.Logf("server read error: %v", err)
			return
		}
		// Echo back
		if err := conn.WriteMessage(ctx, msgType, data); err != nil {
			t.Logf("server write error: %v", err)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := cws.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	conn := NewConn(c)
	defer conn.Close(StatusNormalClosure, "bye")

	// Use WriteBinary convenience method
	binaryData := []byte{0x00, 0x01, 0x02, 0xFF}
	if err := conn.WriteBinary(ctx, binaryData); err != nil {
		t.Fatalf("WriteBinary error: %v", err)
	}

	gotType, gotData, err := conn.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if gotType != cws.MessageBinary {
		t.Fatalf("expected MessageBinary, got %v", gotType)
	}
	if len(gotData) != len(binaryData) {
		t.Fatalf("expected %d bytes, got %d", len(binaryData), len(gotData))
	}
	for i, b := range gotData {
		if b != binaryData[i] {
			t.Fatalf("byte %d: expected 0x%02x, got 0x%02x", i, binaryData[i], b)
		}
	}
}

func TestConnClose(t *testing.T) {
	// Verify Close sends a close frame that the peer receives.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := cws.Accept(w, r, &cws.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		conn := NewConn(c)

		ctx := r.Context()
		// Try to read -- should get a close error after client closes
		_, _, err = conn.ReadMessage(ctx)
		if err == nil {
			t.Error("expected error after client close, got nil")
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := cws.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	conn := NewConn(c)

	// Close with normal closure
	if err := conn.Close(StatusNormalClosure, "goodbye"); err != nil {
		t.Fatalf("close error: %v", err)
	}
}

func TestStatusCodeConstants(t *testing.T) {
	// Verify re-exported status codes match the underlying library.
	if StatusNormalClosure != cws.StatusNormalClosure {
		t.Fatalf("StatusNormalClosure mismatch: %v != %v", StatusNormalClosure, cws.StatusNormalClosure)
	}
	if StatusGoingAway != cws.StatusGoingAway {
		t.Fatalf("StatusGoingAway mismatch: %v != %v", StatusGoingAway, cws.StatusGoingAway)
	}
	if StatusInternalError != cws.StatusInternalError {
		t.Fatalf("StatusInternalError mismatch: %v != %v", StatusInternalError, cws.StatusInternalError)
	}
}

func TestConnMultipleMessages(t *testing.T) {
	// Test sending and receiving multiple messages in sequence.
	const messageCount = 5

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := cws.Accept(w, r, &cws.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		conn := NewConn(c)
		defer conn.Close(StatusNormalClosure, "done")

		ctx := r.Context()
		for range messageCount {
			msgType, data, err := conn.ReadMessage(ctx)
			if err != nil {
				return
			}
			if err := conn.WriteMessage(ctx, msgType, data); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := cws.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	conn := NewConn(c)
	defer conn.Close(StatusNormalClosure, "bye")

	for i := range messageCount {
		msg := []byte{byte(i), byte(i + 1), byte(i + 2)}
		if err := conn.WriteBinary(ctx, msg); err != nil {
			t.Fatalf("write %d error: %v", i, err)
		}

		gotType, gotData, err := conn.ReadMessage(ctx)
		if err != nil {
			t.Fatalf("read %d error: %v", i, err)
		}
		if gotType != cws.MessageBinary {
			t.Fatalf("message %d: expected binary, got %v", i, gotType)
		}
		if len(gotData) != len(msg) {
			t.Fatalf("message %d: expected %d bytes, got %d", i, len(msg), len(gotData))
		}
	}
}

// TestDrainReads_DiscardsClientDataThenCancelsOnClose pins the fix for the
// PaaS-deploy "连接断开" bug: the frontend TaskTerminalDialog sends resize
// data frames to every stream it opens, including write-only progress tails.
// DrainReads must DISCARD those (not close the connection, as coder's
// CloseRead would via StatusPolicyViolation), yet still cancel the ctx when
// the client actually disconnects.
func TestDrainReads_DiscardsClientDataThenCancelsOnClose(t *testing.T) {
	handlerDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := cws.Accept(w, r, &cws.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		conn := NewConn(c)
		defer conn.Close(cws.StatusNormalClosure, "")
		<-conn.DrainReads(context.Background()).Done() // unwinds only on client disconnect
		close(handlerDone)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cli, _, err := cws.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// A client data frame (like a resize) must be discarded, not close the
	// stream: the old CloseRead closed here with StatusPolicyViolation.
	if err := cli.Write(ctx, cws.MessageBinary, []byte("resize")); err != nil {
		t.Fatalf("client write: %v", err)
	}
	time.Sleep(150 * time.Millisecond) // let the drain process the frame
	select {
	case <-handlerDone:
		t.Fatal("server unwound after a client data frame; DrainReads must discard, not close")
	default:
	}

	// A real disconnect must cancel the drain ctx so the handler unwinds.
	_ = cli.Close(cws.StatusNormalClosure, "")
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("DrainReads did not cancel ctx on client close")
	}
}
