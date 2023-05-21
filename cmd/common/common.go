package common

import (
	"log"
	"os"
)

type Instance struct {
	KramaID      string `json:"krama_id"`
	RPCUrl       string `json:"rpc_url"`
	ConsensusKey string `json:"consensus_key"`
}

func Err(err error) {
	if err != nil {
		log.Println("Error starting MOIPOD", err)
		os.Exit(1)
	}
}
