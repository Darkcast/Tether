#!/usr/bin/env bash
# tether builder - thin wrapper around `go build`.
#
# Usage:
#   ./build.sh                 Build for the host OS/arch into bin/tether
#   ./build.sh all             Build the full release matrix (Linux/Windows/macOS, x86+ARM)
#   ./build.sh test            Build bin/tether-test with a known password + bundled key (local testing)
#
# Customise the baked-in defaults with env vars (all optional):
#   PASS=secret        login password                    (default: random 8 bytes; test build: testpass123)
#   PUB="ssh-ed25519 ..."   authorized public key        (default: none)
#   SHELL_BIN=/bin/sh  shell to spawn                    (default: /bin/bash)
#   LHOST=10.0.0.1     dial-home host(s), comma-sep      (default: none = bind mode)
#   LPORT=443          listen/connect port                (default: 31337)
#   BPORT=0            reverse bind port                  (0 = any free port)
#   PROXY=socks5://... outbound proxy URL                 (default: env HTTPS_PROXY / ALL_PROXY)
#   SNI=host.example   TLS server name; enables TLS wrap  (default: disabled)
#
# Examples:
#   PASS=hunter2 ./build.sh
#   LHOST=10.0.0.1 LPORT=443 ./build.sh all
#   ./build.sh test        # then: ./bin/tether-test -v -l -p 2222

set -euo pipefail
cd "$(dirname "$0")"

# --- assemble ldflags from env ------------------------------------------------
PASS="${PASS:-$(head -c8 /dev/urandom | xxd -p)}"
LD="-s -w -X 'main.localPassword=${PASS}'"
[ -n "${PUB:-}" ]       && LD="$LD -X 'main.authorizedKey=${PUB}'"
[ -n "${SHELL_BIN:-}" ] && LD="$LD -X 'main.defaultShell=${SHELL_BIN}'"
[ -n "${LHOST:-}" ]     && LD="$LD -X 'main.LHOST=${LHOST}'"
[ -n "${LPORT:-}" ]     && LD="$LD -X 'main.LPORT=${LPORT}'"
[ -n "${BPORT:-}" ]     && LD="$LD -X 'main.HomeBindPort=${BPORT}'"
[ -n "${SESSLOG:-}" ]   && LD="$LD -X 'main.SESSLOG=${SESSLOG}'"
[ -n "${SNI:-}" ]       && LD="$LD -X 'main.SNI=${SNI}'"
[ -n "${PROXY:-}" ]     && LD="$LD -X 'main.PROXY=${PROXY}'"

build() { # os arch out [extra-ld]
	local os="$1" arch="$2" out="$3" extra="${4:-}"
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" ${GOARM:+GOARM=$GOARM} \
		go build -trimpath -ldflags="$LD $extra" -o "bin/$out" .
	echo "  bin/$out"
}

mkdir -p bin

case "${1:-host}" in
host)
	echo "Building for host ($(go env GOOS)/$(go env GOARCH))..."
	CGO_ENABLED=0 go build -trimpath -ldflags="$LD" -o bin/tether .
	echo "  bin/tether"
	echo "Login password: ${PASS}"
	;;

all)
	echo "Building release matrix..."
	build linux   amd64 tether-linux-amd64
	build linux   386   tether-linux-386
	build linux   arm64 tether-linux-arm64
	GOARM=7 build linux arm tether-linux-armv7
	build windows amd64 tether-windows-amd64.exe
	build windows 386   tether-windows-386.exe
	build windows arm64 tether-windows-arm64.exe
	build darwin  amd64 tether-darwin-amd64
	build darwin  arm64 tether-darwin-arm64
	echo "Login password: ${PASS}"
	;;

test)
	# Predictable creds for local testing: known password + bundled key authorized.
	PASS="${PASS:-testpass123}"
	LD="-s -w -X 'main.localPassword=${PASS}' -X 'main.authorizedKey=$(cat assets/id_tether.pub)'"
	echo "Building bin/tether-test (password=${PASS}, key=assets/id_tether)..."
	CGO_ENABLED=0 go build -trimpath -ldflags="$LD" -o bin/tether-test .
	echo "Run:  ./bin/tether-test -v -l -p 2222"
	echo "Then: ssh -i assets/id_tether -p 2222 localhost   (chmod 600 the key first)"
	echo "  or: sftp -P 2222 localhost                       (password: ${PASS})"
	;;

*)
	echo "Unknown target '$1'. Use: host | all | test" >&2
	exit 1
	;;
esac
