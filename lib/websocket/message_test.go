package websocket

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestEncodeDecodeDataMessage(t *testing.T) {
	data := []byte("hello world")
	encoded := EncodeMessage(MsgData, data)

	if len(encoded) != len(data)+1 {
		t.Fatalf("expected encoded length %d, got %d", len(data)+1, len(encoded))
	}
	if encoded[0] != MsgData {
		t.Fatalf("expected first byte %d, got %d", MsgData, encoded[0])
	}

	msgType, payload, err := DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgType != MsgData {
		t.Fatalf("expected message type %d, got %d", MsgData, msgType)
	}
	if !bytes.Equal(payload, data) {
		t.Fatalf("expected payload %q, got %q", data, payload)
	}
}

func TestEncodeDecodeEmptyMessage(t *testing.T) {
	encoded := EncodeMessage(MsgData, nil)

	if len(encoded) != 1 {
		t.Fatalf("expected encoded length 1, got %d", len(encoded))
	}
	if encoded[0] != MsgData {
		t.Fatalf("expected first byte %d, got %d", MsgData, encoded[0])
	}

	msgType, payload, err := DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgType != MsgData {
		t.Fatalf("expected message type %d, got %d", MsgData, msgType)
	}
	if len(payload) != 0 {
		t.Fatalf("expected empty payload, got %q", payload)
	}
}

func TestDecodeMessageEmpty(t *testing.T) {
	_, _, err := DecodeMessage(nil)
	if err == nil {
		t.Fatal("expected error for empty message, got nil")
	}

	_, _, err = DecodeMessage([]byte{})
	if err == nil {
		t.Fatal("expected error for empty message, got nil")
	}
}

func TestEncodeDecodeStatusMessage(t *testing.T) {
	status := &StatusPayload{
		Status:  "connected",
		Message: "session started",
	}

	encoded, err := EncodeStatusMessage(status)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if encoded[0] != MsgStatus {
		t.Fatalf("expected first byte %d, got %d", MsgStatus, encoded[0])
	}

	// Decode the message
	msgType, payload, err := DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgType != MsgStatus {
		t.Fatalf("expected message type %d, got %d", MsgStatus, msgType)
	}

	// Parse status payload via DecodeStatusPayload
	decoded, err := DecodeStatusPayload(payload)
	if err != nil {
		t.Fatalf("failed to decode status payload: %v", err)
	}
	if decoded.Status != status.Status {
		t.Fatalf("expected status %q, got %q", status.Status, decoded.Status)
	}
	if decoded.Message != status.Message {
		t.Fatalf("expected message %q, got %q", status.Message, decoded.Message)
	}
}

func TestEncodeDecodeStatusMessageWithError(t *testing.T) {
	status := &StatusPayload{
		Status:  "error",
		Message: "connection failed: timeout",
	}

	encoded, err := EncodeStatusMessage(status)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgType, payload, err := DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgType != MsgStatus {
		t.Fatalf("expected message type %d, got %d", MsgStatus, msgType)
	}

	decoded, err := DecodeStatusPayload(payload)
	if err != nil {
		t.Fatalf("failed to decode status payload: %v", err)
	}
	if decoded.Status != "error" {
		t.Fatalf("expected status %q, got %q", "error", decoded.Status)
	}
	if decoded.Message != status.Message {
		t.Fatalf("expected message %q, got %q", status.Message, decoded.Message)
	}
}

func TestDecodeStatusPayloadInvalid(t *testing.T) {
	_, err := DecodeStatusPayload([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestDecodeStatusPayloadOmitEmptyMessage(t *testing.T) {
	// Message is omitempty — when empty string, it should be omitted from JSON
	status := &StatusPayload{Status: "connected"}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decoded, err := DecodeStatusPayload(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded.Status != "connected" {
		t.Fatalf("expected status %q, got %q", "connected", decoded.Status)
	}
	if decoded.Message != "" {
		t.Fatalf("expected empty message, got %q", decoded.Message)
	}
}

func TestEncodeDecodeResizeMessage(t *testing.T) {
	resize := &ResizePayload{
		Cols: 120,
		Rows: 40,
	}

	encoded, err := EncodeResizeMessage(resize)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if encoded[0] != MsgResize {
		t.Fatalf("expected first byte %d, got %d", MsgResize, encoded[0])
	}

	// Decode the message
	msgType, payload, err := DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgType != MsgResize {
		t.Fatalf("expected message type %d, got %d", MsgResize, msgType)
	}

	// Parse resize payload
	decoded, err := DecodeResizePayload(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded.Cols != resize.Cols {
		t.Fatalf("expected cols %d, got %d", resize.Cols, decoded.Cols)
	}
	if decoded.Rows != resize.Rows {
		t.Fatalf("expected rows %d, got %d", resize.Rows, decoded.Rows)
	}
}

func TestDecodeResizePayloadInvalid(t *testing.T) {
	_, err := DecodeResizePayload([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestDecodeConnectPayload(t *testing.T) {
	connect := ConnectPayload{
		Cols:     80,
		Rows:     24,
		User:     "root",
		Password: "secret",
		Port:     22,
	}
	data, _ := json.Marshal(connect)

	decoded, err := DecodeConnectPayload(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded.Cols != 80 {
		t.Fatalf("expected cols 80, got %d", decoded.Cols)
	}
	if decoded.Rows != 24 {
		t.Fatalf("expected rows 24, got %d", decoded.Rows)
	}
	if decoded.User != "root" {
		t.Fatalf("expected user %q, got %q", "root", decoded.User)
	}
	if decoded.Password != "secret" {
		t.Fatalf("expected password %q, got %q", "secret", decoded.Password)
	}
	if decoded.Port != 22 {
		t.Fatalf("expected port 22, got %d", decoded.Port)
	}
}

func TestDecodeConnectPayloadWithPrivateKey(t *testing.T) {
	data := []byte(`{"cols":100,"rows":30,"user":"deploy","privateKey":"-----BEGIN RSA PRIVATE KEY-----\nMIIE..."}`)

	decoded, err := DecodeConnectPayload(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded.User != "deploy" {
		t.Fatalf("expected user %q, got %q", "deploy", decoded.User)
	}
	if decoded.PrivateKey == "" {
		t.Fatal("expected non-empty privateKey")
	}
	if decoded.Password != "" {
		t.Fatalf("expected empty password, got %q", decoded.Password)
	}
}

func TestDecodeConnectPayloadDefaults(t *testing.T) {
	// Only provide optional fields; cols and rows should get defaults
	data := []byte(`{"user":"admin"}`)

	decoded, err := DecodeConnectPayload(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded.Cols != DefaultCols {
		t.Fatalf("expected default cols %d, got %d", DefaultCols, decoded.Cols)
	}
	if decoded.Rows != DefaultRows {
		t.Fatalf("expected default rows %d, got %d", DefaultRows, decoded.Rows)
	}
}

func TestDecodeConnectPayloadZeroValues(t *testing.T) {
	// Explicitly set cols/rows to 0 -- should still get defaults
	data := []byte(`{"cols":0,"rows":0}`)

	decoded, err := DecodeConnectPayload(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded.Cols != DefaultCols {
		t.Fatalf("expected default cols %d, got %d", DefaultCols, decoded.Cols)
	}
	if decoded.Rows != DefaultRows {
		t.Fatalf("expected default rows %d, got %d", DefaultRows, decoded.Rows)
	}
}

func TestDecodeConnectPayloadInvalid(t *testing.T) {
	_, err := DecodeConnectPayload([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestEncodeConnectMessage(t *testing.T) {
	connect := &ConnectPayload{
		Cols:     120,
		Rows:     40,
		User:     "root",
		Password: "pass",
		Port:     2222,
	}

	encoded, err := EncodeConnectMessage(connect)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if encoded[0] != MsgConnect {
		t.Fatalf("expected first byte %d, got %d", MsgConnect, encoded[0])
	}

	// Round-trip: decode the framed message, then parse the payload
	msgType, payload, err := DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgType != MsgConnect {
		t.Fatalf("expected message type %d, got %d", MsgConnect, msgType)
	}

	decoded, err := DecodeConnectPayload(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded.Cols != 120 {
		t.Fatalf("expected cols 120, got %d", decoded.Cols)
	}
	if decoded.Rows != 40 {
		t.Fatalf("expected rows 40, got %d", decoded.Rows)
	}
	if decoded.User != "root" {
		t.Fatalf("expected user %q, got %q", "root", decoded.User)
	}
	if decoded.Password != "pass" {
		t.Fatalf("expected password %q, got %q", "pass", decoded.Password)
	}
	if decoded.Port != 2222 {
		t.Fatalf("expected port 2222, got %d", decoded.Port)
	}
}

func TestMessageTypeConstants(t *testing.T) {
	// Verify the constants have the expected values
	if MsgData != 0x00 {
		t.Fatalf("expected MsgData=0x00, got 0x%02x", MsgData)
	}
	if MsgResize != 0x01 {
		t.Fatalf("expected MsgResize=0x01, got 0x%02x", MsgResize)
	}
	if MsgConnect != 0x02 {
		t.Fatalf("expected MsgConnect=0x02, got 0x%02x", MsgConnect)
	}
	if MsgStatus != 0x03 {
		t.Fatalf("expected MsgStatus=0x03, got 0x%02x", MsgStatus)
	}
}

func TestEncodeMessagePreserveBinaryData(t *testing.T) {
	// Test with binary data containing all byte values
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}

	encoded := EncodeMessage(MsgData, data)
	msgType, payload, err := DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgType != MsgData {
		t.Fatalf("expected message type %d, got %d", MsgData, msgType)
	}
	if !bytes.Equal(payload, data) {
		t.Fatal("binary data not preserved through encode/decode")
	}
}

func TestEncodeMessageWithDifferentTypes(t *testing.T) {
	payload := []byte("test")

	tests := []struct {
		name    string
		msgType byte
	}{
		{"data", MsgData},
		{"resize", MsgResize},
		{"connect", MsgConnect},
		{"status", MsgStatus},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeMessage(tt.msgType, payload)
			decoded, data, err := DecodeMessage(encoded)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if decoded != tt.msgType {
				t.Fatalf("expected type %d, got %d", tt.msgType, decoded)
			}
			if !bytes.Equal(data, payload) {
				t.Fatalf("expected payload %q, got %q", payload, data)
			}
		})
	}
}
