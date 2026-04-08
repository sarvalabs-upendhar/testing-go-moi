package bgclient

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"os/exec"
	"strings"
)

//nolint:unused
func findLineAfterKeyword(lines []string, word string) string {
	var helloOutput string

	for _, line := range lines {
		if strings.Contains(line, word) {
			index := strings.Index(line, ":")
			helloOutput = line[index+2:] // Your Bootstrap ID Is: /ip, we need to extract /ip from that string

			break
		}
	}

	return helloOutput
}

// runCommand executes command with given arguments
func runCommand(binary string, args []string, stdout io.Writer) error {
	var stdErr bytes.Buffer

	cmd := exec.Command(binary, args...)
	cmd.Stderr = &stdErr
	cmd.Stdout = stdout

	if err := cmd.Run(); err != nil {
		if stdErr.Len() > 0 {
			return fmt.Errorf("failed to execute command: %s", stdErr.String())
		}

		return fmt.Errorf("failed to execute command: %w", err)
	}

	return nil
}

func getRandomUpperCaseString(length int) (string, error) {
	const characters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	randomString := make([]byte, length)

	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(characters))))
		if err != nil {
			return "", err
		}

		randomString[i] = characters[num.Int64()]
	}

	return string(randomString), nil
}
