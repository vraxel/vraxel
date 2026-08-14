package agentgw

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"vraxel.io/vraxel/lib/logger"
)

// installScript is the onboarding script, embedded so it ships inside the
// server binary and cannot drift from the protocol it targets. The
// canonical copy lives beside this handler for the same reason.
//
//go:embed install-agent.sh
var installScript []byte

// agentBinaryDirEnv overrides where cross-compiled agent binaries are
// served from. Default matches the Makefile's output directory.
const agentBinaryDirEnv = "VRAXEL_AGENT_BINARY_DIR"

const defaultAgentBinaryDir = "bin"

// agentBinaryDir resolves where to serve binaries from. The env override
// exists because the server's working directory is not the repo root in
// any real deployment.
func agentBinaryDir() string {
	if dir := os.Getenv(agentBinaryDirEnv); dir != "" {
		return dir
	}
	return defaultAgentBinaryDir
}

// agentBinaryTargets is the allowlist of cross-compile targets. It also
// serves as path-traversal defence: os / arch never reach the filesystem
// unvalidated.
var agentBinaryTargets = map[string]map[string]bool{
	"linux": {"amd64": true, "arm64": true},
}

// handleInstallScript serves the onboarding script, with the current
// binaries' checksums substituted in.
//
// Unauthenticated for the same reason as the binary: it runs before any
// credential exists, and it carries no secret (the join token is passed
// in by the operator, never baked in).
func (h *protocolHandler) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline; filename=install-agent.sh")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(h.renderInstallScript())
	}
}

// renderInstallScript fills the script's checksum placeholders with the
// digests of the binaries this server actually serves.
//
// Inlining them, rather than adding a digest endpoint, keeps the trust
// chain in one hop: whoever the operator trusted enough to pipe into a
// root shell also states what the binary must hash to. A missing binary
// leaves its placeholder in place, and the script says so out loud rather
// than silently skipping verification.
func (h *protocolHandler) renderInstallScript() []byte {
	out := installScript
	for goos, arches := range agentBinaryTargets {
		for goarch := range arches {
			sum, err := h.binarySHA256(goos, goarch)
			if err != nil {
				continue
			}
			out = bytes.ReplaceAll(out,
				[]byte(fmt.Sprintf("__VRAXEL_AGENT_SHA256_%s__", goarch)), []byte(sum))
		}
	}
	return out
}

// binarySHA256 returns the hex digest of one agent binary, cached per
// (size, mtime) so a fleet-wide install storm hashes a ~20MB file once
// instead of once per host.
func (h *protocolHandler) binarySHA256(goos, goarch string) (string, error) {
	path := filepath.Join(h.binaryDir, fmt.Sprintf("vr-agent-%s-%s", goos, goarch))
	stat, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("%s|%d|%d", path, stat.Size(), stat.ModTime().UnixNano())

	h.shaMu.Lock()
	cached, ok := h.shaCache[key]
	h.shaMu.Unlock()
	if ok {
		return cached, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "", err
	}
	digest := hex.EncodeToString(sum.Sum(nil))

	h.shaMu.Lock()
	if h.shaCache == nil {
		h.shaCache = map[string]string{}
	}
	h.shaCache[key] = digest
	h.shaMu.Unlock()
	return digest, nil
}

// handleBinary serves the cross-compiled agent binary. Unauthenticated by
// design (): install-agent.sh runs before any credential
// Unauthenticated by design: install-agent.sh runs before any credential
// exists on the machine, and the binary is not a secret. The join token
// -- which is one -- is supplied separately by the operator.
func (h *protocolHandler) handleBinary(w http.ResponseWriter, r *http.Request, rest string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	goos, goarch, found := strings.Cut(rest, "/")
	if !found || strings.Contains(goarch, "/") {
		http.NotFound(w, r)
		return
	}
	if !agentBinaryTargets[goos][goarch] {
		http.Error(w, "unsupported target "+goos+"/"+goarch, http.StatusNotFound)
		return
	}
	name := fmt.Sprintf("vr-agent-%s-%s", goos, goarch)
	path := filepath.Join(h.binaryDir, name)
	f, err := os.Open(path)
	if err != nil {
		logger.Warnf("agentgw: agent binary %s unavailable: %v (build it with `make vr-agent-binaries`, or point %s at its directory)", path, err, agentBinaryDirEnv)
		http.Error(w, "agent binary for "+goos+"/"+goarch+" is not available on this server", http.StatusNotFound)
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "stat agent binary", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	http.ServeContent(w, r, name, stat.ModTime(), f)
}
