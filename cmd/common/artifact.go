package common

import (
	"encoding/json"
	"os"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common/hexutil"
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
		return nil, errors.New("error reading artifact file")
	}

	if err = json.Unmarshal(file, ar); err != nil {
		return nil, errors.New("error unmarshalling into artifact")
	}

	return ar, nil
}

type AssetArtifact struct {
	Standard hexutil.Uint16 `json:"standard"`
	Manifest hexutil.Bytes  `json:"manifest"`
}

type AssetArtifacts []AssetArtifact

func ReadAssetArtifactFile(path string) (AssetArtifacts, error) {
	assetArtifacts := make(AssetArtifacts, 0)

	file, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("error reading asset artifact file")
	}

	if err = json.Unmarshal(file, &assetArtifacts); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling into asset artifact")
	}

	return assetArtifacts, nil
}
