package httpserver

import (
	"context"
	"crypto/tls"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/klauspost/compress/gzhttp"
	"golang.org/x/net/http2"
	"vraxel.io/vraxel/lib/appmetrics"
	"vraxel.io/vraxel/lib/fastrand"
	"vraxel.io/vraxel/lib/fasttime"
	"vraxel.io/vraxel/lib/lflag"
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/utils/stringsutil"
)

var (
	tlsEnable   = lflag.NewArrayBool("tls", "Whether to enable TLS for the server, -tlsCertFile and -tlsKeyFile must be set if -tls is set")
	tlsCertFile = lflag.NewArrayString("tlsCertFile", "Path to file with TLS certificate for the corresponding -httpListenAddr if -tls is set."+
		"Prefer ECDSA certs instead of RSA certs as RSA certs are slower")
	tlsKeyFile = lflag.NewArrayString("tlsKeyFile", "Path to file with TLS key for the corresponding -httpListenAddr if -tls is set")

	disableHTTP2 = flag.Bool("http.disableHTTP2", false, "Whether to disable HTTP/2 for the server")
	disableCORS  = flag.Bool("http.disableCORS", false, "Disable CORS for all origins (*)")

	pathPrefix = flag.String("http.pathPrefix", "", "An optional prefix to add to all the paths handled by http server. For example, if '-http.pathPrefix=/foo/bar' is set, "+
		"then all the http requests will be handled on '/foo/bar/*' paths. This may be useful for proxied requests")

	httpAuthUsername = flag.String("httpAuth.username", "", "Username for HTTP server's Basic Auth. The authentication is disabled if empty. See also -httpAuth.password")
	httpAuthPassword = lflag.NewPassword("httpAuth.password", "Password for HTTP server's Basic Auth. The authentication is disabled if -httpAuth.username is empty")
	metricsAuthKey   = lflag.NewPassword("metricsAuthKey", "Auth key for /metrics endpoint. It must be passed via authKey query arg. It overrides -httpAuth.*")
	flagsAuthKey     = lflag.NewPassword("flagsAuthKey", "Auth key for /flags endpoint. It must be passed via authKey query arg. It overrides -httpAuth.*")
	pprofAuthKey     = lflag.NewPassword("pprofAuthKey", "Auth key for /debug/pprof/* endpoints. It must be passed via authKey query arg. It overrides -httpAuth.*")

	maxGracefulShutdownDuration = flag.Duration("http.maxGracefulShutdownDuration", 7*time.Second, `The maximum duration for a graceful shutdown of the HTTP server. A highly loaded server may require increased value for a graceful shutdown`)
	shutdownDelay               = flag.Duration("http.shutdownDelay", 0, `Optional delay before http server shutdown. During this delay, the server returns non-OK responses from /health page, so load balancers can route new requests to other servers`)
	idleConnTimeout             = flag.Duration("http.idleConnTimeout", time.Minute, "Timeout for incoming idle http connections")
	connTimeout                 = flag.Duration("http.connTimeout", 2*time.Minute, "Incoming connections to -httpListenAddr are closed after the configured timeout. "+
		"This may help evenly spreading load among a cluster of services behind TCP-level load balancer. Zero value disables closing of incoming connections")

	headerHSTS         = flag.String("http.header.hsts", "max-age=31536000; includeSubDomains", "Value for 'Strict-Transport-Security' header, recommended: 'max-age=31536000; includeSubDomains'")
	headerFrameOptions = flag.String("http.header.frameOptions", "SAMEORIGIN", "Value for 'X-Frame-Options' header")
	headerCSP          = flag.String("http.header.csp", "default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:", `Value for 'Content-Security-Policy' header`)
)

var (
	servers     = make(map[string]*server)
	serversLock sync.Mutex
)

var (
	requestsTotal          = metrics.NewCounter(`vraxel_http_requests_all_total`)
	metricsRequests        = metrics.NewCounter(`vraxel_http_requests_total{path="/metrics"}`)
	metricsHandlerDuration = metrics.NewHistogram(`vraxel_http_request_duration_seconds{path="/metrics"}`)
	connTimeoutClosedConns = metrics.NewCounter(`vraxel_http_conn_timeout_closed_conns_total`)

	pprofRequests        = metrics.NewCounter(`vraxel_http_requests_total{path="/debug/pprof/"}`)
	pprofCmdlineRequests = metrics.NewCounter(`vraxel_http_requests_total{path="/debug/pprof/cmdline"}`)
	pprofProfileRequests = metrics.NewCounter(`vraxel_http_requests_total{path="/debug/pprof/profile"}`)
	pprofSymbolRequests  = metrics.NewCounter(`vraxel_http_requests_total{path="/debug/pprof/symbol"}`)
	pprofTraceRequests   = metrics.NewCounter(`vraxel_http_requests_total{path="/debug/pprof/trace"}`)
	pprofMutexRequests   = metrics.NewCounter(`vraxel_http_requests_total{path="/debug/pprof/mutex"}`)
	pprofDefaultRequests = metrics.NewCounter(`vraxel_http_requests_total{path="/debug/pprof/default"}`)

	faviconRequests = metrics.NewCounter(`vraxel_http_requests_total{path="*/favicon.ico"}`)

	authBasicRequestErrors   = metrics.NewCounter(`vraxel_http_request_errors_total{path="*", reason="wrong_basic_auth"}`)
	authKeyRequestErrors     = metrics.NewCounter(`vraxel_http_request_errors_total{path="*", reason="wrong_auth_key"}`)
	unsupportedRequestErrors = metrics.NewCounter(`vraxel_http_request_errors_total{path="*", reason="unsupported"}`)
)

var hostname = func() string {
	h, err := os.Hostname()
	if err != nil {
		// Cannot use logger.Errorf, since it isn't initialized yet.
		// So use log.Printf instead.
		log.Printf("ERROR: cannot determine hostname: %s", err)
		return "unknown"
	}
	return h
}()

var gzipHandlerWrapper = func() func(http.Handler) http.HandlerFunc {
	hw, err := gzhttp.NewWrapper(
		gzhttp.CompressionLevel(1),
		// Prefer gzip over zstd compression if the client supports both methods
		gzhttp.PreferZstd(false),
	)
	if err != nil {
		panic(fmt.Errorf("BUG: cannot initialize gzip http wrapper: %w", err))
	}
	return hw
}()

type server struct {
	s                     *http.Server
	shutdownDelayDeadline atomic.Int64
}

// RequestHandler must serve the given request r and write response to w
//
// RequestHandler must return true if the request has been served (successfully or not)
//
// RequestHandler must return false if it cannot serve the given request
type RequestHandler func(w http.ResponseWriter, r *http.Request) bool

type ServerOptions struct {
	// UseProxyProtocol if is set to true for the corresponding addr, then the incoming connections are accepted via proxy protocol
	UseProxyProtocol *lflag.ArrayBool
	// DisableBuiltinRoutes whether not to serve built-in routes for the given server, such as:
	// /health, /debug/pprof and few others
	// In addition basic auth check and authKey checks will be disabled for the given addr
	//
	// Mostly required by http proxy servers, which performs own authorization and requests routing
	DisableBuiltinRoutes bool
}

// Serve starts an http server on the given addresses with the given optional request handler
func Serve(addrs []string, rh RequestHandler, opts ServerOptions) {
	if rh == nil {
		rh = func(_ http.ResponseWriter, _ *http.Request) bool { return false }
	}
	for idx, addr := range addrs {
		if addr == "" {
			continue
		}
		logger.Infof("starting http server on %s", addr)
		go serve(addr, rh, idx, opts)
	}
}

func serve(addr string, rh RequestHandler, idx int, opts ServerOptions) {
	scheme := "http"
	if tlsEnable.GetOptionalArg(idx) {
		scheme = "https"
	}
	useProxyProto := false
	if opts.UseProxyProtocol != nil {
		useProxyProto = opts.UseProxyProtocol.GetOptionalArg(idx)
	}

	var tlsConfig *tls.Config
	if tlsEnable.GetOptionalArg(idx) {
		certFile := tlsCertFile.GetOptionalArg(idx)
		keyFile := tlsKeyFile.GetOptionalArg(idx)
		tc, err := GetServerTLSConfig(certFile, keyFile)
		if err != nil {
			logger.Fatalf("cannot load TLS cert from -tlsCertFile=%q, -tlsKeyFile=%q: %s", certFile, keyFile, err)
		}
		// Can't use SSLv3 because of POODLE and BEAST
		// Can't use TLSv1.0 because of POODLE and BEAST using CBC cipher
		// Can't use TLSv1.1 because of RC4 cipher usage
		tc.MinVersion = tls.VersionTLS12
		if *disableHTTP2 {
			logger.Infof("forcing use of http/1.1 only")
			tc.NextProtos = []string{"http/1.1"}
		} else {
			tc.NextProtos = []string{"h2", "http/1.1"}
		}
		tlsConfig = tc
	}

	// create a TCP listener
	ln, err := NewTCPListener(scheme, addr, useProxyProto, tlsConfig)
	if err != nil {
		logger.Fatalf("cannot start http server on %s: %v", addr, err)
	}
	logger.Infof("started http server on %s://%s/", scheme, ln.Addr())
	if !opts.DisableBuiltinRoutes {
		logger.Infof("pprof handlers are exposed at %s://%s/debug/pprof/", scheme, ln.Addr())
	}

	serveWithListener(addr, ln, rh, opts.DisableBuiltinRoutes)
}

func serveWithListener(addr string, ln net.Listener, rh RequestHandler, disableBuiltinRoutes bool) {
	var s server

	rhw := rh
	if !disableBuiltinRoutes {
		rhw = func(w http.ResponseWriter, r *http.Request) bool {
			return builtinRoutesHandler(&s, r, w, rh)
		}
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerWrapper(w, r, rhw)
	})

	h = gzipHandlerWrapper(h)

	s.s = &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       90 * time.Second, // matches http.DefaultTransport keep-alive timeout
		ErrorLog:          logger.StdErrorLogger(),
	}

	if *disableHTTP2 {
		s.s.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	} else {
		// HTTP/2 per-stream upload window. The previous 256 KiB value
		// stalled large POSTs: a 4 MiB issue-create body (a few inline
		// base64 image attachments, well under the 64 MiB issue body
		// limit declared by pkg/apis/dev/install.go) needed ~16 window
		// updates, and a 40-80 ms RTT made the client time out before
		// the body finished arriving (bug #85: "提交三张不同图片提交bug，
		// 出现超时"; second-attempt edit succeeded because attachments
		// dribbled in one by one).
		//
		// 1 MiB matches the x/net/http2 default; a single 4 MiB body
		// settles in ~4 RTTs and the worst-case per-connection buffer
		// (1 MiB * 100 streams) is 100 MiB — acceptable for a process
		// that already permits 64 MiB of inline attachments per issue.
		// MaxReadFrameSize is held at 256 KiB on purpose: it caps a
		// single HTTP/2 frame, only matters for keeping a slow-decoding
		// peer from blocking the connection on one huge frame, and has
		// no bearing on upload throughput once the window is large
		// enough.
		const uploadBufferPerStream = 1024 * 1024
		const maxReadFrameSize = 256 * 1024

		http2Options := &http2.Server{
			IdleTimeout:              90 * time.Second,
			MaxUploadBufferPerStream: uploadBufferPerStream,
			MaxReadFrameSize:         maxReadFrameSize,
			MaxConcurrentStreams:     100,
		}

		http2Options.MaxUploadBufferPerConnection = http2Options.MaxUploadBufferPerStream * int32(http2Options.MaxConcurrentStreams)
		// apply settings to the server
		if err := http2.ConfigureServer(s.s, http2Options); err != nil {
			logger.Panicf("cannot configure http/2 for http server on %s: %v", addr, err)
		}
	}

	if *connTimeout > 0 {
		s.s.ConnContext = func(ctx context.Context, _ net.Conn) context.Context {
			timeoutSec := connTimeout.Seconds()
			// Add a jitter for connection timeout in order to prevent Thundering herd problem
			// when all the connections are established at the same time.
			// See https://en.wikipedia.org/wiki/Thundering_herd_problem
			jitterSec := fastrand.Uint32n(uint32(timeoutSec / 10))
			return context.WithValue(ctx, connDeadlineTimeKey, new(fasttime.UnixTimestamp()+uint64(timeoutSec)+uint64(jitterSec)))
		}
	}

	//s.s.SetKeepAlivesEnabled(true)

	serversLock.Lock()
	servers[addr] = &s
	serversLock.Unlock()
	if err := s.s.Serve(ln); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			return
		}
		logger.Panicf("FATAL: cannot serve http server on %s: %v", addr, err)
	}
}

// Stop stops the http server on the given addrs, which has been started via Serve func
func Stop(addrs []string) error {
	var errGlobalLock sync.Mutex
	var errGlobal error

	var wg sync.WaitGroup
	for _, addr := range addrs {
		if addr == "" {
			continue
		}
		wg.Add(1)
		go func(addr string) {
			if err := stop(addr); err != nil {
				errGlobalLock.Lock()
				errGlobal = err
				errGlobalLock.Unlock()
			}
			wg.Done()
		}(addr)
	}
	wg.Wait()

	return errGlobal
}

//go:embed favicon.ico
var faviconData []byte

func builtinRoutesHandler(s *server, r *http.Request, w http.ResponseWriter, rh RequestHandler) bool {
	h := w.Header()
	path := r.URL.Path
	if strings.HasSuffix(path, "/favicon.ico") {
		w.Header().Set("Cache-Control", "max-age=3600")
		faviconRequests.Inc()
		_, _ = w.Write(faviconData)
		return true
	}

	switch r.URL.Path {
	case "/health":
		h.Set("Content-Type", "text/plain; charset=utf-8")
		deadline := s.shutdownDelayDeadline.Load()
		if deadline <= 0 {
			_, _ = w.Write([]byte("OK"))
			return true
		}
		// Return non-OK response during grace period before shutting down the server
		// Load balancers must notify these responses and re-route new requests to other servers
		d := max(time.Until(time.Unix(0, deadline)), 0)
		errMsg := fmt.Sprintf("The server is in delayed shutdown mode, which will end in %.3fs", d.Seconds())
		http.Error(w, errMsg, http.StatusServiceUnavailable)
		return true
	case "/ping":
		status := http.StatusNoContent
		if verbose := r.FormValue("verbose"); verbose == "true" {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		return true
	case "/metrics":
		metricsRequests.Inc()
		if !CheckAuthFlag(w, r, metricsAuthKey) {
			return true
		}
		startTime := time.Now()
		h.Set("Content-Type", "text/plain; charset=utf-8")
		appmetrics.WritePrometheusMetrics(w)
		metricsHandlerDuration.UpdateDuration(startTime)
		return true
	case "/flags":
		if !CheckAuthFlag(w, r, flagsAuthKey) {
			return true
		}
		h.Set("Content-Type", "text/plain; charset=utf-8")
		lflag.WriteFlags(w)
		return true
	case "/-/healthy":
		// This is needed for Prometheus compatibility
		_, _ = fmt.Fprintf(w, "Vraxel is Healthy.\n")
		return true
	case "/-/ready":
		// This is needed for Prometheus compatibility
		_, _ = fmt.Fprintf(w, "Vraxel is Ready.\n")
		return true
	case "/robots.txt":
		// This prevents search engines from indexing contents
		_, _ = fmt.Fprintf(w, "User-agent: *\nDisallow: /\n")
		return true
	default:
		if served := builtinDefaultRouteHandler(r, w); served {
			return true
		}
	}
	return rh(w, r)
}

// builtinDefaultRouteHandler handles the default branch of builtinRoutesHandler:
// the /debug/pprof/* tree plus the basic-auth gate for unprotected paths. It
// returns true when the request has been fully served (so the caller must not
// fall through to the user handler) and false when the request should continue
// to rh.
func builtinDefaultRouteHandler(r *http.Request, w http.ResponseWriter) bool {
	if strings.HasPrefix(r.URL.Path, "/debug/pprof/") {
		pprofRequests.Inc()
		if !CheckAuthFlag(w, r, pprofAuthKey) {
			return true
		}
		pprofHandler(r.URL.Path[len("/debug/pprof/"):], w, r)
		return true
	}

	if !isProtectedByAuthFlag(r.URL.Path) && !CheckBasicAuth(w, r) {
		return true
	}
	return false
}

func stop(addr string) error {
	serversLock.Lock()
	s := servers[addr]
	delete(servers, addr)
	serversLock.Unlock()
	if s == nil {
		err := fmt.Errorf("BUG: there is no server at %q", addr)
		logger.Panicf("%s", err)
		return err
	}

	deadline := time.Now().Add(*shutdownDelay).UnixNano()
	s.shutdownDelayDeadline.Store(deadline)
	if *shutdownDelay > 0 {
		// Sleep for a while until load balancer in front of the server
		// notifies that "/health" endpoint returns non-OK responses
		logger.Infof("Waiting for %.3fs before shutdown of http server %q, so load balancers could re-route requests to other servers", shutdownDelay.Seconds(), addr)
		time.Sleep(*shutdownDelay)
		logger.Infof("Starting shutdown for http server %q", addr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *maxGracefulShutdownDuration)
	defer cancel()
	if err := s.s.Shutdown(ctx); err != nil {
		return fmt.Errorf("cannot gracefully shutdown http server at %q in %.3fs; "+
			"probably, `-http.maxGracefulShutdownDuration` command-line flag value must be increased; error: %s", addr, maxGracefulShutdownDuration.Seconds(), err)
	}
	return nil
}

func handlerWrapper(w http.ResponseWriter, r *http.Request, rh RequestHandler) {
	defer func() {
		if err := recover(); err != nil {
			buf := make([]byte, 1<<20)
			n := runtime.Stack(buf, false)
			_, _ = fmt.Fprintf(os.Stderr, "panic: %v\n\n%s", err, buf[:n])
			os.Exit(1)
		}
	}()

	h := w.Header()
	if *headerHSTS != "" {
		h.Add("Strict-Transport-Security", *headerHSTS)
	}
	if *headerFrameOptions != "" {
		h.Add("X-Frame-Options", *headerFrameOptions)
	}
	if *headerCSP != "" {
		h.Add("Content-Security-Policy", *headerCSP)
	}
	h.Add("X-Server-Hostname", hostname)
	requestsTotal.Inc()
	if whetherToCloseConn(r) {
		connTimeoutClosedConns.Inc()
		h.Set("Connection", "close")
	}

	path := r.URL.Path

	prefix := GetPathPrefix()
	if prefix != "" {
		// Trim -http.pathPrefix from path
		prefixNoTrailingSlash := strings.TrimSuffix(prefix, "/")
		if path == prefixNoTrailingSlash {
			// Redirect to url with / at the end
			// This is needed for proper handling of relative URLs in web browsers
			// Intentionally ignore query args, since it is expected that the requested url
			// is composed by a human, so it doesn't contain query args
			Redirect(w, prefix)
			return
		}
		if !strings.HasPrefix(path, prefix) {
			Errorf(w, r, "missing -http.pathPrefix=%q in the requested path %q", *pathPrefix, path)
			unsupportedRequestErrors.Inc()
			return
		}
		path = path[len(prefix)-1:]
		r.URL.Path = path
	}

	w = &responseWriterWithAbort{
		ResponseWriter: w,
	}
	if rh(w, r) {
		return
	}

	Errorf(w, r, "unsupported path requested: %q", r.URL.Path)
	unsupportedRequestErrors.Inc()
}

func isProtectedByAuthFlag(path string) bool {
	// These paths must explicitly call CheckAuthFlag()
	return strings.HasSuffix(path, "/config") || strings.HasSuffix(path, "/reload") ||
		strings.HasSuffix(path, "/resetRollupResultCache") || strings.HasSuffix(path, "/delSeries") || strings.HasSuffix(path, "/delete_series") ||
		strings.HasSuffix(path, "/force_merge") || strings.HasSuffix(path, "/force_flush") || strings.HasSuffix(path, "/snapshot") ||
		strings.HasPrefix(path, "/snapshot/") || strings.HasSuffix(path, "/admin/status/metric_names_stats/reset")
}

// EnableCORS enables https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS
// on the response.
func EnableCORS(w http.ResponseWriter, _ *http.Request) {
	if *disableCORS {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
}

var connDeadlineTimeKey = any("connDeadlineSecs")

func whetherToCloseConn(r *http.Request) bool {
	if *connTimeout <= 0 {
		return false
	}
	ctx := r.Context()
	v := ctx.Value(connDeadlineTimeKey)
	deadline, ok := v.(*uint64)
	return ok && fasttime.UnixTimestamp() > *deadline
}

// GetPathPrefix - returns http server path prefix.
func GetPathPrefix() string {
	prefix := *pathPrefix
	if prefix == "" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

// Redirect redirects to the given url.
func Redirect(w http.ResponseWriter, url string) {
	// Do not use http.Redirect, since it breaks relative redirects
	// if the http.Request.URL contains unexpected url
	w.Header().Set("Location", url)
	// Use http.StatusFound instead of http.StatusMovedPermanently
	// since browsers can cache incorrect redirects returned with StatusMovedPermanently
	// This may require browser cache cleaning after the incorrect redirect is fixed
	w.WriteHeader(http.StatusFound)
}

// Errorf writes formatted error message to w and to logger.
func Errorf(w http.ResponseWriter, r *http.Request, format string, args ...any) {
	errStr := fmt.Sprintf(format, args...)
	logHTTPError(r, errStr)

	// Extract statusCode from args
	statusCode := http.StatusBadRequest
	for _, arg := range args {
		if err, ok := arg.(error); ok {
			if esc, ok := errors.AsType[*ErrorWithStatusCode](err); ok {
				statusCode = esc.StatusCode
				break
			}
		}
	}

	if rwa, ok := w.(*responseWriterWithAbort); ok && rwa.sentHeaders {
		// HTTP status code has been already sent to client, so it cannot be sent again.
		// Just write errStr to the response and abort the client connection, so the client could notice the error

		// HTML-escape the errStr in order to protect from possible XSS, since the errStr may contain user input
		errStrEscaped := html.EscapeString(errStr)

		_, _ = fmt.Fprintf(w, "\n%s\n", errStrEscaped)
		rwa.abort()
		return
	}
	http.Error(w, errStr, statusCode)
}

// logHTTPError logs the errStr with the client remote address and the request URI obtained from r.
func logHTTPError(r *http.Request, errStr string) {
	remoteAddr := GetQuotedRemoteAddr(r)
	requestURI := GetRequestURI(r)
	errStr = fmt.Sprintf("remoteAddr: %s; requestURI: %s; %s", remoteAddr, requestURI, errStr)
	logger.WarnfSkipFrames(2, "%s", errStr)
}

// GetQuotedRemoteAddr returns quoted remote address.
func GetQuotedRemoteAddr(r *http.Request) string {
	remoteAddr := r.RemoteAddr
	if addr := r.Header.Get("X-Forwarded-For"); addr != "" {
		remoteAddr += ", X-Forwarded-For: " + addr
	}
	// quote remoteAddr and X-Forwarded-For, since they may contain untrusted input
	return stringsutil.JSONString(remoteAddr)
}

// GetRequestURI returns requestURI for r
func GetRequestURI(r *http.Request) string {
	requestURI := r.RequestURI
	if r.Method != http.MethodPost {
		return requestURI
	}
	_ = r.ParseForm()
	if len(r.PostForm) == 0 {
		return requestURI
	}
	// code copied from url.Query.Encode
	var queryArgs strings.Builder
	for k := range r.PostForm {
		vs := r.PostForm[k]
		// mask authKey as well-known secret
		if k == "authKey" {
			vs = []string{"secret"}
		}
		keyEscaped := url.QueryEscape(k)
		for _, v := range vs {
			if queryArgs.Len() > 0 {
				queryArgs.WriteByte('&')
			}
			queryArgs.WriteString(keyEscaped)
			queryArgs.WriteByte('=')
			queryArgs.WriteString(url.QueryEscape(v))
		}
	}
	delimiter := "?"
	if strings.Contains(requestURI, delimiter) {
		delimiter = "&"
	}
	return requestURI + delimiter + queryArgs.String()
}

// ErrorWithStatusCode is error with HTTP status code.
//
// The given StatusCode is sent to client when the error is passed to Errorf
type ErrorWithStatusCode struct {
	Err        error
	StatusCode int
}

// Unwrap returns e.Err
//
// This is used by standard errors package. See https://golang.org/pkg/errors
func (e *ErrorWithStatusCode) Unwrap() error {
	return e.Err
}

// Error implements error interface
func (e *ErrorWithStatusCode) Error() string {
	return e.Err.Error()
}

type responseWriterWithAbort struct {
	http.ResponseWriter

	sentHeaders bool
	aborted     bool
}

func (rwa *responseWriterWithAbort) Write(data []byte) (int, error) {
	if rwa.aborted {
		return 0, fmt.Errorf("response connection is aborted")
	}
	if !rwa.sentHeaders {
		rwa.sentHeaders = true
	}
	return rwa.ResponseWriter.Write(data)
}

func (rwa *responseWriterWithAbort) WriteHeader(statusCode int) {
	if rwa.aborted {
		logger.WarnfSkipFrames(1, "cannot write response headers with statusCode=%d, since the response connection has been aborted", statusCode)
		return
	}
	if rwa.sentHeaders {
		logger.WarnfSkipFrames(1, "cannot write response headers with statusCode=%d, since they were already sent", statusCode)
		return
	}
	rwa.ResponseWriter.WriteHeader(statusCode)
	rwa.sentHeaders = true
}

// Flush implements net/http.Flusher interface
func (rwa *responseWriterWithAbort) Flush() {
	if rwa.aborted {
		return
	}
	if !rwa.sentHeaders {
		rwa.sentHeaders = true
	}
	flusher, ok := rwa.ResponseWriter.(http.Flusher)
	if !ok {
		logger.Panicf("BUG: it is expected http.ResponseWriter (%T) supports http.Flusher interface", rwa.ResponseWriter)
	}
	flusher.Flush()
}

// Unwrap returns the original ResponseWriter wrapped by rwa.
//
// This is needed for the net/http.ResponseController - see https://pkg.go.dev/net/http#NewResponseController
func (rwa *responseWriterWithAbort) Unwrap() http.ResponseWriter {
	return rwa.ResponseWriter
}

// abort aborts the client connection associated with rwa
//
// The last http chunk in the response stream is intentionally written incorrectly,
// so the client, which reads the response, could notice this error
func (rwa *responseWriterWithAbort) abort() {
	if !rwa.sentHeaders {
		logger.Panicf("BUG: abort can be called only after http response headers are sent")
	}
	if rwa.aborted {
		// Nothing to do. The connection has been already aborted
		return
	}
	hj, ok := rwa.ResponseWriter.(http.Hijacker)
	if !ok {
		logger.Panicf("BUG: ResponseWriter must implement http.Hijacker interface")
	}
	conn, bw, err := hj.Hijack()
	if err != nil {
		logger.WarnfSkipFrames(2, "cannot hijack response connection: %s", err)
		return
	}

	// Just write an error message into the client connection as is without http chunked encoding.
	// This is needed in order to notify the client about the aborted connection.
	_, _ = bw.WriteString("\nthe connection has been aborted; see the last line in the response and/or in the server log for the reason\n")
	_ = bw.Flush()

	// Forcibly close the client connection in order to break http keep-alive at client side.
	_ = conn.Close()

	rwa.aborted = true
}

// CheckAuthFlag checks whether the given authKey is set and valid
//
// Falls back to checkBasicAuth if authKey is not set
func CheckAuthFlag(w http.ResponseWriter, r *http.Request, expectedKey *lflag.Password) bool {
	expectedValue := expectedKey.Get()
	if expectedValue == "" {
		return CheckBasicAuth(w, r)
	}
	if len(r.FormValue("authKey")) == 0 {
		authKeyRequestErrors.Inc()
		http.Error(w, fmt.Sprintf("Expected to receive non-empty authKey when -%s is set", expectedKey.Name()), http.StatusUnauthorized)
		return false
	}
	if r.FormValue("authKey") != expectedValue {
		authKeyRequestErrors.Inc()
		http.Error(w, fmt.Sprintf("The provided authKey doesn't match -%s", expectedKey.Name()), http.StatusUnauthorized)
		return false
	}
	return true
}

// CheckBasicAuth validates credentials provided in request if httpAuth.* flags are set
// returns true if credentials are valid or httpAuth.* flags are not set
func CheckBasicAuth(w http.ResponseWriter, r *http.Request) bool {
	if len(*httpAuthUsername) == 0 {
		// HTTP Basic Auth is disabled.
		return true
	}
	username, password, ok := r.BasicAuth()
	if ok {
		if username == *httpAuthUsername && password == httpAuthPassword.Get() {
			return true
		}
		authBasicRequestErrors.Inc()
	}

	w.Header().Set("WWW-Authenticate", `Basic realm="Vraxel"`)
	http.Error(w, "", http.StatusUnauthorized)
	return false
}

func pprofHandler(profileName string, w http.ResponseWriter, r *http.Request) {
	// This switch has been stolen from init func at https://golang.org/src/net/http/pprof/pprof.go
	switch profileName {
	case "cmdline":
		pprofCmdlineRequests.Inc()
		pprof.Cmdline(w, r)
	case "profile":
		pprofProfileRequests.Inc()
		pprof.Profile(w, r)
	case "symbol":
		pprofSymbolRequests.Inc()
		pprof.Symbol(w, r)
	case "trace":
		pprofTraceRequests.Inc()
		pprof.Trace(w, r)
	case "mutex":
		pprofMutexRequests.Inc()
		seconds, _ := strconv.Atoi(r.FormValue("seconds"))
		if seconds <= 0 {
			seconds = 10
		}
		prev := runtime.SetMutexProfileFraction(10)
		time.Sleep(time.Duration(seconds) * time.Second)
		pprof.Index(w, r)
		runtime.SetMutexProfileFraction(prev)
	default:
		pprofDefaultRequests.Inc()
		pprof.Index(w, r)
	}
}
