package poi

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/crypto/common"
)

func TestGetKeystore(t *testing.T) {
	// Temp folder
	datadir1, err := ioutil.TempDir("", "testDataDir")
	require.NoError(t, err)

	// Validator 1 Init
	_, _, err = RandGenKeystore(datadir1, "nodepass1")
	require.NoError(t, err)

	_, err = GetKeystore(datadir1)
	require.Equal(t, nil, err)

	// Temp folder
	datadir2, err := ioutil.TempDir("", "testDataDir")
	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(datadir1)
		os.RemoveAll(datadir2)
	})

	_, err = GetKeystore(datadir2)
	require.Equal(t, err, common.ErrNoKeystore)
}
