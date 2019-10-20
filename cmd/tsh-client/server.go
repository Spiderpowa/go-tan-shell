package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/Spiderpowa/go-tan-shell/contract"
)

func serverMode(client *contract.Client) {
	id := int64(*clientID)
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("> ")
		if !scanner.Scan() {
			break
		}
		wg := sync.WaitGroup{}
		input := []byte(scanner.Text())
		stdout, stderr, err := client.Write(context.Background(), id, input)
		if err != nil {
			fmt.Println("error sending command", err)
			continue
		}
		connect := func(writer io.Writer, reader io.Reader) {
			defer wg.Done()
			if _, err := io.Copy(writer, reader); err != nil {
				if err == io.EOF {
					return
				}
				fmt.Fprintln(os.Stderr, "error copying", err)
			}
		}
		wg.Add(2)
		go connect(os.Stdout, stdout)
		go connect(os.Stderr, stderr)
		wg.Wait()
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
}
