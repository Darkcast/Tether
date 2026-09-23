//go:build !windows

package main

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"os/user"

	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
)

func createPty(s ssh.Session, shell string) {
	var (
		ptyReq, winCh, _ = s.Pty()
		cmd              = exec.CommandContext(s.Context(), shell)
	)

	cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))
	if currentUser, err := user.Current(); err == nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("HOME=%s", currentUser.HomeDir))
	}
	f, err := pty.Start(cmd)
	if err != nil {
		log.Fatalln("Could not start shell:", err)
	}
	go func() {
		for win := range winCh {
			winSize := &pty.Winsize{Rows: uint16(win.Height), Cols: uint16(win.Width)}
			pty.Setsize(f, winSize)
		}
	}()

	logFile := openSessionLog(s, "pty")
	if logFile != nil {
		defer logFile.Close()
	}

	go func() {
		io.Copy(f, s)
		s.Close()
	}()
	go func() {
		io.Copy(teeSessionOutput(s, logFile), f)
		s.Close()
	}()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			log.Println("Session ended with error:", err)
			s.Exit(255)
			return
		}
		log.Println("Session ended normally")
		s.Exit(cmd.ProcessState.ExitCode())
		return

	case <-s.Context().Done():
		log.Printf("Session terminated: %s", s.Context().Err())
		return
	}
}
