package main

import (
	"bytes"
	"koding/tools/dnode"
	"koding/tools/kite"
	"koding/tools/log"
	"koding/tools/pty"
	"koding/virt"
	"os"
	"syscall"
	"time"
	"unicode/utf8"
)

type WebtermServer struct {
	remote           WebtermRemote
	pty              *pty.PTY
	process          *os.Process
	currentSecond    int64
	messageCounter   int
	byteCounter      int
	lineFeeedCounter int
}

type WebtermRemote struct {
	Output       dnode.Callback
	SessionEnded dnode.Callback
}

func newWebtermServer(session *kite.Session, remote WebtermRemote, args []string, sizeX, sizeY int) *WebtermServer {
	server := &WebtermServer{
		remote: remote,
		pty:    pty.New(pty.DefaultPtsPath),
	}
	server.SetSize(float64(sizeX), float64(sizeY))
	session.OnDisconnect(func() { server.Close() })

	user := SessionUser(session)
	cmd := virt.GetDefaultVM(user).AttachCommand(user.Uid) // empty command is default shell
	server.pty.AdaptCommand(cmd)
	err := cmd.Start()
	if err != nil {
		panic(err)
	}
	server.process = cmd.Process

	go func() {
		defer log.RecoverAndLog()

		cmd.Wait()
		server.pty.Master.Close()
		server.pty.Slave.Close()
		server.remote.SessionEnded()
	}()

	go func() {
		defer log.RecoverAndLog()

		buf := make([]byte, (1<<12)-4, 1<<12)
		runes := make([]rune, 1<<12)
		for {
			n, err := server.pty.Master.Read(buf)
			for n < cap(buf)-1 {
				r, _ := utf8.DecodeLastRune(buf[:n])
				if r != utf8.RuneError {
					break
				}
				server.pty.Master.Read(buf[n : n+1])
				n++
			}

			s := time.Now().Unix()
			if server.currentSecond != s {
				server.currentSecond = s
				server.messageCounter = 0
				server.byteCounter = 0
				server.lineFeeedCounter = 0
			}
			server.messageCounter += 1
			server.byteCounter += n
			server.lineFeeedCounter += bytes.Count(buf[:n], []byte{'\n'})
			if server.messageCounter > 100 || server.byteCounter > 1<<18 || server.lineFeeedCounter > 300 {
				time.Sleep(time.Second)
			}

			// convert manually to fix invalid utf-8 chars
			i := 0
			c := 0
			for {
				r, l := utf8.DecodeRune(buf[i:n])
				if l == 0 {
					break
				}
				if r >= 0xD800 {
					r = utf8.RuneError
				}
				runes[c] = r
				i += l
				c++
			}

			server.remote.Output(string(runes[:c]))
			if err != nil {
				break
			}
		}
	}()

	return server
}

func (server *WebtermServer) Input(data string) {
	server.pty.Master.Write([]byte(data))
}

func (server *WebtermServer) ControlSequence(data string) {
	server.pty.MasterEncoded.Write([]byte(data))
}

func (server *WebtermServer) SetSize(x, y float64) {
	server.pty.SetSize(uint16(x), uint16(y))
}

func (server *WebtermServer) Close() error {
	server.process.Signal(syscall.SIGHUP)
	return nil
}
