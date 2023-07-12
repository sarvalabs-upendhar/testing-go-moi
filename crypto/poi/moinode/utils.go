package moinode

import (
	"bytes"
	hexutil "encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/sarvalabs/go-moi/common/kramaid"
)

// MoiNodeRegistry Holds the baseURL of the RPC/HTTP host
type MoiNodeRegistry string

// String converts MoiNodeRegistry to string
func (mnR MoiNodeRegistry) String() string {
	return string(mnR)
}

// Init makes the health check for  MoiNodeRegistry and ready to accept/do requests
func Init(moiIDHost string) MoiNodeRegistry {
	currentBaseURL := MoiNodeRegistry(moiIDHost)

	return currentBaseURL
}

/*
GetNodes used to fetch nodes in MoiNet

options:

	userID: <user_moi_id_address>
	nodeType: NodeType

returns a list of nodes in MOINET with given options
*/
func (mnR *MoiNodeRegistry) GetNodes(optArgs map[string]interface{}) ([]MoiNode, uint32, error) {
	var targetedUserID string

	if optArgs["userID"] != nil {
		givenUserID, ok := (optArgs["userID"]).(string)
		if !ok {
			return nil, 0, errors.New("error in type assertion")
		}

		targetedUserID = givenUserID
	} else {
		return nil, 0, errors.New("user id cannot to nil")
	}

	var requestURL string

	isCountOnlyRequest := false

	if optArgs["countOnly"] != nil {
		givenCountOnly, ok := optArgs["countOnly"].(bool)
		if !ok {
			return nil, 0, errors.New("countOnly cannot to nil")
		}

		isCountOnlyRequest = givenCountOnly
	}

	if isCountOnlyRequest {
		requestURL = strings.Join([]string{
			mnR.String(),
			"/moi-id/moinode/list?userID=", targetedUserID, "&countOnly=true",
		}, "")
	} else {
		requestURL = strings.Join([]string{mnR.String(), "/moi-id/moinode/list?userID=", targetedUserID}, "")
	}

	log.Println("Request URL : ", requestURL)

	allNodesResponse, err := http.Get(requestURL) //nolint
	if err != nil {
		return nil, 0, err
	}

	allNodesResponseInBytes, err := io.ReadAll(allNodesResponse.Body)
	if err != nil {
		return nil, 0, err
	}

	if isCountOnlyRequest {
		var countResponse map[string]uint32
		err = json.Unmarshal(allNodesResponseInBytes, &countResponse)

		if err != nil {
			return nil, 0, err
		}

		return nil, countResponse["totalNodesCount"], nil
	}

	var targetedNodes, finalListOfNodes []MoiNode

	err = json.Unmarshal(allNodesResponseInBytes, &targetedNodes)
	if err != nil {
		return nil, 0, err
	}

	if optArgs["nodeType"] == nil {
		finalListOfNodes = targetedNodes
	} else {
		targetedNodeType, ok := (optArgs["nodeType"]).(MoiNodeType)
		if !ok {
			return nil, 0, errors.New("countOnly cannot to nil")
		}

		targetedByteString := targetedNodeType.ByteString()
		for _, _moiNode := range targetedNodes {
			if strings.EqualFold(_moiNode.NodeType, targetedByteString) {
				finalListOfNodes = append(finalListOfNodes, _moiNode)
			}
		}
	}

	return finalListOfNodes, 0, nil
}

// UpdateNode registers/upgrades the NODE in MoiNet
func (mnR *MoiNodeRegistry) UpdateNode(upgradeBool bool,
	nodeSpecificPublicBytes []byte,
	userID, requestedNodeType,
	kramaID string,
) (bool, error) {
	/* Getting the Node co-ordinates */
	ipInfo, err := http.Get("https://ipinfo.io")
	if err != nil {
		return false, err
	}

	ipInfoInBytes, err := io.ReadAll(ipInfo.Body)
	if err != nil {
		return false, err
	}

	var ipInfoObj map[string]string

	err = json.Unmarshal(ipInfoInBytes, &ipInfoObj)
	if err != nil {
		return false, err
	}

	coords := strings.Split(ipInfoObj["loc"], ",")

	fmt.Println("isUpgradeRequest: ", upgradeBool)
	fmt.Println("Node Public key: ", nodeSpecificPublicBytes)
	fmt.Println("Owner id: ", userID)
	fmt.Println("Type of Node: ", requestedNodeType)
	fmt.Println("New kramaID", kramaID)

	nodeSpecificPublicBytesInHex := hexutil.EncodeToString(nodeSpecificPublicBytes)

	// constructing the request payload
	payload, err := json.Marshal(map[string]interface{}{
		"isUpgradeRequest": upgradeBool,
		"nodePublicKey":    "0x" + nodeSpecificPublicBytesInHex,
		"ownerMoiID":       "0x" + userID,
		"typeOfNode":       requestedNodeType,
		"kramaID":          kramaID,
		"coOrdinates":      coords,
		"extraData":        "0x",
	})
	if err != nil {
		return false, err
	}

	requestBody := bytes.NewBuffer(payload)
	updateNodeReqURL := strings.Join([]string{mnR.String(), "/moi-id/moinode/update"}, "")

	// Making a call to register/upgrade the node
	resp, err := http.Post(updateNodeReqURL, "application/json", requestBody) //nolint
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	if resp.StatusCode == 200 {
		fmt.Print("\n\n")
		fmt.Println("Registration successful: ", string(body))
	} else {
		return false, errors.New(string(body))
	}

	return true, nil
}

// GetNodePublicKey used to fetch publicKey of node in MoiNet
func (mnR *MoiNodeRegistry) GetNodePublicKey(nodeKramaID kramaid.KramaID) ([]byte, error) {
	userMoiID, err := nodeKramaID.MoiID()
	if err != nil {
		return nil, errors.New("user moi id address cannot be empty")
	}

	nodeIndex, err := nodeKramaID.NodeIndex()
	if err != nil {
		return nil, err
	}

	getNodeInfoPayload, err := json.Marshal(map[string]interface{}{
		"uid":       "0x" + userMoiID,
		"nodeIndex": nodeIndex,
	})
	if err != nil {
		return nil, err
	}

	requestBody := bytes.NewBuffer(getNodeInfoPayload)
	updateNodeReqURL := strings.Join([]string{mnR.String(), "/moi-id/moinode/getnodeinfo"}, "")

	// Making a call to register/upgrade the node
	resp, err := http.Post(updateNodeReqURL, "application/json", requestBody) //nolint
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	tarMoiNode := new(MoiNode)
	if resp.StatusCode == 200 {
		err = json.Unmarshal(body, &tarMoiNode)
		if err != nil {
			return nil, err
		}

		tarPubKey, err := hexutil.DecodeString(tarMoiNode.NodePublicKey[2:])
		if err != nil {
			return nil, err
		}

		return tarPubKey, nil
	} else {
		return nil, errors.New(string(body))
	}
}
