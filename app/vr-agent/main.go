// Command vr-agent is the vraxel host agent. It runs as a root systemd
// service on a managed machine, dials out to the server, and keeps a
// control channel open.
//
// First-cut scope: registration, heartbeat and the control channel. Job
// execution, data channels and metric scraping arrive in later slices,
// once the server side exposes them.
//
// Outbound only. The agent listens on nothing, which is the whole point:
// the platform can manage a host it cannot reach.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"vraxel.io/vraxel/lib/agent/client"
	"vraxel.io/vraxel/lib/agent/hostinfo"
	"vraxel.io/vraxel/lib/agent/transport"
	agenttypes "vraxel.io/vraxel/lib/agent/types"
	"vraxel.io/vraxel/lib/buildinfo"
)

// defaultStatePath is where the agent persists its credential. Under
// /etc because it is host configuration that must survive a package
// upgrade, and because /var/lib is not guaranteed mounted at the point
// systemd starts the unit on some minimal images.
const defaultStatePath = "/etc/vr-agent/agent.json"

func main() {
	var (
		serverURL  = flag.String("server", "", "server base URL, e.g. https://vraxel.example.com")
		joinToken  = flag.String("token", "", "one-time join token; required on first run only")
		statePath  = flag.String("state", defaultStatePath, "path to the agent state file")
		reRegister = flag.Bool("re-register", false,
			"force a fresh registration even if a state file exists (rebinds the same host row)")
		registerOnly = flag.Bool("register-only", false,
			"register, persist the state file, and exit without opening the control channel")
		caFile = flag.String("ca-file", "",
			"PEM bundle of the CA that signed the server's certificate; empty uses the system trust store")
	)
	flag.Parse()
	// -version is registered by lib/buildinfo's package init, shared with
	// vraxel-server; declaring our own would panic with "flag redefined".
	buildinfo.Init()

	version := buildinfo.ShortVersion()

	logger := stdLogger{log.New(os.Stderr, "", log.LstdFlags|log.Lmsgprefix)}

	// One trust store for every outbound path. Loaded before anything
	// dials: with a private CA, registration is the first thing that
	// fails without it.
	tlsCfg, err := transport.LoadCA(*caFile)
	if err != nil {
		logger.Warnf("vr-agent: %v", err)
		os.Exit(1)
	}
	httpClient := transport.HTTPClient(tlsCfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := ensureRegistered(ctx, httpClient, *statePath, *serverURL, *joinToken, *reRegister, version, logger)
	if err != nil {
		logger.Warnf("vr-agent: %v", err)
		os.Exit(1)
	}

	if *registerOnly {
		logger.Infof("vr-agent: registration complete (host %d); exiting as requested", st.HostID)
		return
	}

	ch := &client.Channel{
		ServerURL:  st.ServerURL,
		AgentToken: func() string { return st.AgentToken },
		Version:    version,
		Log:        logger,
		HTTPClient: httpClient,
		OnFrame:    onFrame(logger),
	}

	logger.Infof("vr-agent %s: agent %s, host %d, server %s", version, st.AgentID, st.HostID, st.ServerURL)
	ch.Run(ctx)
	logger.Infof("vr-agent: shutting down")
}

// onFrame handles server-pushed control frames. With only the control
// slice live, no subsystem consumes a frame yet; logging each arrival is
// what proves the channel is two-way. Later slices (jobs, data channel,
// probes) route frames here to their owners.
func onFrame(logger stdLogger) func(context.Context, agenttypes.Frame, client.SendFunc) {
	return func(_ context.Context, f agenttypes.Frame, _ client.SendFunc) {
		logger.Infof("vr-agent: control frame %s", f.Type)
	}
}

// ensureRegistered returns usable state, registering first if needed.
//
// Re-running the install script is the common case, so an existing state
// file short-circuits: the machine is already onboarded and its token is
// still valid. -re-register exists for recovery (revoked token, moved
// deployment) and is safe because the server keys registration on the
// machine id, so it rebinds the same host row rather than creating a
// second one.
func ensureRegistered(ctx context.Context, httpClient *http.Client, statePath, serverURL, joinToken string, force bool, version string, logger stdLogger) (*state, error) {
	st, err := loadState(statePath)
	if err != nil {
		return nil, err
	}
	if st != nil && !force {
		// -server on a registered agent overrides the stored URL, so an
		// operator can move a deployment behind a new address without
		// re-onboarding every host.
		if serverURL != "" && serverURL != st.ServerURL {
			st.ServerURL = serverURL
			if err := saveState(statePath, st, logger); err != nil {
				return nil, err
			}
		}
		return st, nil
	}

	if serverURL == "" {
		return nil, fmt.Errorf("-server is required for registration")
	}
	if joinToken == "" {
		return nil, fmt.Errorf("-token is required for registration (no state file at %s)", statePath)
	}

	facts := hostinfo.Collect()
	if facts.MachineID == "" {
		return nil, fmt.Errorf("cannot determine a stable machine id (no /etc/machine-id and no hostname)")
	}
	if strings.HasPrefix(facts.MachineID, "hostname:") {
		logger.Warnf("vr-agent: no /etc/machine-id on this host; falling back to hostname identity. " +
			"Two hosts sharing a hostname would then contend for one host row -- run systemd-machine-id-setup.")
	}
	if facts.DefaultRouteIP == "" {
		logger.Warnf("vr-agent: no default-route IPv4 found; the host will register without a reported IP")
	}

	resp, err := client.Register(ctx, httpClient, serverURL, joinToken, agenttypes.RegisterRequest{
		MachineID:      facts.MachineID,
		Hostname:       facts.Hostname,
		OS:             facts.OS,
		Arch:           facts.Arch,
		CPUCores:       facts.CPUCores,
		MemoryMB:       facts.MemoryMB,
		DiskGB:         facts.DiskGB,
		DefaultRouteIP: facts.DefaultRouteIP,
		AgentVersion:   version,
	})
	if err != nil {
		return nil, err
	}
	st = &state{
		ServerURL:  serverURL,
		AgentID:    resp.AgentID,
		HostID:     resp.HostID,
		AgentToken: resp.AgentToken,
	}
	if err := saveState(statePath, st, logger); err != nil {
		return nil, err
	}
	logger.Infof("vr-agent: registered as host %d (agent %s)", resp.HostID, resp.AgentID)
	return st, nil
}

// stdLogger adapts the standard library logger onto client.Logger.
type stdLogger struct{ l *log.Logger }

func (s stdLogger) Infof(format string, args ...any) { s.l.Printf(format, args...) }
func (s stdLogger) Warnf(format string, args ...any) { s.l.Printf("WARN "+format, args...) }
