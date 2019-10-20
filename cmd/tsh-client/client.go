package main

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/Spiderpowa/go-tan-shell/contract"
)

var shellMap = map[string][]string{
	"sh":   []string{"-c"},
	"bash": []string{"-c"},
}

func buildCmd(cmd []byte) *exec.Cmd {
	args := append(shellMap[*shell], string(cmd))
	return exec.Command(*shell, args...)
}

func clientMode(client *contract.Client) {
	if _, exist := shellMap[*shell]; !exist {
		panic(fmt.Errorf("unsupported shell: %s", *shell))
	}
	if _, err := exec.LookPath(*shell); err != nil {
		panic(err)
	}
	ch := make(chan *contract.Command)
	client.Listen(ch)
	fmt.Println("Listening to remote command...")
	for {
		func() {
			command := <-ch
			fmt.Println("Executing command", string(command.CMD))
			cmd := buildCmd(command.CMD)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				command.Stderr <- []byte(err.Error())
				close(command.Stderr)
				return
			}
			defer stdout.Close()
			stderr, err := cmd.StderrPipe()
			if err != nil {
				command.Stderr <- []byte(err.Error())
				close(command.Stderr)
				return
			}
			defer stderr.Close()
			connect := func(writer chan<- []byte, reader io.Reader) {
				for {
					buf := make([]byte, 20*1024)
					n, err := reader.Read(buf)
					if err != nil {
						if err == io.EOF {
							close(writer)
						} else {
							command.Stderr <- []byte(err.Error())
							close(command.Stderr)
						}
						break
					}
					writer <- buf[:n]
				}
			}
			go connect(command.Stdout, stdout)
			go connect(command.Stderr, stderr)
			if err := cmd.Start(); err != nil {
				command.Stderr <- []byte(err.Error())
				close(command.Stderr)
				return
			}
			if err := cmd.Wait(); err != nil {
				command.Stderr <- []byte(err.Error())
				close(command.Stderr)
				return
			}
		}()
	}
}
