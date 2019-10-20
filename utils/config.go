package utils

import (
	"encoding/json"
	"io/ioutil"
	"os"
)

// Config of a tan shell client.
type Config struct {
	Endpoint        string `json:"endpoint"`
	ContractAddress string `json:"contract_address"`
	PrivateKey      string `json:"private_key"`
}

// ReadConfigFromFile and returns config structure.
func ReadConfigFromFile(filename string) (*Config, error) {
	fd, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadAll(fd)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
