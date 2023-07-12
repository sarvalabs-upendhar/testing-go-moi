package common

import (
	"encoding/json"
	"log"
	"os"

	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/common/hexutil"
)

type Artifact struct {
	Name     string        `json:"name"`
	Callsite string        `json:"callsite"`
	Calldata hexutil.Bytes `json:"calldata"`
	Manifest hexutil.Bytes `json:"manifest"`
}

func ReadArtifactFile(path string) (*Artifact, error) {
	ar := new(Artifact)

	file, err := os.ReadFile(path)
	if err != nil {
		log.Println("path : ", path)

		return nil, errors.New("error reading artifact file")
	}

	if err = json.Unmarshal(file, ar); err != nil {
		return nil, errors.New("error unmarshalling into artifact")
	}

	return ar, nil
}
