# lib/agent

The reusable half of the agent: everything a host agent needs to be
managed from a control plane it cannot be dialled from.

Outbound only. The agent listens on nothing.

## Packages

| Package | Role |
|---|---|
| `types` | The wire contract. Zero dependencies, shared verbatim by both ends. |
| `client` | Control channel: register, dial, heartbeat, reconnect, frame loop. |
| `datachan` | Data channel: one WSS per host, yamux streams, `tcp` / `pty` / `exec` / `file` kinds. |
| `probe` | Local periodic probes with k8s threshold semantics. |
| `transport` | The TLS trust store and HTTP client every outbound call shares. |
| `scrape` | Local exporter collection, pushed to VictoriaMetrics. |
| `upgrade` | Self-replacement of the agent binary, with rollback. |
| `hostinfo` | Static host facts for registration. |

## Dependency surface

`lib/agent/...` imports exactly one package from this repo,
`lib/websocket` (itself self-contained), and four third-party libraries:
`coder/websocket`, `hashicorp/yamux`, `creack/pty`, `google/uuid`.

It imports nothing from `pkg/apis`, and that is enforced:

```
go list -deps ./lib/agent/... | grep pkg/apis    # must be empty
```

Porting means copying `lib/agent/` and `lib/websocket/`, then writing a
`main` that supplies the config below. Job execution is deliberately not
here: the agent binary owns it, because running ansible plays is a platform
decision, not something every agent needs.

## What the host program supplies

```go
tlsCfg, _ := transport.LoadCA(caFile)      // "" = system trust store
httpClient := transport.HTTPClient(tlsCfg) // hand to every one of the below

ch := &client.Channel{ServerURL, AgentToken, Version, Log, HTTPClient, OnFrame, RunningJobs, ProbeStates}
dc := datachan.New(datachan.Config{ServerURL, Token, Guard, Shell, Log})
pr := probe.New(onChange)
sc := scrape.New(scrape.Config{ServerURL, Token, Log})
up := &upgrade.Manager{BinaryPath, StateDir, ServerURL, Version, Busy, VersionOf, Log}
```

`Log` is a two-method interface per package, so no logging library comes
along for the ride.

## What the server must implement

Endpoints (paths are built by `types`, so they cannot drift):

| Endpoint | Auth | Purpose |
|---|---|---|
| `POST /api/agent/v1/register` | join token | exchange a one-time token for a durable one |
| `GET /api/agent/v1/channel` | agent token | control channel (WSS) |
| `GET /api/agent/v1/data-channel` | session token | data channel (WSS), gateway is the yamux **client** |
| `GET /api/agent/v1/scrape-targets` | session token | which local exporters to scrape, where to push |
| `POST /api/agent/v1/token:renew` | session token | fresh durable token before the old one expires |
| `GET /api/agent/v1/binary/{os}/{arch}` | none | agent binaries, for install and self-upgrade |

Frames are one JSON envelope, `types.Frame`, switched on `Type`. The
server sends `session.token`, `job.dispatch`, `job.cancel`,
`channel.open`, `config.reload`, `probe.config`, `agent.upgrade`; the agent sends `hello`,
`heartbeat`, `job.ack`, `probe.report`, `agent.status`, `error`.

## Two properties worth preserving in a port

**The agent never trusts a target it is handed.** A `tcp` stream must
resolve to a loopback address, names included, or it is refused. An
attacker who owns the control plane still cannot use an agent to reach
anything else on the customer's network. The port allowlist rides on top
of that rule, not in place of it.

**The agent never trusts a URL it is handed.** The upgrade frame carries
a version and a digest; the download URL is built from the server the
agent is enrolled with.

**A private CA replaces the public roots, it does not join them.**
`transport.LoadCA` builds a pool from the given bundle alone. A
deployment that issues its own certificates did so to stop trusting
several hundred commercial CAs; merging the system pool would hand that
trust straight back.

## Stream framing

`tcp` streams are raw after the handshake: they are a `net.Conn`, and the
gateway runs `http.Transport` over them. That single behaviour serves
HTTP APIs, k8s REST, SPDY exec, streaming logs and TCP probes, and it is
why response streaming is automatic rather than a feature.

`pty` and `file` streams are framed (1-byte type, 4-byte length), because
both need out-of-band messages interleaved with payload: a terminal
resize between two chunks of output, a file op's result after its body.
