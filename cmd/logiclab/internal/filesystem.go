package internal

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// ErrMissingFile is a special error signal used when an item file cannot be found at
// its expected path. The handling function may employ custom handling for this situation
var ErrMissingFile = errors.New("file missing")

// inventoryFilename generates the filename for the inventory object
func inventoryFilename(labdir string) string {
	return filepath.Join(labdir, "inventory.json")
}

// participantFilename generates the filename for a participant object
func participantFilename(labdir, username string) string {
	return filepath.Join(labdir, fmt.Sprintf("participant-%v.json", username))
}

// logicFilename generates the filename for a logic object
func logicFilename(labdir, name string) string {
	return filepath.Join(labdir, fmt.Sprintf("logic-%v.json", name))
}

// pathExists confirms if a directory/file exists at the given path
func pathExists(path string) bool {
	_, err := os.Stat(path)

	return !os.IsNotExist(err)
}

// createDir creates a directory at the given path.
// It is a no-op if the directory already exists.
func createDir(dirpath string) error {
	return os.MkdirAll(dirpath, 0o755)
}

// deleteFile removes a file at the given path
func deleteFile(filename string) error {
	return os.Remove(filename)
}

// Storable is type constraint for objects that
// can be stored and retrieved from a file
type Storable interface {
	*Inventory | *ParticipantState | *LogicAccountState
}

// loadFile stores a Storable object to a file at the given path
func loadFile[S Storable](filename string, object S) error {
	// Check if the file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return ErrMissingFile
	}

	// Read the contents of the file
	encoded, err := ioutil.ReadFile(filename)
	if err != nil {
		return errors.Wrap(err, "failed to read file")
	}

	// Decode the file contents into the object
	if err = json.Unmarshal(encoded, object); err != nil {
		return errors.Wrap(err, "failed to decode file")
	}

	return nil
}

// saveFile retrieves a Storable object from a file at the given path
func saveFile[S Storable](filename string, object S) error {
	// Encode the participant object
	encoded, _ := json.MarshalIndent(object, "", "\t")

	// Write the encoded data to the file location.
	if err := ioutil.WriteFile(filename, encoded, 0o600); err != nil {
		return errors.Wrap(err, "failed to write file")
	}

	return nil
}
