package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gliderlabs/ssh"
)

// sessionLogDir, when non-empty, is the directory session output is written to.
// Configured via the -L flag or the SESSLOG build variable.
var sessionLogDir = ""

// openSessionLog creates a per-session log file when sessionLogDir is set and
// returns a writer for the shell's output stream. It never captures input so a
// password typed at a prompt does not end up on disk. On any failure it logs
// and returns nil so the caller can fall back to the plain session writer.
func openSessionLog(s ssh.Session, kind string) io.WriteCloser {
	if sessionLogDir == "" {
		return nil
	}
	if err := os.MkdirAll(sessionLogDir, 0o700); err != nil {
		log.Printf("session-log: mkdir %s: %v", sessionLogDir, err)
		return nil
	}

	peer := s.RemoteAddr().String()
	safePeer := strings.NewReplacer(":", "_", "/", "_").Replace(peer)
	safeUser := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(s.User())
	name := fmt.Sprintf("%s-%s-%s.log",
		time.Now().UTC().Format("20060102T150405Z"),
		safePeer,
		safeUser,
	)
	path := filepath.Join(sessionLogDir, name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		log.Printf("session-log: open %s: %v", path, err)
		return nil
	}

	ptyReq, _, isPty := s.Pty()
	term := ""
	if isPty {
		term = fmt.Sprintf(" term=%s size=%dx%d", ptyReq.Term, ptyReq.Window.Width, ptyReq.Window.Height)
	}
	cmd := strings.Join(s.Command(), " ")
	if cmd == "" {
		cmd = "-"
	}
	fmt.Fprintf(f, "# tether session log kind=%s user=%s peer=%s cmd=%q start=%s%s\n",
		kind, s.User(), peer, cmd, time.Now().UTC().Format(time.RFC3339Nano), term)

	log.Printf("Recording session output to %s", path)
	return f
}

// teeSessionOutput returns w wrapped so writes are duplicated into logFile when
// one is present. Callers pass the ssh.Session (or PTY writer) as w.
func teeSessionOutput(w io.Writer, logFile io.Writer) io.Writer {
	if logFile == nil {
		return w
	}
	return io.MultiWriter(w, logFile)
}
