package main

import (
	"io"
	"log"
	"os/exec"

	"github.com/gliderlabs/ssh"
)

func createSSHSessionHandler(shell string) ssh.Handler {
	return func(s ssh.Session) {
		log.Printf("New login from %s@%s", s.User(), s.RemoteAddr().String())
		_, _, isPty := s.Pty()

		switch {
		case isPty:
			log.Println("PTY requested")

			createPty(s, shell)

		case len(s.Command()) > 0:
			log.Printf("Command execution requested: '%s'", s.RawCommand())

			cmd := exec.CommandContext(s.Context(), s.Command()[0], s.Command()[1:]...)

			// We use StdinPipe to avoid blocking on missing input
			if stdin, err := cmd.StdinPipe(); err != nil {
				log.Println("Could not initialize stdinPipe", err)
				s.Exit(255)
				return
			} else {
				go func() {
					if _, err := io.Copy(stdin, s); err != nil {
						log.Printf("Error while copying input from %s to stdin: %s", s.RemoteAddr().String(), err)
					}
					s.Close()
				}()
			}

			cmd.Stdout = s
			cmd.Stderr = s

			done := make(chan error, 1)
			go func() { done <- cmd.Run() }()

			select {
			case err := <-done:
				if err != nil {
					log.Println("Command execution failed:", err)
					io.WriteString(s, "Command execution failed: "+err.Error()+"\n")
					s.Exit(255)
					return
				}
				log.Println("Command execution successful")
				s.Exit(cmd.ProcessState.ExitCode())
				return

			case <-s.Context().Done():
				log.Printf("Session terminated: %s", s.Context().Err())
				return
			}

		default:
			log.Println("No PTY requested, no command supplied")

			// Keep this open until the session exits, could e.g. be port forwarding
			<-s.Context().Done()
			log.Printf("Session terminated: %s", s.Context().Err())
			return
		}
	}
}
