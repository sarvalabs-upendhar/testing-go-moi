package poi

import (
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetKeystore(t *testing.T) {

	// Temp folder
	datadir1, err := ioutil.TempDir("", "testDataDir")
	if err != nil {
		log.Fatal(err)
	}

	// Validator 1 Init
	_, _, err = RandGenKeystore(datadir1, "nodepass1")
	if err != nil {
		log.Panicln(err)
	}

	_, err = GetKeystore(datadir1)
	require.Equal(t, nil, err)

	// Temp folder
	datadir2, err := ioutil.TempDir("", "testDataDir")
	if err != nil {
		log.Fatal(err)
	}

	defer os.RemoveAll(datadir1)
	defer os.RemoveAll(datadir2)

	_, err = GetKeystore(datadir2)
	require.Equal(t, err.Error(), "no keystore at given datadir")

}
