package websocket

import (
	"encoding/json"
	"fmt"
)

// Message type prefix bytes. Each WebSocket binary message starts with one of
// these bytes to distinguish the payload kind. Inspired by Kubernetes remote
// command / exec channel IDs.
const (
	// MsgData carries raw terminal I/O (stdin -> server, stdout/stderr -> client).
	MsgData byte = 0x00

	// MsgResize carries a JSON-encoded ResizePayload to change the PTY window size.
	MsgResize byte = 0x01

	// MsgConnect carries a JSON-encoded ConnectPayload sent by the client to
	// initiate an SSH session to a specific host.
	MsgConnect byte = 0x02

	// MsgStatus carries a JSON-encoded StatusPayload for session lifecycle
	// events (started, exited, error).
	MsgStatus byte = 0x03
)

// Default terminal dimensions used when the client does not specify them.
const (
	DefaultCols int = 80
	DefaultRows int = 24
)

// ResizePayload is sent by the client to resize the remote PTY.
type ResizePayload struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// ConnectPayload is sent by the client to initiate an SSH session.
// The target host is identified by the URL path parameter, not in this payload.
type ConnectPayload struct {
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
	User       string `json:"user,omitempty"`
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"privateKey,omitempty"`
	Port       int    `json:"port,omitempty"`
}

// StatusPayload is sent by the server to report session status.
// Status is a string such as "connected", "exited", or "error".
type StatusPayload struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// EncodeMessage prepends the message type byte to the payload.
// The returned slice is [type || payload].
func EncodeMessage(msgType byte, payload []byte) []byte {
	msg := make([]byte, 1+len(payload))
	msg[0] = msgType
	copy(msg[1:], payload)
	return msg
}

// DecodeMessage splits a raw WebSocket message into its type byte and payload.
// Returns an error if the message is empty.
func DecodeMessage(data []byte) (msgType byte, payload []byte, err error) {
	if len(data) == 0 {
		return 0, nil, fmt.Errorf("empty message")
	}
	return data[0], data[1:], nil
}

// EncodeStatusMessage creates a MsgStatus message with a JSON-encoded StatusPayload.
func EncodeStatusMessage(status *StatusPayload) ([]byte, error) {
	payload, err := json.Marshal(status)
	if err != nil {
		return nil, fmt.Errorf("marshal status payload: %w", err)
	}
	return EncodeMessage(MsgStatus, payload), nil
}

// EncodeResizeMessage creates a MsgResize message with a JSON-encoded ResizePayload.
func EncodeResizeMessage(resize *ResizePayload) ([]byte, error) {
	payload, err := json.Marshal(resize)
	if err != nil {
		return nil, fmt.Errorf("marshal resize payload: %w", err)
	}
	return EncodeMessage(MsgResize, payload), nil
}

// EncodeConnectMessage creates a MsgConnect message with a JSON-encoded ConnectPayload.
func EncodeConnectMessage(connect *ConnectPayload) ([]byte, error) {
	payload, err := json.Marshal(connect)
	if err != nil {
		return nil, fmt.Errorf("marshal connect payload: %w", err)
	}
	return EncodeMessage(MsgConnect, payload), nil
}

// DecodeResizePayload parses a JSON-encoded ResizePayload from raw bytes.
func DecodeResizePayload(data []byte) (*ResizePayload, error) {
	var p ResizePayload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("unmarshal resize payload: %w", err)
	}
	return &p, nil
}

// DecodeConnectPayload parses a JSON-encoded ConnectPayload from raw bytes.
// If Cols or Rows are zero, they are set to DefaultCols and DefaultRows.
func DecodeConnectPayload(data []byte) (*ConnectPayload, error) {
	var p ConnectPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("unmarshal connect payload: %w", err)
	}
	if p.Cols == 0 {
		p.Cols = DefaultCols
	}
	if p.Rows == 0 {
		p.Rows = DefaultRows
	}
	return &p, nil
}

// DecodeStatusPayload parses a JSON-encoded StatusPayload from raw bytes.
func DecodeStatusPayload(data []byte) (*StatusPayload, error) {
	var p StatusPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("unmarshal status payload: %w", err)
	}
	return &p, nil
}
