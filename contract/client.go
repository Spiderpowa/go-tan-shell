package contract

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"fmt"
	"io"
	"math/big"
	"os"
	"sync"

	tanshell "github.com/Spiderpowa/tan-shell"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Client for accessing contract.
type Client struct {
	client   *ethclient.Client
	msgID    int64
	nonce    uint64
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	callLock sync.Mutex

	// contract accesser data
	address         common.Address
	contractAddress common.Address
	contract        *tanshell.Tanshell
	privateKey      *ecdsa.PrivateKey
	signer          types.Signer
}

// New creates a new Client instance for accessing tan-shell contract.
func New(
	endpoint string,
	contractAddress string,
	privateKey *ecdsa.PrivateKey,
) (*Client, error) {
	client, err := ethclient.Dial(endpoint)
	if err != nil {
		return nil, err
	}
	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		return nil, err
	}
	ret := &Client{
		client:          client,
		address:         crypto.PubkeyToAddress(privateKey.PublicKey),
		contractAddress: common.HexToAddress(contractAddress),
		privateKey:      privateKey,
		signer:          types.NewEIP155Signer(chainID),
	}
	contract, err := tanshell.NewTanshell(ret.contractAddress, client)
	if err != nil {
		return nil, err
	}
	ret.contract = contract
	if err := ret.updateNonce(context.Background()); err != nil {
		return nil, err
	}
	ret.ctx, ret.cancel = context.WithCancel(context.Background())
	return ret, nil
}

// Close the client gracefully.
func (c *Client) Close() {
	c.cancel()
	c.wg.Wait()
}

func (c *Client) updateNonce(ctx context.Context) error {
	nonce, err := c.client.NonceAt(ctx, c.address, nil)
	if err != nil {
		return err
	}
	c.nonce = nonce
	return nil
}

func (c *Client) call(ctx context.Context, transact func(*bind.TransactOpts) error) error {
	gasPrice, err := c.client.SuggestGasPrice(ctx)
	if err != nil {
		return err
	}
	opts := &bind.TransactOpts{
		From: c.address,
		Signer: func(_ types.Signer, _ common.Address, tx *types.Transaction) (*types.Transaction, error) {
			return types.SignTx(tx, c.signer, c.privateKey)
		},
		Context:  ctx,
		GasPrice: gasPrice,
		GasLimit: 100000,
	}
	c.callLock.Lock()
	defer c.callLock.Unlock()
TxLoop:
	for {
		select {
		case <-ctx.Done():
			break TxLoop
		default:
		}
		opts.Nonce = new(big.Int).SetUint64(c.nonce)
		if err := transact(opts); err != nil {
			fmt.Println(err)
			if err.Error() == "nonce too low" {
				fmt.Println("updating nonce", err)
				c.updateNonce(ctx)
				continue
			} else if err.Error() == "replacement transaction underpriced" {
				opts.GasPrice = new(big.Int).Add(opts.GasPrice, bigOne)
				fmt.Println("increase gasprice", opts.GasPrice)
				continue
			}
			return err
		}
		break TxLoop
	}
	c.nonce++
	return nil
}

// Write sends the shell command to the remote client.
func (c *Client) Write(ctx context.Context, client int64, data []byte) (io.Reader, io.Reader, error) {
	c.msgID++
	msgID := big.NewInt(c.msgID)
	clientID := big.NewInt(client)
	buffer := bytes.NewReader(data)
	for {
		chunk := make([]byte, maxDataSize)
		n, err := buffer.Read(chunk)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, nil, err
		}
		chunk = chunk[:n]
		if err := c.call(ctx, func(opts *bind.TransactOpts) error {
			_, err := c.contract.Stdin(opts, clientID, msgID, chunk, false)
			return err
		}); err != nil {
			return nil, nil, err
		}
	}
	if err := c.call(ctx, func(opts *bind.TransactOpts) error {
		_, err := c.contract.Stdin(opts, clientID, msgID, nil, true)
		return err
	}); err != nil {
		return nil, nil, err
	}

	cleanup := []func(){}
	defer func() {
		for _, fn := range cleanup {
			fn()
		}
	}()

	stdoutRd, stdoutWr := io.Pipe()
	stdoutCh := make(chan *tanshell.TanshellStdout)
	stdoutSub, err := c.contract.WatchStdout(nil, stdoutCh, []*big.Int{clientID}, []*big.Int{msgID})
	if err != nil {
		return nil, nil, err
	}
	cleanup = append(cleanup, stdoutSub.Unsubscribe)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer stdoutWr.Close()
		for {
			select {
			case err := <-stdoutSub.Err():
				if err == nil {
					continue
				}
				fmt.Fprintln(os.Stderr, "error in subscription", err)
				return
			case <-c.ctx.Done():
				stdoutSub.Unsubscribe()
				return
			case log := <-stdoutCh:
				if _, err := stdoutWr.Write(log.Stream); err != nil {
					fmt.Fprintln(os.Stderr, "error writing to pipe", err)
				}
				if log.Eof {
					return
				}
			}
		}
	}()
	stderrRd, stderrWr := io.Pipe()
	stderrCh := make(chan *tanshell.TanshellStderr)
	stderrSub, err := c.contract.WatchStderr(nil, stderrCh, []*big.Int{clientID}, []*big.Int{msgID})
	if err != nil {
		return nil, nil, err
	}
	cleanup = append(cleanup, stderrSub.Unsubscribe)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case err := <-stderrSub.Err():
				if err == nil {
					continue
				}
				fmt.Fprintln(os.Stderr, "error in subscription", err)
				stderrWr.CloseWithError(err)
				return
			case <-c.ctx.Done():
				stderrSub.Unsubscribe()
				stderrWr.CloseWithError(io.EOF)
				return
			case log := <-stderrCh:
				if _, err := stderrWr.Write(log.Stream); err != nil {
					fmt.Fprintln(os.Stderr, "error writing to pipe", err)
				}
				if log.Eof {
					stderrWr.CloseWithError(io.EOF)
					return
				}
			}
		}
	}()
	cleanup = nil
	return stdoutRd, stderrRd, nil
}

// Listen to server's command.
func (c *Client) Listen(commandCh chan<- *Command) error {
	ID, err := c.contract.ClientID(&bind.CallOpts{
		Context: c.ctx,
	}, c.address)
	if err != nil {
		return err
	}
	messageChunks := make(map[uint64][]byte)
	ch := make(chan *tanshell.TanshellStdin)
	sub, err := c.contract.WatchStdin(nil, ch, []*big.Int{ID}, nil)
	if err != nil {
		return err
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case err := <-sub.Err():
				if err == nil {
					continue
				}
				fmt.Fprintln(os.Stderr, "error in subscription", err)
				// TODO: error handling
				return
			case <-c.ctx.Done():
				sub.Unsubscribe()
				return
			case log := <-ch:
				msgID := log.MsgId.Uint64()
				messageChunks[msgID] = append(messageChunks[msgID], log.Stream...)
				if log.Eof {
					stdoutCh := make(chan []byte)
					stderrCh := make(chan []byte)
					c.handleStdout(stdoutCh, log.MsgId)
					c.handleStderr(stderrCh, log.MsgId)
					commandCh <- &Command{
						ID:     msgID,
						CMD:    messageChunks[msgID],
						Stdout: stdoutCh,
						Stderr: stderrCh,
					}
					delete(messageChunks, msgID)
				}
			}
		}
	}()
	return nil
}

func (c *Client) handleStdout(ch chan []byte, msgID *big.Int) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case <-c.ctx.Done():
				close(ch)
				return
			case stream, ok := <-ch:
				if !ok {
					if err := c.call(c.ctx, func(opts *bind.TransactOpts) error {
						_, err := c.contract.Stdout(opts, msgID, nil, true)
						return err
					}); err != nil {
						fmt.Fprintln(os.Stderr, "error sending stdout", err)
						return
					}
					return
				}
				if err := c.call(c.ctx, func(opts *bind.TransactOpts) error {
					_, err := c.contract.Stdout(opts, msgID, stream, false)
					return err
				}); err != nil {
					fmt.Fprintln(os.Stderr, "error sending stdout", err)
				}
			}
		}
	}()
}

func (c *Client) handleStderr(ch chan []byte, msgID *big.Int) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case <-c.ctx.Done():
				close(ch)
				return
			case stream, ok := <-ch:
				if !ok {
					if err := c.call(c.ctx, func(opts *bind.TransactOpts) error {
						_, err := c.contract.Stderr(opts, msgID, nil, true)
						return err
					}); err != nil {
						fmt.Fprintln(os.Stderr, "error sending stderr", err)
						return
					}
					return
				}
				if err := c.call(c.ctx, func(opts *bind.TransactOpts) error {
					_, err := c.contract.Stderr(opts, msgID, stream, false)
					return err
				}); err != nil {
					fmt.Fprintln(os.Stderr, "error sending stderr", err)
				}
			}
		}
	}()
}
