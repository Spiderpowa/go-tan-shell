package contract

// Command from remote server.
type Command struct {
	ID     uint64
	CMD    []byte
	Stdout chan<- []byte
	Stderr chan<- []byte
}
