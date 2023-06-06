package test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"

	"github.com/pkg/errors"

	id "github.com/sarvalabs/moichain/mudra/kramaid"
)

type Request struct {
	ID  string `json:"kramaID"`
	Key string `json:"publicKey"`
}

func getThisNodeIP() (string, error) {
	// Retrieve the network interface addresses of the host machine
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		// Return the error
		return "", err
	}

	// Iterate over the network interface addresses
	for _, a := range addrs {
		// Check that the address is an IP address and not a loopback address
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			// Check if the IP address is an IPv4 address
			if ipnet.IP.To4() != nil {
				// Convert into a string IP address and store to variable
				return ipnet.IP.String(), nil
			}
		}
	}

	return "", errors.New("this node's ip not found")
}

func StoreKey(id id.KramaID, key []byte) error {
	data, err := json.Marshal(Request{ID: string(id), Key: hex.EncodeToString(key)})
	if err != nil {
		return err
	}

	res, err := http.Post("http://91.107.196.74/api/store", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		return errors.New("error storing the public key")
	}

	return nil
}
