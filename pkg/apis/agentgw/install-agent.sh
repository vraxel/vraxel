#!/bin/sh
# install-agent.sh -- install and register vr-agent on this host.
#
#   ./install-agent.sh --server https://vraxel.example.com --token <join-token>
#
# Idempotent: re-running on an already-onboarded host reinstalls the
# binary and restarts the service without registering again, so it never
# produces a duplicate host in vraxel. Pass --force-register to re-register
# anyway (recovery after a revoked token); the server keys registration
# on /etc/machine-id, so that still rebinds the same host row.
#
# POSIX sh, no bashisms: this runs on whatever minimal image the customer
# happens to have.

set -eu

main() {
    SERVER=""
    TOKEN=""
    FORCE_REGISTER=0
    INSTALL_DIR="/opt/vraxel/bin"
    STATE_DIR="/etc/vr-agent"
    STATE_FILE="${STATE_DIR}/agent.json"
    UNIT_FILE="/etc/systemd/system/vr-agent.service"
    BINARY="${INSTALL_DIR}/vr-agent"
    CA_FILE=""

    usage() {
        cat >&2 <<EOF
usage: $0 --server <url> --token <join-token> [--force-register] [--install-dir <dir>] [--ca-file <path>]

  --server          vraxel-server base URL, e.g. https://vraxel.example.com
  --token           one-time join token minted in vraxel
                    (compute > agent-join-tokens)
  --force-register  re-register even if this host is already onboarded
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
            --force-register) FORCE_REGISTER=1; shift ;;
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

    # Registration needs a token only on the first run.
    if [ ! -f "$STATE_FILE" ] || [ "$FORCE_REGISTER" = "1" ]; then
        [ -n "$TOKEN" ] || { echo "--token is required (host is not registered yet)" >&2; usage; }
    fi

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

    echo "==> downloading vr-agent (linux/${ARCH}) from ${DOWNLOAD_URL}"
    TMP_BIN="$(mktemp)"
    trap 'rm -f "$TMP_BIN"' EXIT
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
    echo "==> installing to ${BINARY}"
    mkdir -p "$INSTALL_DIR"
    # Stop first: replacing a running executable in place gives ETXTBSY.
    systemctl stop vr-agent 2>/dev/null || true
    install -m 0755 "$TMP_BIN" "$BINARY"

    # restorecon applies the context the policy assigns to this path
    # (bin_t under /opt/vraxel/bin), which is what makes the exec allowed.
    # Absent on non-SELinux distros, hence the guard.
    if command -v restorecon >/dev/null 2>&1; then
        echo "==> restoring SELinux context on ${BINARY}"
        restorecon -F "$BINARY" || true
    fi

    mkdir -p "$STATE_DIR"
    chmod 0700 "$STATE_DIR"

    if [ ! -f "$STATE_FILE" ] || [ "$FORCE_REGISTER" = "1" ]; then
        echo "==> registering with ${SERVER_TRIMMED}"
        REGISTER_ARGS="--server ${SERVER_TRIMMED} --token ${TOKEN} --register-only${CA_ARG}"
        [ "$FORCE_REGISTER" = "1" ] && REGISTER_ARGS="${REGISTER_ARGS} --re-register"
        # shellcheck disable=SC2086
        "$BINARY" $REGISTER_ARGS
        [ -f "$STATE_FILE" ] || { echo "registration produced no state file" >&2; exit 1; }
        echo "==> registered"
    else
        echo "==> already registered (${STATE_FILE}); skipping registration"
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
}

# Call main only after the whole script has been parsed. `curl | sh` feeds
# the shell a stream, so a transfer cut short would otherwise execute
# whatever prefix arrived; set -eu does not protect against truncation.
main "$@"
