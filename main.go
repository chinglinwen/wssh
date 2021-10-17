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
	"strings"
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
	defaultCmd := flag.String("cmd", "bash", "cmd to run when connected")
	port := flag.String("port", ":2222", "port to listen")
	pubKey := flag.String("pubkey", defaultPubKeys, "default pub key for auth if provided")
	authorizedKeyFile := flag.String("authorizedKeyFile", defaultAuthorizedKeyFile(), "default pub keys also read from authorizedKeyFile path if provided")
	flag.Parse()
	ssh.Handle(func(s ssh.Session) {
		log.Printf("got connect from: %v, raddr: %v, local: %v, cmd: %v\n",
			s.User(), s.RemoteAddr(), s.LocalAddr(), s.Command())
		defer func() {
			log.Printf("%v disconnected\n", s.User())
		}()
		cmdarg := strings.Fields(strings.Join(s.Command(), " "))
		if len(cmdarg) == 0 {
			cmdarg = []string{*defaultCmd}
		}
		var cmd *exec.Cmd
		if len(cmdarg) >= 1 {
			cmd = exec.Command(cmdarg[0], cmdarg[1:]...)
		} else {
			cmd = exec.Command(cmdarg[0])
		}
		ptyReq, winCh, isPty := s.Pty()
		if isPty {
			cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))
			f, err := pty.Start(cmd)
			if err != nil {
				io.WriteString(s, fmt.Sprintf("cmd: %v, err: %v\n", cmdarg, err))
				s.Exit(1)
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
			out, err := cmd.CombinedOutput()
			if err != nil {
				io.WriteString(s, fmt.Sprintf("cmd: %v, err: %v, out: %s\n", cmdarg, err, string(out)))
				s.Exit(1)
			}
			io.WriteString(s, string(out))
			s.Exit(1)
		}
	})

	data, _ := ioutil.ReadFile(*authorizedKeyFile)
	data = append(data, []byte(*pubKey)...) // for my pubkey
	allowed, _, _, _, _ := ssh.ParseAuthorizedKey(data)

	publicKeyOption := ssh.PublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
		return ssh.KeysEqual(key, allowed)
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

const defaultPubKeys = `
ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC9QbQ8vtHRECYafrld1KsdF92Q0DG92PUmw3kbRxS9tMFHdraKxKi3hf1jrx/MtrrzWgQbP/LQPfuqs82+u0ADT3QS/4CVTusYET5thk6fPRYmANVFYaOYqsJsqSRH8svCiVhceUDvF8VTv5gc83jAozAdB6qbK6Nn2DalYne5qz/jyvC5j15q315Lc2COlKPx0MdzCVVo7sTIaQIGtL/H1yB+CoOltpmZ6voux0KjuZlcCftEf1kkTX6xCuvdDkhZLFuvl+UGJBvcbYiW6MRHJ2b/89eY0KMPAcspEyqB12O4XfAIrxB7vyhnnLQEWMJWKfjx8s+z5LbByK8TwWTz HDB-20171208HRW+Administrator@HDB-20171208HRW
`
