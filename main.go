package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
)

func setWinsize(f *os.File, w, h int) {
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&struct{ h, w, x, y uint16 }{uint16(h), uint16(w), 0, 0})))
}

func main() {
	cmd := flag.String("cmd", "bash", "cmd to run when connected")
	port := flag.String("port", ":2222", "port to listen")
	authorizedKeyFile := flag.String("authorizedKeyFile", defaultAuthorizedKeyFile(), "authorizedKeyFile path")
	flag.Parse()
	ssh.Handle(func(s ssh.Session) {
		cmd := exec.Command(*cmd)
		ptyReq, winCh, isPty := s.Pty()
		if isPty {
			cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))
			f, err := pty.Start(cmd)
			if err != nil {
				panic(err)
			}
			go func() {
				for win := range winCh {
					setWinsize(f, win.Width, win.Height)
				}
			}()
			go func() {
				io.Copy(f, s) // stdin
			}()
			io.Copy(s, f) // stdout
			cmd.Wait()
		} else {
			io.WriteString(s, "No PTY requested.\n")
			s.Exit(1)
		}
	})

	data, _ := ioutil.ReadFile(*authorizedKeyFile)
	allowed, _, _, _, _ := ssh.ParseAuthorizedKey(data)

	publicKeyOption := ssh.PublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
		ssh.KeysEqual(key, allowed)
		return true // allow all keys, or use ssh.KeysEqual() to compare against known keys
	})

	log.Printf("starting ssh server on port %v...", *port)
	log.Fatal(ssh.ListenAndServe(*port, nil, publicKeyOption))
}

func defaultAuthorizedKeyFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "authorized_keys")
}
