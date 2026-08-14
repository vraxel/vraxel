#!/bin/sh
# install-agent.sh -- install and register vr-agent on this host.
#
#   ./install-agent.sh --server https://vraxel.example.com --token <join-token>
#
# One command, one behaviour: this always registers, so a join token is
# always required. The state file is an output of that decision, never an
# input to it.
#
# The alternative -- skip registration when the state file is already
# there -- infers the operator's intent from the filesystem, and gets it
# wrong in exactly the case that needs it most: a host deleted on the
# server leaves a state file holding a credential the server no longer
# honours, so the recovery command silently did nothing while the agent
# retried a rejected token forever.
#
# Registering unconditionally is safe because the server keys identity on
# /etc/machine-id: a machine that has onboarded before rebinds its
# existing host row instead of creating a second one, and a rebind never
# moves a host between scopes (rebindAuthorised refuses a token from
# another tenant outright).
#
# There is no upgrade-only mode either: upgrading is this same command,
# run again. It costs one join token and rotates the credential, which is
# a smaller price than a second mechanism doing half of what this one
# already does -- and the half it would skip (re-registering) is the half
# that heals a host whose credential has gone stale.
#
# POSIX sh, no bashisms: this runs on whatever minimal image the customer
# happens to have.

set -eu

main() {
    SERVER=""
    TOKEN=""
    # Both are read by cleanup(), which can fire from anywhere below.
    NEW_BINARY=""
    AGENT_WAS_RUNNING=0
    # The closing summary, produced by the agent's registration. Empty
    # under set -u until then, and stays empty against an agent too old to
    # print one.
    SUMMARY=""
    INSTALL_DIR="/opt/vraxel/bin"
    STATE_DIR="/etc/vr-agent"
    STATE_FILE="${STATE_DIR}/agent.json"
    UNIT_FILE="/etc/systemd/system/vr-agent.service"
    BINARY="${INSTALL_DIR}/vr-agent"
    CA_FILE=""

    usage() {
        cat >&2 <<EOF
usage: $0 --server <url> --token <join-token> [--install-dir <dir>] [--ca-file <path>]

  --server          vraxel-server base URL, e.g. https://vraxel.example.com
  --token           one-time join token minted in vraxel (compute >
                    agent-join-tokens, or the button on a host's page).
                    Required every time: this script always registers.
  --install-dir     where to place the binary (default ${INSTALL_DIR})
  --ca-file         PEM bundle of the CA that signed the vraxel-server
                    certificate. Required when that certificate is not
                    publicly trusted; the file must stay readable at the
                    given path, since the agent re-reads it on every start.
EOF
        exit 2
    }

    while [ $# -gt 0 ]; do
        case "$1" in
            # ${2:?...} rather than ${2:-}: a flag given without a value
            # used to make `shift 2` run off the end, and under set -eu the
            # script then exited silently with no clue what was wrong.
            --server) SERVER="${2:?--server needs a URL}"; shift 2 ;;
            --token) TOKEN="${2:?--token needs a value}"; shift 2 ;;
            # Accepted and ignored. Registration is now unconditional, so
            # this asks for what already happens -- but agents built
            # before that change print it as the recovery instruction, and
            # failing on it would send whoever followed that advice
            # looking for a second problem.
            --force-register) echo "note: --force-register is no longer needed; this script always registers" >&2; shift ;;
            --install-dir) INSTALL_DIR="${2:?--install-dir needs a path}"; BINARY="${INSTALL_DIR}/vr-agent"; shift 2 ;;
            --ca-file) CA_FILE="${2:?--ca-file needs a path}"; shift 2 ;;
            -h|--help) usage ;;
            *) echo "unknown argument: $1" >&2; usage ;;
        esac
    done

    [ -n "$SERVER" ] || { echo "--server is required" >&2; usage; }
    if [ -n "$CA_FILE" ]; then
        [ -r "$CA_FILE" ] || { echo "--ca-file ${CA_FILE} is not readable" >&2; exit 1; }
        CA_ARG=" --ca-file ${CA_FILE}"
    else
        CA_ARG=""
    fi
    [ "$(id -u)" = "0" ] || { echo "must run as root" >&2; exit 1; }

    [ -n "$TOKEN" ] || { echo "--token is required" >&2; usage; }

    case "$(uname -m)" in
        x86_64|amd64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
    esac

    SERVER_TRIMMED="${SERVER%/}"
    DOWNLOAD_URL="${SERVER_TRIMMED}/api/agent/v1/binary/linux/${ARCH}"

    # The server substitutes the expected digest below when it serves this
    # script. Verifying it matters because the binary that follows runs as
    # root on every managed host, and plenty of deployments front vraxel with
    # plain HTTP inside the intranet, where a swapped download is a
    # one-hop attack rather than a theoretical one.
    # Two literal placeholders, because the server substitutes literal
    # strings: writing __VRAXEL_AGENT_SHA256_${ARCH}__ would leave nothing in
    # the file for it to match, and verification would silently never run.
    SHA_amd64="__VRAXEL_AGENT_SHA256_amd64__"
    SHA_arm64="__VRAXEL_AGENT_SHA256_arm64__"
    case "$ARCH" in
        amd64) EXPECTED_SHA="$SHA_amd64" ;;
        arm64) EXPECTED_SHA="$SHA_arm64" ;;
        *)     EXPECTED_SHA="" ;;
    esac

    # Checked BEFORE the download, and fatal.
    #
    # The digest and the binary come from the same file on the server, so a
    # placeholder that survived substitution means there is no binary to
    # fetch either -- the download would return 404 and `curl -f` would
    # discard the server's explanation along with the body. Saying it here
    # turns "curl: (22) error 404" into the actual problem.
    #
    # Fatal rather than a warning: the alternative is installing an
    # unverified binary that runs as root on this host.
    case "$EXPECTED_SHA" in
        ""|__VRAXEL_AGENT_SHA256_*)
            echo "this server has no vr-agent binary for linux/${ARCH}." >&2
            echo "build it there with \`make agent-binaries\`, or point VRAXEL_AGENT_BINARY_DIR at a directory holding vr-agent-linux-${ARCH}." >&2
            exit 1
            ;;
    esac

    # cleanup owns every exit path from here on.
    #
    # The one thing this script must never do is leave a host without a
    # running agent. Registration requires stopping the one that is there
    # (the server refuses a re-registration while the channel is live), so
    # between that stop and the final restart there is a window where a
    # failure -- a spent token, an unreachable server, a full disk --
    # would otherwise walk away from a silently unmanaged machine.
    #
    # Restarting is always safe: the old credential is either untouched
    # (registration failed, and the server refunded the token) or already
    # replaced on disk (it succeeded, and a later step failed). Both are
    # states the old binary can run in.
    cleanup() {
        status=$?
        rm -f "$TMP_BIN"
        if [ -n "$NEW_BINARY" ]; then rm -f "$NEW_BINARY"; fi
        if [ "$status" -ne 0 ] && [ "$AGENT_WAS_RUNNING" = "1" ]; then
            echo "==> install failed; restarting the previous agent" >&2
            systemctl start vr-agent >/dev/null 2>&1 || true
        fi
        exit $status
    }

    # Stdout carries the summary block and nothing else; the agent logs to
    # stderr, which is left streaming so progress and failures reach the
    # operator as they happen rather than at the end.
    register_agent() {
        # --re-register unconditionally: the agent short-circuits on an
        # existing state file otherwise, which is the same inference this
        # script just stopped making.
        # shellcheck disable=SC2086
        "$NEW_BINARY" --server "$SERVER_TRIMMED" --token "$TOKEN" \
            --register-only --re-register $CA_ARG
    }

    echo "==> downloading vr-agent (linux/${ARCH}) from ${DOWNLOAD_URL}"
    TMP_BIN="$(mktemp)"
    trap cleanup EXIT
    if command -v curl >/dev/null 2>&1; then
        # Deliberately not -f: it turns an HTTP error into a bare exit
        # status and throws away the body, which is where the server says
        # what is wrong. Taking the status from -w instead keeps the body
        # in $TMP_BIN, so a failure can be quoted back. One request, so a
        # transient fault cannot be retried into printing a 9MB binary.
        HTTP_CODE="$(curl -sSL -w '%{http_code}' -o "$TMP_BIN" "$DOWNLOAD_URL")" || HTTP_CODE="000"
        if [ "$HTTP_CODE" != "200" ]; then
            echo "download failed from ${DOWNLOAD_URL} (HTTP ${HTTP_CODE}):" >&2
            head -c 400 "$TMP_BIN" >&2
            echo >&2
            exit 1
        fi
    elif command -v wget >/dev/null 2>&1; then
        wget -q -O "$TMP_BIN" "$DOWNLOAD_URL" || {
            echo "download failed from ${DOWNLOAD_URL}" >&2
            exit 1
        }
    else
        echo "neither curl nor wget is available" >&2
        exit 1
    fi
    [ -s "$TMP_BIN" ] || { echo "downloaded binary is empty" >&2; exit 1; }

    if command -v sha256sum >/dev/null 2>&1; then
        ACTUAL_SHA="$(sha256sum "$TMP_BIN" | cut -d' ' -f1)"
    elif command -v shasum >/dev/null 2>&1; then
        ACTUAL_SHA="$(shasum -a 256 "$TMP_BIN" | cut -d' ' -f1)"
    else
        echo "neither sha256sum nor shasum is available; cannot verify the download" >&2
        exit 1
    fi
    [ "$ACTUAL_SHA" = "$EXPECTED_SHA" ] || {
        echo "binary checksum mismatch: got ${ACTUAL_SHA}, expected ${EXPECTED_SHA}" >&2
        exit 1
    }
    echo "==> checksum verified"

    # The binary MUST NOT be executed from /tmp. Under the SELinux policy
    # Rocky 8 ships enabled by default, files created in /tmp carry
    # tmp_t, and systemd (init_t) is not permitted to exec that type: the
    # unit fails with status=203/EXEC while the very same binary runs fine
    # in the foreground. Measured in spike 0.2. So: install to a real
    # location first, relabel, and only then write the unit.
    # Staged under .new rather than written straight to $BINARY: a run
    # that fails during registration then leaves the previous install
    # exactly as it was, old binary and all. It also sidesteps the ETXTBSY
    # that replacing a running executable in place would give.
    echo "==> staging ${BINARY}"
    mkdir -p "$INSTALL_DIR"
    NEW_BINARY="${BINARY}.new"
    install -m 0755 "$TMP_BIN" "$NEW_BINARY"

    # restorecon applies the context the policy assigns to this path
    # (bin_t under /opt/vraxel/bin), which is what makes the exec allowed.
    # Absent on non-SELinux distros, hence the guard.
    if command -v restorecon >/dev/null 2>&1; then
        restorecon -F "$NEW_BINARY" || true
    fi

    # Down before registering, not after: a live control channel makes the
    # server refuse the registration outright, because a machine id
    # presented while its own agent is still connected is more likely to
    # be someone else holding it than a reinstall.
    if systemctl is-active --quiet vr-agent 2>/dev/null; then
        AGENT_WAS_RUNNING=1
        echo "==> stopping the running agent"
        systemctl stop vr-agent || true
    fi

    mkdir -p "$STATE_DIR"
    chmod 0700 "$STATE_DIR"

    echo "==> registering with ${SERVER_TRIMMED}"
    if ! SUMMARY="$(register_agent)"; then
        # The stop above only reaches the server as a socket close
        # travelling over the network, and until it lands the server still
        # sees a live channel. One retry covers that window; a second
        # would be waiting on something that is not going to change.
        echo "==> registration failed; retrying once in 3s" >&2
        sleep 3
        SUMMARY="$(register_agent)"
    fi
    [ -f "$STATE_FILE" ] || { echo "registration produced no state file" >&2; exit 1; }
    echo "==> registered"

    # Promote the staged binary only now: everything that could have sent
    # us to cleanup() has already run.
    mv -f "$NEW_BINARY" "$BINARY"
    NEW_BINARY=""
    if command -v restorecon >/dev/null 2>&1; then
        echo "==> restoring SELinux context on ${BINARY}"
        restorecon -F "$BINARY" || true
    fi

    echo "==> writing ${UNIT_FILE}"
    cat > "$UNIT_FILE" <<EOF
[Unit]
Description=vraxel host agent
Documentation=https://vraxel.io/docs/agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BINARY} --server ${SERVER_TRIMMED}${CA_ARG}
# Restart=always with a bounded delay: a crashed agent means an
# unmanaged host, and the agent holds no state that a restart could
# corrupt (an interrupted job is failed server-side, never resumed).
Restart=always
RestartSec=5
# StartLimitInterval=0 disables systemd's give-up-after-N-restarts
# behaviour. The default would permanently stop retrying after a burst
# of failures -- e.g. an vraxel-server outage during a reboot -- leaving
# the host silently unmanaged with no operator signal.
StartLimitInterval=0
User=root
WorkingDirectory=/opt/vraxel

[Install]
WantedBy=multi-user.target
EOF

    if command -v restorecon >/dev/null 2>&1; then
        restorecon -F "$UNIT_FILE" || true
    fi

    systemctl daemon-reload
    systemctl enable vr-agent >/dev/null 2>&1 || true
    systemctl restart vr-agent

    echo "==> vr-agent installed and started"
    systemctl --no-pager status vr-agent | head -n 5 || true

    # Last, because it is the one part an operator keeps: the host it can
    # now find this machine under, and the two versions that were paired.
    # An `if` rather than `[ -n "$SUMMARY" ] && printf ...`, which would
    # make an empty summary the script's exit status and send a successful
    # install through cleanup()'s failure path.
    if [ -n "$SUMMARY" ]; then
        # Blank line first: systemctl's status block ends on an indented
        # "Main PID:" line, and without a separator the summary reads as
        # more of systemd's output rather than as ours.
        printf '\n%s\n' "$SUMMARY"
    fi
}

# Call main only after the whole script has been parsed. `curl | sh` feeds
# the shell a stream, so a transfer cut short would otherwise execute
# whatever prefix arrived; set -eu does not protect against truncation.
main "$@"
