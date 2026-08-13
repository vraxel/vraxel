package datachan

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// fileChunk is the payload size for streamed file bytes. Large enough to
// keep syscall overhead low, small enough that a transfer never holds a
// meaningful amount of the agent's memory -- the whole point of streaming
// rather than reading a file in (design §5.3).
const fileChunk = 256 * 1024

// fileIdleTimeout bounds how long a file stream may sit without a
// message. A gateway that opens an upload and then stalls -- without
// dying, so yamux never notices -- would otherwise block its handler
// forever and keep the data channel from ever closing (design §4.3).
// Refreshed per message rather than watched by a goroutine, because
// every blocking operation here is a single stream read or write.
var fileIdleTimeout = 5 * time.Minute

// refreshDeadline re-arms the idle deadline before a blocking operation.
func refreshDeadline(stream net.Conn) {
	_ = stream.SetDeadline(time.Now().Add(fileIdleTimeout))
}

// serveFile runs one local filesystem operation.
//
// The agent already runs as root on the host, and the SFTP path it
// replaces had the same reach, so there is no path sandbox here: adding
// one would silently break the existing file manager rather than add
// security. What did change is the direction -- the host is never dialled.
func (c *Channel) serveFile(ctx context.Context, stream net.Conn, open agenttypes.StreamOpen) {
	refreshDeadline(stream)
	if err := accept(stream); err != nil {
		return
	}
	var mu sync.Mutex

	switch open.Op {
	case agenttypes.FileOpList:
		entries, err := listDir(open.Path)
		respond(&mu, stream, agenttypes.FileResult{Ok: err == nil, Error: errText(err), Entries: entries})
	case agenttypes.FileOpStat:
		entry, err := statEntry(open.Path)
		respond(&mu, stream, agenttypes.FileResult{Ok: err == nil, Error: errText(err), Entry: entry})
	case agenttypes.FileOpRead:
		c.fileRead(ctx, stream, &mu, open)
	case agenttypes.FileOpWrite:
		n, err := fileWrite(stream, open)
		respond(&mu, stream, agenttypes.FileResult{Ok: err == nil, Error: errText(err), Written: n})
	case agenttypes.FileOpMkdir:
		err := os.MkdirAll(open.Path, fileMode(open.Mode, 0o755))
		respond(&mu, stream, agenttypes.FileResult{Ok: err == nil, Error: errText(err)})
	case agenttypes.FileOpRemove:
		err := os.RemoveAll(open.Path)
		respond(&mu, stream, agenttypes.FileResult{Ok: err == nil, Error: errText(err)})
	case agenttypes.FileOpRename:
		err := os.Rename(open.Path, open.Dest)
		respond(&mu, stream, agenttypes.FileResult{Ok: err == nil, Error: errText(err)})
	case agenttypes.FileOpChmod:
		err := os.Chmod(open.Path, fileMode(open.Mode, 0o644))
		respond(&mu, stream, agenttypes.FileResult{Ok: err == nil, Error: errText(err)})
	default:
		respond(&mu, stream, agenttypes.FileResult{Error: "unknown file op " + open.Op})
	}
}

// fileRead streams a file (or a window of it) as data messages, ends the
// run with MsgEOF, then reports the outcome. The result comes last so a
// mid-transfer failure is still reportable on the same stream.
func (c *Channel) fileRead(ctx context.Context, stream net.Conn, mu *sync.Mutex, open agenttypes.StreamOpen) {
	f, err := os.Open(open.Path)
	if err != nil {
		respond(mu, stream, agenttypes.FileResult{Error: err.Error()})
		return
	}
	defer f.Close()

	if open.Offset > 0 {
		if _, err := f.Seek(open.Offset, io.SeekStart); err != nil {
			respond(mu, stream, agenttypes.FileResult{Error: err.Error()})
			return
		}
	}
	var src io.Reader = f
	if open.Length > 0 {
		src = io.LimitReader(f, open.Length)
	}

	buf := make([]byte, fileChunk)
	for {
		if ctx.Err() != nil {
			return
		}
		n, rerr := src.Read(buf)
		if n > 0 {
			refreshDeadline(stream)
			mu.Lock()
			werr := agenttypes.WriteMessage(stream, agenttypes.MsgData, buf[:n])
			mu.Unlock()
			if werr != nil {
				return
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			respond(mu, stream, agenttypes.FileResult{Error: rerr.Error()})
			return
		}
	}
	mu.Lock()
	_ = agenttypes.WriteMessage(stream, agenttypes.MsgEOF, nil)
	mu.Unlock()
	respond(mu, stream, agenttypes.FileResult{Ok: true})
}

// fileWrite consumes data messages until MsgEOF and writes them out.
//
// The bytes go to a temp file in the destination directory and are
// renamed into place only after the last one lands, so an interrupted
// upload leaves the previous file intact instead of a truncated one.
func fileWrite(stream net.Conn, open agenttypes.StreamOpen) (int64, error) {
	dir := filepath.Dir(open.Path)
	tmp, err := os.CreateTemp(dir, ".vraxel-upload-*")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	var written int64
	for {
		refreshDeadline(stream)
		typ, payload, err := agenttypes.ReadMessage(stream)
		if err != nil {
			return written, fmt.Errorf("read upload: %w", err)
		}
		if typ == agenttypes.MsgEOF {
			break
		}
		if typ != agenttypes.MsgData {
			continue
		}
		n, err := tmp.Write(payload)
		written += int64(n)
		if err != nil {
			return written, err
		}
		if open.Size > 0 && written > open.Size {
			return written, fmt.Errorf("upload exceeds declared size %d", open.Size)
		}
	}
	if open.Size > 0 && written != open.Size {
		return written, fmt.Errorf("upload is %d bytes, declared %d", written, open.Size)
	}
	if err := tmp.Sync(); err != nil {
		return written, err
	}
	if err := tmp.Chmod(fileMode(open.Mode, 0o644)); err != nil {
		return written, err
	}
	if err := tmp.Close(); err != nil {
		return written, err
	}
	if err := os.Rename(tmpName, open.Path); err != nil {
		return written, err
	}
	return written, nil
}

func listDir(path string) ([]agenttypes.FileEntry, error) {
	des, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	out := make([]agenttypes.FileEntry, 0, len(des))
	for _, de := range des {
		info, err := de.Info()
		if err != nil {
			continue
		}
		out = append(out, toEntry(filepath.Join(path, de.Name()), info))
	}
	return out, nil
}

func statEntry(path string) (*agenttypes.FileEntry, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	e := toEntry(path, info)
	return &e, nil
}

func toEntry(full string, info os.FileInfo) agenttypes.FileEntry {
	e := agenttypes.FileEntry{
		Name:    info.Name(),
		Size:    info.Size(),
		Mode:    uint32(info.Mode().Perm()),
		IsDir:   info.IsDir(),
		ModUnix: info.ModTime().Unix(),
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if target, err := os.Readlink(full); err == nil {
			e.Symlink = target
		}
	}
	return e
}

func fileMode(mode uint32, fallback os.FileMode) os.FileMode {
	if mode == 0 {
		return fallback
	}
	return os.FileMode(mode).Perm()
}

func respond(mu *sync.Mutex, w io.Writer, res agenttypes.FileResult) {
	mu.Lock()
	defer mu.Unlock()
	_ = agenttypes.WriteJSONMessage(w, agenttypes.MsgResult, res)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
