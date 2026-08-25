// Command vr-agent is the vraxel host agent. It runs as a root systemd
// service on a managed machine, dials out to the server, and keeps a
// control channel open.
//
// Scope so far: registration, heartbeat, the control channel, and the
// data channel that carries interactive streams. Job execution and metric
// scraping arrive in later slices, once the server side exposes them.
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
	"sync/atomic"
	"syscall"

	"vraxel.io/vraxel/lib/agent/client"
	"vraxel.io/vraxel/lib/agent/datachan"
	"vraxel.io/vraxel/lib/agent/hostinfo"
	"vraxel.io/vraxel/lib/agent/nodemetrics"
	"vraxel.io/vraxel/lib/agent/scrape"
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
		// No host id here: printInstallSummary has just written it, and
		// this line lands one line above it.
		logger.Infof("vr-agent: registration complete; exiting as requested")
		return
	}

	a := &agent{log: logger}
	// The collector starts before anything can ask it a question, because
	// its first useful answer is two samples away and the clock on that
	// starts here.
	metrics := nodemetrics.New(nodemetrics.Config{Log: logger})
	go metrics.Run(ctx)

	a.data = datachan.New(datachan.Config{
		ServerURL: st.ServerURL,
		Token:     a.sessionToken,
		Metrics:   metrics,
		// An empty allowlist, which means any LOOPBACK port -- the
		// loopback restriction itself is hard-coded in the guard and is
		// not configurable. The operator-facing allowlist narrows it
		// further and belongs to the slice that first opens a tcp stream:
		// shipping the flag now would put a switch in --help that
		// install-agent.sh cannot set, so the only way to use it would be
		// to hand-edit a systemd unit the next install overwrites.
		Guard: datachan.NewGuard(nil),
		// Same trust store as every other outbound path; the data channel
		// is a plain WSS handshake and has no reason to trust differently.
		HTTPClient: httpClient,
		Log:        logger,
	})
	// The root ctx, not the control-channel session ctx: a control-channel
	// blip must not kill a terminal somebody is typing into. The data
	// channel has its own reconnect, and it parks itself when idle.
	go a.data.Run(ctx)

	// The scraper is inert until the server's scrape-targets answer says
	// otherwise (a push URL, targets, the node-metrics switch), so
	// starting it unconditionally costs one 60s polling loop and nothing
	// else on the lite tier.
	a.scraper = scrape.New(scrape.Config{
		ServerURL: st.ServerURL,
		Token:     a.sessionToken,
		TLS:       tlsCfg,
		Self:      metrics.Exposition,
		Log:       logger,
	})
	go a.scraper.Run(ctx)

	ch := &client.Channel{
		// Re-read on every connect rather than captured once: resetting
		// /etc/machine-id is what an operator does to a cloned host, and
		// the server only learns it happened if the next hello says so.
		Fingerprint:    func() agenttypes.MachineFingerprint { return hostinfo.Collect().Fingerprint() },
		Facts:          hostinfo.Facts,
		Processes:      hostinfo.Processes,
		Accounts:       hostinfo.Accounts,
		ServerURL:      st.ServerURL,
		AgentToken:     func() string { return st.AgentToken },
		Version:        version,
		Log:            logger,
		HTTPClient:     httpClient,
		OnFrame:        a.onFrame,
		MetricsSummary: metrics.Summary,
	}

	logger.Infof("vr-agent %s: agent %s, host %d, server %s", version, st.AgentID, st.HostID, st.ServerURL)
	ch.Run(ctx)
	logger.Infof("vr-agent: shutting down")
}

// agent owns the state the control channel's frames feed.
type agent struct {
	log     stdLogger
	data    *datachan.Channel
	scraper *scrape.Scraper

	// token is the short-lived credential the server pushes over the
	// control channel and renews for as long as it lives. Stored
	// atomically because the frame loop writes it while the data
	// channel's dialer reads it, on its own goroutine, per dial.
	token atomic.Pointer[string]
}

// sessionToken returns the current credential, or "" before the first one
// arrives. Empty is a real state, not an error: the data channel refuses
// to dial without one and retries, which is what a reconnect looks like
// from the inside.
func (a *agent) sessionToken() string {
	if p := a.token.Load(); p != nil {
		return *p
	}
	return ""
}

// onFrame routes server-pushed control frames to their owners.
func (a *agent) onFrame(_ context.Context, f agenttypes.Frame, _ client.SendFunc) {
	switch f.Type {
	case agenttypes.FrameTypeSessionToken:
		tok := f.Token
		// Only the first one is worth a line. The server renews on a timer
		// for the life of the channel, and logging every renewal would bury
		// the events an operator is actually reading this for.
		if a.token.Swap(&tok) == nil {
			a.log.Infof("vr-agent: session token received; the data channel can dial")
		}
	case agenttypes.FrameTypeConfigReload:
		// The server changed something the scraper polls for (targets,
		// the push switch); skip the 60s wait.
		a.scraper.Refresh()
	case agenttypes.FrameTypeChannelOpen:
		// Carries no parameters: it only asks for the channel to be up.
		// Ensure is idempotent, so a burst of concurrent openers on the
		// server collapses into one dial here.
		a.log.Infof("vr-agent: server asked for the data channel")
		a.data.Ensure()
	default:
		a.log.Infof("vr-agent: control frame %s", f.Type)
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
		if err := checkStateMachine(statePath, st, logger); err != nil {
			return nil, err
		}
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
		Fingerprint:    facts.Fingerprint(),
	})
	if err != nil {
		return nil, err
	}
	st = &state{
		ServerURL:  serverURL,
		AgentID:    resp.AgentID,
		HostID:     resp.HostID,
		AgentToken: resp.AgentToken,
		MachineID:  facts.MachineID,
	}
	if err := saveState(statePath, st, logger); err != nil {
		return nil, err
	}
	printInstallSummary(st, version, resp.ServerVersion)
	return st, nil
}

// printInstallSummary writes what this machine just became: which host it
// is, which credential it holds, and the two versions now paired.
//
// On stdout, alone. Every log line this agent writes goes to stderr, so
// install-agent.sh can capture this block whole and print it as its
// closing summary while still letting progress and errors reach the
// operator as they happen.
//
// Composed here rather than in the script because this is where the
// values are: the script stays a pipe with nothing to parse and nothing
// to keep in step when a line is added.
func printInstallSummary(st *state, agentVersion, serverVersion string) {
	fmt.Printf("    host          %d\n", st.HostID)
	fmt.Printf("    agent         %s\n", st.AgentID)
	fmt.Printf("    agent version %s\n", agentVersion)
	// A server older than this field says nothing rather than "unknown":
	// the line is here to answer "which build did I just join", and an
	// empty answer is better told by its absence than by a word that
	// reads like the server failed to identify itself.
	if serverVersion != "" {
		fmt.Printf("    server        %s (%s)\n", st.ServerURL, serverVersion)
	} else {
		fmt.Printf("    server        %s\n", st.ServerURL)
	}
}

// checkStateMachine refuses to reuse a credential that was issued to a
// different machine.
//
// The case this exists for: an operator builds a golden image from a host
// that is already onboarded, or full-clones its disk. /etc/vr-agent/agent.json
// travels with the copy, the install script sees a state file and skips
// registration, and every clone comes up holding the original's agent
// token. They then supersede each other's control channel forever and
// jobs land on whichever one happens to hold it.
//
// Refusing to start is the right failure: an agent that cannot prove
// which machine it is has nothing safe to do. The message names the fix
// because the obvious one (delete the state file) is wrong on its own --
// a raw clone carries the machine id too, so re-registering would just
// rebind the same host row.
func checkStateMachine(statePath string, st *state, logger stdLogger) error {
	current := hostinfo.MachineID()
	if current == "" {
		return fmt.Errorf("cannot determine a stable machine id (no /etc/machine-id and no hostname)")
	}
	if st.MachineID == "" {
		// Registered by an agent that predates this field. Adopt the
		// current identity so the check is live from the next start on.
		st.MachineID = current
		return saveState(statePath, st, logger)
	}
	if st.MachineID != current {
		return fmt.Errorf(
			"state file %s was issued to machine %q but this machine is %q. "+
				"This host looks cloned from an already-onboarded one. "+
				"Reset the machine identity first (`rm -f /etc/machine-id && systemd-machine-id-setup`, then reboot), "+
				"then delete %s and re-onboard with a fresh join token",
			statePath, st.MachineID, current, statePath)
	}
	return nil
}

// stdLogger adapts the standard library logger onto client.Logger.
type stdLogger struct{ l *log.Logger }

func (s stdLogger) Infof(format string, args ...any) { s.l.Printf(format, args...) }
func (s stdLogger) Warnf(format string, args ...any) { s.l.Printf("WARN "+format, args...) }
