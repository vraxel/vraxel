package websocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cws "github.com/coder/websocket"
)

func TestAcceptUpgrade(t *testing.T) {
	// Test that Accept upgrades an HTTP connection to WebSocket.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Accept(w, r, nil)
		if err != nil {
			t.Logf("accept error: %v", err)
			return
		}
		defer conn.Close(StatusNormalClosure, "done")

		// Echo one message to confirm it works
		ctx := r.Context()
		msgType, data, err := conn.ReadMessage(ctx)
		if err != nil {
			return
		}
		conn.WriteMessage(ctx, msgType, data)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := cws.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer c.CloseNow()

	want := []byte("test accept")
	if err := c.Write(ctx, cws.MessageText, want); err != nil {
		t.Fatalf("write error: %v", err)
	}

	gotType, gotData, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if gotType != cws.MessageText {
		t.Fatalf("expected text message, got %v", gotType)
	}
	if string(gotData) != string(want) {
		t.Fatalf("expected %q, got %q", want, gotData)
	}
}

func TestAcceptWithInsecureSkipVerify(t *testing.T) {
	// Test Accept with InsecureSkipVerify option set.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Accept(w, r, &AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Logf("accept error: %v", err)
			return
		}
		defer conn.Close(StatusNormalClosure, "done")

		ctx := r.Context()
		msgType, data, err := conn.ReadMessage(ctx)
		if err != nil {
			return
		}
		conn.WriteMessage(ctx, msgType, data)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := cws.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer c.CloseNow()

	want := []byte("insecure test")
	if err := c.Write(ctx, cws.MessageBinary, want); err != nil {
		t.Fatalf("write error: %v", err)
	}

	gotType, gotData, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if gotType != cws.MessageBinary {
		t.Fatalf("expected binary message, got %v", gotType)
	}
	if string(gotData) != string(want) {
		t.Fatalf("expected %q, got %q", want, gotData)
	}
}

func TestAcceptReturnsConn(t *testing.T) {
	// Verify Accept returns a *Conn whose Inner() is not nil.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Accept(w, r, nil)
		if err != nil {
			t.Logf("accept error: %v", err)
			return
		}
		if conn == nil {
			t.Error("Accept returned nil conn")
			return
		}
		if conn.Inner() == nil {
			t.Error("Inner() returned nil")
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
	// Read close frame
	_, _, _ = c.Read(ctx)
}

func TestAcceptNonWebSocketRequest(t *testing.T) {
	// A plain HTTP request (not a WebSocket upgrade) should fail.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	conn, err := Accept(w, r, nil)
	if err == nil {
		t.Fatal("expected error for non-WebSocket request, got nil")
	}
	if conn != nil {
		t.Fatal("expected nil conn for failed upgrade")
	}
}
