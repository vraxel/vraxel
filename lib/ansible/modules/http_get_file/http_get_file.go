package http_get_file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
)

// ModuleHTTPGetFile downloads a file from HTTP/HTTPS to a local path.
//
// Args:
//
//	url:      download URL (required)
//	dest:     local save path (required)
//	username: basic auth username (optional)
//	password: basic auth password (optional)
//	token:    bearer token (optional)
//	timeout:  request timeout duration string (optional, default "30s")
//	headers:  custom headers map[string]any (optional)
//	sha256:   expected hex digest (optional; verified before the file is
//	          renamed into place, so a corrupted or tampered download never
//	          becomes the destination file)
func ModuleHTTPGetFile(ctx context.Context, opts internal.ExecOptions) (string, string, error) {
	urlStr := internal.StringArg(opts.Args, "url")
	dest := internal.StringArg(opts.Args, "dest")
	if urlStr == "" || dest == "" {
		return "", "", fmt.Errorf("http_get_file: url and dest are required")
	}

	urlStr, err := httpGetFileResolveURL(urlStr)
	if err != nil {
		return "", "", err
	}

	// Parse timeout.
	timeout := 30 * time.Second
	if t := internal.StringArg(opts.Args, "timeout"); t != "" {
		if d, err := time.ParseDuration(t); err == nil {
			timeout = d
		}
	}

	// Create HTTP client.
	client := &http.Client{Timeout: timeout}

	req, err := httpGetFileBuildRequest(ctx, urlStr, opts.Args)
	if err != nil {
		return "", "", err
	}

	// Execute request.
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("http_get_file: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("http_get_file: server returned %d: %s", resp.StatusCode, string(body))
	}

	if err := httpGetFileSaveBody(dest, resp.Body, internal.StringArg(opts.Args, "sha256")); err != nil {
		return "", "", err
	}

	return fmt.Sprintf("downloaded %s -> %s", urlStr, dest), "", nil
}

// httpGetFileResolveURL validates the URL scheme: a missing scheme
// defaults to http (returning the rewritten URL string); http/https pass
// through unchanged; anything else is rejected.
func httpGetFileResolveURL(urlStr string) (string, error) {
	// Validate URL scheme.
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("http_get_file: invalid url: %w", err)
	}
	if parsedURL.Scheme == "" {
		parsedURL.Scheme = "http"
		urlStr = parsedURL.String()
	} else if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", fmt.Errorf("http_get_file: unsupported url scheme %q, only http and https are supported", parsedURL.Scheme)
	}
	return urlStr, nil
}

// httpGetFileBuildRequest constructs the GET request and applies basic
// auth / bearer token / custom headers from args.
func httpGetFileBuildRequest(ctx context.Context, urlStr string, args map[string]any) (*http.Request, error) {
	// Build request.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("http_get_file: create request: %w", err)
	}

	// Add basic auth.
	username := internal.StringArg(args, "username")
	password := internal.StringArg(args, "password")
	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}

	// Add bearer token.
	if token := internal.StringArg(args, "token"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Add custom headers.
	if headers, ok := args["headers"].(map[string]any); ok {
		for k, v := range headers {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
	}
	return req, nil
}

// httpGetFileSaveBody downloads body to a temp file under dest's parent
// directory then atomically renames it into place. The temp file is
// removed on any error path.
//
// When wantSHA is set the digest is computed during the copy and checked
// before the rename, so a truncated or tampered download is discarded
// rather than installed. Artifact distribution depends on this: the agent
// pulls artifacts by URL (design §5.4), and a pull with no integrity check
// would make the signed URL the only thing standing between a corrupted
// object store and a host's filesystem.
func httpGetFileSaveBody(dest string, body io.Reader, wantSHA string) error {
	// Ensure parent directory exists.
	parentDir := filepath.Dir(dest)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("http_get_file: create directory %s: %w", parentDir, err)
	}

	// Download to temp file then atomic rename.
	tmpFile, err := os.CreateTemp(parentDir, ".http_get_file_*.tmp")
	if err != nil {
		return fmt.Errorf("http_get_file: create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		// Clean up temp file on any error path.
		_ = os.Remove(tmpPath)
	}()

	h := sha256.New()
	var sink io.Writer = tmpFile
	if wantSHA != "" {
		sink = io.MultiWriter(tmpFile, h)
	}
	if _, err := io.Copy(sink, body); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("http_get_file: download to temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("http_get_file: close temp file: %w", err)
	}

	if wantSHA != "" {
		if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, wantSHA) {
			return fmt.Errorf("http_get_file: digest %s does not match the expected %s", got, wantSHA)
		}
	}

	if err := os.Rename(tmpPath, dest); err != nil {
		return fmt.Errorf("http_get_file: rename temp to dest: %w", err)
	}
	return nil
}
