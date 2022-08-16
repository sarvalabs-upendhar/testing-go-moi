package poi

import (
	"fmt"
	"strings"
	"testing"
)

func TestGetKeystore(t *testing.T) {
	_, err := GetKeystore("/Users/sarvatechdeveloper1/.moi/moinode111")
	if err != nil {
		fmt.Println(strings.Contains(err.Error(), "no such file or directory"))
		t.Fatalf("%v", err)
	}
}
