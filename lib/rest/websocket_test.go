package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cws "github.com/coder/websocket"

	"vraxel.io/vraxel/lib/websocket"
)

func TestHandleWebSocket_Upgrade(t *testing.T) {
	// Verify that HandleWebSocket upgrades the connection and the handler
	// receives the path params.
	handler := HandleWebSocket(func(ctx context.Context, params map[string]string, conn *websocket.Conn) {
		defer conn.Close(websocket.StatusNormalClosure, "done")

		if got := params["userId"]; got != "42" {
			t.Errorf("expected userId=42, got %q", got)
		}

		// Echo one message to confirm the connection works
		msgType, data, err := conn.ReadMessage(ctx)
		if err != nil {
			return
		}
		conn.WriteMessage(ctx, msgType, data)
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = WithPathParams(r, map[string]string{"userId": "42"})
		handler(w, r)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := cws.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer c.CloseNow()

	want := []byte("hello")
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

func TestHandleWebSocket_QueryParamsMerged(t *testing.T) {
	// Verify that query params are merged into the params map and
	// path params take priority over query params.
	handler := HandleWebSocket(func(ctx context.Context, params map[string]string, conn *websocket.Conn) {
		defer conn.Close(websocket.StatusNormalClosure, "done")

		// Path param should win over query param with the same key
		if got := params["userId"]; got != "42" {
			t.Errorf("expected userId=42 (path param), got %q", got)
		}
		// Query-only param should be present
		if got := params["format"]; got != "raw" {
			t.Errorf("expected format=raw (query param), got %q", got)
		}
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = WithPathParams(r, map[string]string{"userId": "42"})
		handler(w, r)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Dial with query params: userId should be overridden by path param, format should pass through
	c, _, err := cws.Dial(ctx, "ws"+srv.URL[4:]+"?userId=99&format=raw", nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer c.CloseNow()

	// Read close frame
	_, _, _ = c.Read(ctx)
}

func TestHandleWebSocket_EchoWithConnection(t *testing.T) {
	// Full echo test: send multiple messages and verify they are echoed back.
	handler := HandleWebSocket(func(ctx context.Context, params map[string]string, conn *websocket.Conn) {
		defer conn.Close(websocket.StatusNormalClosure, "done")

		for {
			msgType, data, err := conn.ReadMessage(ctx)
			if err != nil {
				return
			}
			if err := conn.WriteMessage(ctx, msgType, data); err != nil {
				return
			}
		}
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(w, r)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := cws.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer c.CloseNow()

	messages := []string{"first", "second", "third"}
	for _, msg := range messages {
		if err := c.Write(ctx, cws.MessageText, []byte(msg)); err != nil {
			t.Fatalf("write %q error: %v", msg, err)
		}
		gotType, gotData, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("read error for %q: %v", msg, err)
		}
		if gotType != cws.MessageText {
			t.Fatalf("expected text message, got %v", gotType)
		}
		if string(gotData) != msg {
			t.Fatalf("expected %q, got %q", msg, gotData)
		}
	}
}
