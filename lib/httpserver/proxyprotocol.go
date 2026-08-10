package httpserver

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/utils/bytesutil"
)

type proxyProtocolConn struct {
	net.Conn
	once       sync.Once
	remoteAddr net.Addr
	readErr    error
}

func newProxyProtocolConn(c net.Conn) net.Conn {
	return &proxyProtocolConn{
		Conn: c,
	}
}

var proxyProtocolReadErrorLogger = logger.WithThrottler("proxyProtocolReadError", 5*time.Second)

func (ppc *proxyProtocolConn) init() {
	ppc.once.Do(func() {
		addr, err := readProxyProto(ppc.Conn)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				proxyProtocolReadErrorLogger.Errorf("cannot read proxy proto conn for TCP addr %q: %s", ppc.Conn.RemoteAddr(), err)
			}
			ppc.readErr = err
			return
		}
		if addr == nil {
			addr = ppc.Conn.RemoteAddr()
		}
		ppc.remoteAddr = addr
	})
}

func (ppc *proxyProtocolConn) Read(p []byte) (int, error) {
	ppc.init()
	if ppc.readErr != nil {
		return 0, ppc.readErr
	}
	return ppc.Conn.Read(p)
}

func (ppc *proxyProtocolConn) RemoteAddr() net.Addr {
	ppc.init()
	if ppc.readErr != nil {
		return ppc.Conn.RemoteAddr()
	}
	return ppc.remoteAddr
}

func readProxyProto(r io.Reader) (net.Addr, error) {
	bb := bbPool.Get()
	defer bbPool.Put(bb)

	// Read the first 16 bytes of proxy protocol header:
	// - bytes 0-11: v2Identifier
	// - byte 12: version and command
	// - byte 13: family and protocol
	// - bytes 14-15: payload length
	//
	// See https://www.haproxy.org/download/1.8/doc/proxy-protocol.txt
	bb.B = bytesutil.ResizeNoCopyMayOverallocate(bb.B, 16)
	if _, err := io.ReadFull(r, bb.B); err != nil {
		return nil, fmt.Errorf("cannot read proxy protocol header: %w", err)
	}
	command := bb.B[12] & 0x0f
	family := bb.B[13] >> 4
	blockLen, err := validateProxyProtoHeader(bb.B)
	if err != nil {
		return nil, err
	}

	// Read the protocol block itself
	bb.B = bytesutil.ResizeNoCopyMayOverallocate(bb.B, blockLen)
	if _, err := io.ReadFull(r, bb.B); err != nil {
		return nil, fmt.Errorf("cannot read proxy protocol block with the length %d bytes: %w", blockLen, err)
	}
	switch command {
	case 0:
		// Proxy LOCAL command. Ignore the protocol block. The real sender address should be used.
		return nil, nil
	case 1:
		// Parse the protocol block according to the family.
		return readProxyProtoAddrBlock(family, bb.B)
	default:
		return nil, fmt.Errorf("the proxy protocol implementation doesn't support command %d; supported values: 0, 1", command)
	}
}

// validateProxyProtoHeader checks the 16-byte v2 header in hdr (identifier,
// version, proto/command) and returns the declared protocol block length.
func validateProxyProtoHeader(hdr []byte) (int, error) {
	ident := hdr[:12]
	if string(ident) != v2Identifier {
		return 0, fmt.Errorf("unexpected proxy protocol header: %q; want %q", ident, v2Identifier)
	}
	version := hdr[12] >> 4
	command := hdr[12] & 0x0f
	proto := hdr[13] & 0x0f
	if version != 2 {
		return 0, fmt.Errorf("unsupported proxy protocol version, only v2 protocol version is supported, got: %d", version)
	}
	// check for supported proto:
	switch {
	case proto == 0 && command == 0:
		// 0 - UNSPEC with LOCAL command 0. Common use case for load balancer health checks.
	case proto == 1:
		// 1 - TCP (aka STREAM).
	default:
		return 0, fmt.Errorf("the proxy protocol implementation doesn't support proto %d and command: %d; expecting proto 1 or proto 0 with command 0", proto, command)
	}
	// The length of the remainder of the header including any TLVs in network byte order
	// 0, 1, 2
	blockLen := int(binary.BigEndian.Uint16(hdr[14:16]))
	// in general RFC doesn't limit block length, but for sanity check lets limit it to 2kb
	// in theory TLVs may occupy some space
	if blockLen > 2048 {
		return 0, fmt.Errorf("too big proxy protocol block length: %d; it mustn't exceed 2048 bytes", blockLen)
	}
	return blockLen, nil
}

// readProxyProtoAddrBlock parses the PROXY command protocol block according to
// the address family and returns the conveyed source address.
func readProxyProtoAddrBlock(family byte, block []byte) (net.Addr, error) {
	switch family {
	case 1:
		// ipv4 (aka AF_INET)
		if len(block) < 12 {
			return nil, fmt.Errorf("cannot read ipv4 address from proxy protocol block with the length %d bytes; expected at least 12 bytes", len(block))
		}
		remoteAddr := &net.TCPAddr{
			IP:   net.IPv4(block[0], block[1], block[2], block[3]),
			Port: int(binary.BigEndian.Uint16(block[8:10])),
		}
		return remoteAddr, nil
	case 2:
		// ipv6 (aka AF_INET6)
		if len(block) < 36 {
			return nil, fmt.Errorf("cannot read ipv6 address from proxy protocol block with the length %d bytes; expected at least 36 bytes", len(block))
		}
		remoteAddr := &net.TCPAddr{
			IP:   block[0:16],
			Port: int(binary.BigEndian.Uint16(block[32:34])),
		}
		return remoteAddr, nil
	default:
		return nil, fmt.Errorf("the proxy protocol implementation doesn't support protocol family %d; supported values: 1, 2", family)
	}
}

const v2Identifier = "\r\n\r\n\x00\r\nQUIT\n"

var bbPool bytesutil.ByteBufferPool
