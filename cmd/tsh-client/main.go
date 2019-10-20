package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/Spiderpowa/go-tan-shell/contract"
	"github.com/Spiderpowa/go-tan-shell/utils"
)

var filename = flag.String("config", "", "config file")
var mode = flag.String("mode", "", "tsh client mode (client or server)")
var clientID = flag.Int("client", 0, "client")
var shell = flag.String("shell", "sh", "shell for client mode")

func main() {
	flag.Parse()
	if *filename == "" ||
		(*mode != "client" && *mode != "server") ||
		(*mode == "server" && *clientID == 0) {
		fmt.Printf("usage[client]: %s --mode client --config filename [-shell shell]\n", os.Args[0])
		fmt.Printf("usage[server]: %s --mode server --config filename --client idx\n", os.Args[0])
		os.Exit(1)
	}
	cfg, err := utils.ReadConfigFromFile(*filename)
	if err != nil {
		panic(err)
	}
	prv, err := crypto.ToECDSA(common.FromHex(cfg.PrivateKey))
	if err != nil {
		panic(err)
	}
	client, err := contract.New(cfg.Endpoint, cfg.ContractAddress, prv)
	if err != nil {
		panic(err)
	}
	switch *mode {
	case "client":
		clientMode(client)
	case "server":
		serverMode(client)
	default:
		panic(fmt.Errorf("unknown mode %s", *mode))
	}
}
