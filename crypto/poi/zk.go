package poi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type zkAuthProof struct {
	Nonce    string      `json:"nonce"`
	Protocol interface{} `json:"protocol"`
	Curve    interface{} `json:"curve"`
	Hashfn   interface{} `json:"hashfn"`
	C        string      `json:"c"`
	S        string      `json:"s"`
}

const ZkAuthJSScript = "try { const cmdArgs = process.argv.splice(2); const zkPackagePath = cmdArgs[0]; " +
	"const zkChallenge = JSON.parse(cmdArgs[1]); const passPhrase = cmdArgs[2]; let nuid = require(zkPackagePath); " +
	"let oneTimeChallenge = nuid.challengeFromCredential(zkChallenge['_proof']); " +
	"let generatedProof = nuid.proofFromSecret(oneTimeChallenge, passPhrase); " +
	"const authStatus = nuid.proofIsVerified(generatedProof); let aToken = generatedProof; delete aToken['pub']; " +
	"delete aToken['keyfn']; let authToken = { nonce: aToken.nonce, protocol: { id: aToken.protocol.id }, " +
	"curve: { id: aToken.curve.id }, hashfn: { id: aToken.hashfn.id, " +
	"'normalization-form': aToken.hashfn['normalization-form'] }, c: aToken.c, s: aToken.s }; " +
	"console.log(JSON.stringify({ authStatus, authToken })); }catch(e) { console.log(e) }"

func deleteAuthTempFile() error {
	// remove a single file
	if err := os.Remove("auth.js"); err != nil {
		return err
	}

	return nil
}

// Authenticate will authenticate the user using zero-knowledge proofs
func Authenticate(defAddr, passPhrase, moiIDBaseURL string) (bool, *zkAuthProof, error) {
	// Finding node installation path
	nodePathShell := exec.Command("npm", "config", "get", "prefix")

	nodePath, err := nodePathShell.Output()
	if err != nil {
		return false, nil, err
	}

	// getting zk proof of user defAddr
	getZKProofPayload, err := json.Marshal(map[string]string{
		"defAddr":     defAddr,
		"typeOfProof": "zk",
	})
	if err != nil {
		return false, nil, err
	}

	requestBody := bytes.NewBuffer(getZKProofPayload)

	zkChallengeResp, err := http.Post(moiIDBaseURL+"/moi-id/auth/getmks", "application/json", requestBody)
	if err != nil {
		return false, nil, err
	}
	defer zkChallengeResp.Body.Close()

	zkChallengeRespInBytes, err := io.ReadAll(zkChallengeResp.Body)
	if err != nil {
		return false, nil, err
	}

	// Creating auth.js
	f, err := os.Create("auth.js")
	if err != nil {
		fmt.Println("error creating auth.js", err)

		return false, nil, err
	}

	bytesWritten, err := f.Write([]byte(ZkAuthJSScript))
	if err != nil {
		f.Close()

		return false, nil, err
	}

	if bytesWritten > 0 {
		err = f.Close()
		if err != nil {
			return false, nil, err
		}
	}

	// constructing zk.js exec
	// node auth.js <path to <@nuid/zk node package> <zkchallenge JSON stringified object> <password>
	command := "node"
	fileToBeExecuted := "auth.js"
	zkChallenge := string(zkChallengeRespInBytes)
	packagePath := strings.Trim(string(nodePath), "\n")
	packagePath += "/lib/node_modules/@nuid/zk"

	authCmd := exec.Command(command, fileToBeExecuted, packagePath, zkChallenge, passPhrase)

	authResponse, err := authCmd.Output()
	if err != nil {
		er2 := deleteAuthTempFile()
		if er2 != nil {
			return false, nil, errors.New("problem deleting temp auth.js and " + err.Error())
		}

		return false, nil, errors.New("failed executing auth.js: " + err.Error())
	}

	fmt.Println("\n\n Authentication token", string(authResponse))

	if err = deleteAuthTempFile(); err != nil {
		return false, nil, errors.New("error deleting temp auth.js " + err.Error())
	}

	// capturing response from auth.js from stdout
	type resFromAuth struct {
		AuthStatus bool        `json:"authStatus"`
		AuthToken  zkAuthProof `json:"authToken"`
	}

	var tempResponseFromExec resFromAuth
	err = json.Unmarshal(authResponse, &tempResponseFromExec)

	if err != nil {
		return false, nil, err
	}

	return tempResponseFromExec.AuthStatus, &tempResponseFromExec.AuthToken, nil
}
