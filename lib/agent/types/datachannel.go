package types

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// DataChannelPath is the WSS endpoint the agent dials to establish its
// single data channel (design §4.3). One physical connection per host;
// every concurrent session is a yamux logical stream on top of it.
const DataChannelPath = ProtocolPathPrefix + "data-channel"

// Stream kinds. The design table lists five (http / stream / tcp / pty /
// file), but on the agent side http, stream and tcp collapse into one
// behaviour -- open a TCP connection to a loopback port and copy bytes --
// because the gateway keeps all HTTP semantics on its own side (design
// §6.6: "最简实现是全部按 stream 透传", which is why agentdialer.DialFor
// returns a net.Conn). So the agent implements three kinds, and only two
// behaviours: raw passthrough (tcp) and framed (pty / file).
const (
	// StreamKindTCP is raw bidirectional passthrough to a loopback port.
	// Serves the design's http, stream and tcp kinds: middleware APIs,
	// k8s REST, k8s pod exec SPDY, HTTP and TCP probes.
	StreamKindTCP = "tcp"
	// StreamKindPTY forks a process under a PTY. Serves the web terminal,
	// which needs a real terminal for interactivity, Ctrl-C and resize.
	StreamKindPTY = "pty"
	// StreamKindExec forks a process WITHOUT a PTY and streams its stdout
	// (merged with stderr) line by line. Serves service log tail
	// (journalctl -f): the log panel scans lines, it is not a terminal,
	// and a PTY's line discipline would translate every \n into \r\n and
	// leave a ^M on each line. The distinction is by intent, not by reuse
	// -- this is exactly how the SSH log tail streams today (a plain
	// pipe, no pty-req), so the agent path stays byte-for-byte identical.
	StreamKindExec = "exec"
	// StreamKindFile runs local filesystem operations.
	StreamKindFile = "file"
)

// MaxStreamHeaderBytes caps the opening header of a stream. The header is
// a small JSON object; anything larger is a malformed or hostile peer.
const MaxStreamHeaderBytes = 32 * 1024

// StreamOpen is the first message on every yamux stream, sent by the
// gateway, answered by StreamAccept.
//
// The design routed these parameters through a control-channel
// channel.open frame, because v5 had the agent dial a fresh WS per
// session and something had to trigger the dial. With one multiplexed
// data channel (v6, §4.3) the gateway can just open a stream, so putting
// the parameters in the stream header removes a control-channel round
// trip from every request on the hot path (k8s list, middleware API) and
// removes the cross-channel state needed to match a session id to a
// later-arriving stream. The security properties are unchanged: the
// agent enforces the same target rules wherever the request arrives.
type StreamOpen struct {
	// Kind is one of the StreamKind* constants.
	Kind string `json:"kind"`

	// TimeoutMs bounds establishing the stream (for tcp, the connect to
	// the local service). It does not bound the session: pty, file and
	// long-lived tcp streams are governed by idle timeouts instead.
	TimeoutMs int `json:"timeoutMs,omitempty"`

	// --- tcp ---
	// Target is host:port. The agent rejects anything that is not a
	// loopback address (see the package guard); the port allowlist is a
	// second, configurable check on top of that.
	Target string `json:"target,omitempty"`

	// --- pty ---
	// Command is argv for the process to run under the PTY. Empty means
	// the agent's configured login shell.
	Command []string `json:"command,omitempty"`
	Cols    uint16   `json:"cols,omitempty"`
	Rows    uint16   `json:"rows,omitempty"`
	// Dir is the working directory for the PTY process (optional).
	Dir string `json:"dir,omitempty"`

	// --- file ---
	// Op is one of the FileOp* constants.
	Op string `json:"op,omitempty"`
	// Path is the target path for the file op; Dest is the second path
	// for rename.
	Path string `json:"path,omitempty"`
	Dest string `json:"dest,omitempty"`
	// Mode is the permission bits for write / mkdir / chmod.
	Mode uint32 `json:"mode,omitempty"`
	// Size is the byte count the gateway will stream for a write op.
	// Required: it is what lets the agent stop reading at the end of the
	// payload and keep the stream usable for the reply.
	Size int64 `json:"size,omitempty"`
	// Offset / Length window a read op; zero Length means "to EOF".
	Offset int64 `json:"offset,omitempty"`
	Length int64 `json:"length,omitempty"`
}

// StreamAccept is the agent's answer to StreamOpen. A stream is only
// usable after Ok; on failure the agent closes it immediately.
type StreamAccept struct {
	Ok    bool   `json:"ok"`
	Code  string `json:"code,omitempty"`
	Error string `json:"error,omitempty"`
}

// Stream rejection codes.
const (
	StreamErrTargetNotAllowed = "target_not_allowed"
	StreamErrDialFailed       = "dial_failed"
	StreamErrUnknownKind      = "unknown_kind"
	StreamErrOpFailed         = "op_failed"
)

// PTY / file streams stay framed after the header, because both need
// out-of-band messages interleaved with the payload (a terminal resize
// arriving between two chunks of shell output, a file op's result after
// its body). Raw kinds carry no framing at all -- they are a net.Conn.
const (
	// MsgData carries payload bytes in either direction.
	MsgData byte = 1
	// MsgResize carries a PTYResize JSON body (gateway -> agent).
	MsgResize byte = 2
	// MsgExit carries a PTYExit JSON body (agent -> gateway) and is the
	// last message on a pty stream.
	MsgExit byte = 3
	// MsgResult carries a FileResult JSON body (agent -> gateway).
	MsgResult byte = 4
	// MsgEOF marks the end of a data run without closing the stream, so
	// a reply can still follow (file read, file write body).
	MsgEOF byte = 5
)

// MaxMessageBytes caps one framed message payload. Bulk transfer is
// chunked below this; the cap exists so a corrupt length cannot make the
// receiver allocate arbitrarily.
const MaxMessageBytes = 1 << 20

// PTYResize is the MsgResize body.
type PTYResize struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// PTYExit is the MsgExit body.
type PTYExit struct {
	Code  int    `json:"code"`
	Error string `json:"error,omitempty"`
}

// File ops.
const (
	FileOpList   = "list"
	FileOpStat   = "stat"
	FileOpRead   = "read"
	FileOpWrite  = "write"
	FileOpMkdir  = "mkdir"
	FileOpRemove = "remove"
	FileOpRename = "rename"
	FileOpChmod  = "chmod"
)

// FileEntry is one directory entry in a list result.
type FileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Mode    uint32 `json:"mode"`
	IsDir   bool   `json:"isDir"`
	ModUnix int64  `json:"modUnix"`
	// Symlink is the link target when the entry is a symlink.
	Symlink string `json:"symlink,omitempty"`
}

// FileResult is the MsgResult body: the outcome of a file op, sent after
// any streamed payload.
type FileResult struct {
	Ok      bool        `json:"ok"`
	Error   string      `json:"error,omitempty"`
	Entries []FileEntry `json:"entries,omitempty"`
	Entry   *FileEntry  `json:"entry,omitempty"`
	// Written is the byte count accepted by a write op.
	Written int64 `json:"written,omitempty"`
}

// WriteMessage writes one framed message: 1-byte type, 4-byte big-endian
// length, payload.
func WriteMessage(w io.Writer, typ byte, payload []byte) error {
	if len(payload) > MaxMessageBytes {
		return fmt.Errorf("message payload %d bytes exceeds %d limit", len(payload), MaxMessageBytes)
	}
	var hdr [5]byte
	hdr[0] = typ
	binary.BigEndian.PutUint32(hdr[1:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := w.Write(payload)
	return err
}

// WriteJSONMessage marshals v and writes it as one framed message.
func WriteJSONMessage(w io.Writer, typ byte, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal message type %d: %w", typ, err)
	}
	return WriteMessage(w, typ, data)
}

// ReadMessage reads one framed message. Returns io.EOF at a clean end of
// stream.
func ReadMessage(r io.Reader) (byte, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[1:])
	if n > MaxMessageBytes {
		return 0, nil, fmt.Errorf("message payload %d bytes exceeds %d limit", n, MaxMessageBytes)
	}
	if n == 0 {
		return hdr[0], nil, nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, nil, err
	}
	return hdr[0], buf, nil
}

// WriteStreamHeader writes a length-prefixed JSON header (the StreamOpen
// / StreamAccept handshake). Separate from WriteMessage because the
// handshake precedes any framing mode and must be readable identically
// by raw and framed kinds.
func WriteStreamHeader(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal stream header: %w", err)
	}
	if len(data) > MaxStreamHeaderBytes {
		return fmt.Errorf("stream header %d bytes exceeds %d limit", len(data), MaxStreamHeaderBytes)
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(data)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// ReadStreamHeader reads a length-prefixed JSON header into v.
func ReadStreamHeader(r io.Reader, v any) error {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > MaxStreamHeaderBytes {
		return fmt.Errorf("stream header %d bytes exceeds %d limit", n, MaxStreamHeaderBytes)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	return json.Unmarshal(buf, v)
}
