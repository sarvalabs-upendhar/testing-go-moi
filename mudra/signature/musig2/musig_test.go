package musig2

import (
	"errors"
	"sync"
	"testing"

	log "github.com/sirupsen/logrus"
	"gitlab.com/sarvalabs/btcd-musig/btcec/schnorr/musig2"
	"gitlab.com/sarvalabs/moichain/mudra/common"
)

func testMusigForNnodes(nodesInICS int) error {
	signerKeys, pubKeySet := getSetOfKeys(nodesInICS)

	sortedPubKeySet, err := GetSortedKeySet(pubKeySet)
	if err != nil {
		return errors.New("unable to sort and parse public keys: " + err.Error())
	}

	aggPubKey := GetAggregatedPublicKey(sortedPubKeySet)

	// Now that we have all the signers, we will initiate a new session at every node/signer
	signerSessions := make([]*musig2.Session, nodesInICS)

	for i, signerKey := range signerKeys {
		sessionAtNode, err := InitMusigSession(signerKey, sortedPubKeySet)
		if err != nil {
			return errors.New("unable to generate context: " + err.Error())
		}

		signerSessions[i] = sessionAtNode
	}

	allPublicNonces := make([][66]byte, nodesInICS)
	// Exchange public nonce as part of Pre-vote Round
	var wg sync.WaitGroup

	for i, signCtx := range signerSessions {
		signCtx := signCtx

		wg.Add(1)

		allPublicNonces[i] = signCtx.PublicNonce()

		go func(idx int, sessionAtOneNode *musig2.Session) {
			defer wg.Done()

			for j, otherNodesSession := range signerSessions {
				if idx == j {
					continue
				}

				// Getting public nonce from another nodes
				nonce := otherNodesSession.PublicNonce()
				// TODO: Sign the tesseract along with above public nonce and sent to other node

				// TODO: Verify the signature with corresponding public key before registering the nonce to session
				haveAll, err := sessionAtOneNode.RegisterPubNonce(nonce)
				if err != nil {
					log.Fatal("unable to add public nonce")
				}

				if j == len(signerSessions)-1 && !haveAll {
					log.Fatal("all public nonces should have been detected")
				}
			}
		}(i, signCtx)
	}

	wg.Wait()

	//fmt.Println("\nDone with Pre vote round")
	//fmt.Println("Now all session have all the public nonce from other nodes in ICS")

	msg := "I'm the tesseract data"

	var bytes32Message [32]byte

	copy(bytes32Message[:], common.GetKeccak256Hash([]byte(msg)))

	// Exchange partial signatures as part of Pre-commit Round
	preCommitSignatures := make([][]byte, nodesInICS)

	for i := range signerSessions {
		partialSig, err := signerSessions[i].Sign(bytes32Message)
		if err != nil {
			return errors.New("unable to generate partial sig:" + err.Error())
		}

		preCommitSignatures[i] = UnmarshalPartialSig(partialSig)
	}
	//fmt.Println("\nDone with Pre Commit round")
	//fmt.Println("Now every node have other's partial signature")

	// After Pre-commit round, check for aggregated signature
	// Here we are doing for first node as example

	//fmt.Println("\nVerifying each partial signatures and Aggregating them at NODE 1")
	// Verify Partial Signature
	sessionOfNode1 := signerSessions[0]

	partialSigOfNode1, err := MarshalToPartialSig(preCommitSignatures[0])
	if err != nil {
		return errors.New("error in verifying partial signature")
	}

	combinedNonce, err := musig2.AggregateNonces(allPublicNonces)
	if err != nil {
		return errors.New("unable to aggregate public nonces")
	}

	isValid := partialSigOfNode1.Verify(allPublicNonces[0],
		combinedNonce, sortedPubKeySet, sortedPubKeySet[0], bytes32Message)

	if isValid {
		preCommitSigExceptNode1 := preCommitSignatures[1:]
		for i := range preCommitSigExceptNode1 {
			partialSigOfOtherNode, err := MarshalToPartialSig(preCommitSigExceptNode1[i])
			if err != nil {
				return errors.New("error in verifying partial signature")
			}

			_, err = sessionOfNode1.CombineSig(partialSigOfOtherNode)
			if err != nil {
				return errors.New("unable to combine partial sig:" + err.Error())
			}
		}
	}

	//fmt.Println("\nVerifying Aggregated signature")
	finalSig := sessionOfNode1.FinalSig()
	if !finalSig.Verify(bytes32Message[:], aggPubKey) {
		return errors.New("final sig is invalid")
	}

	return nil
}

func TestMusig(t *testing.T) {
	if err := testMusigForNnodes(8); err != nil {
		t.Fatalf("%v", err)
	}
}

func BenchmarkMusigOf3Nodes(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = testMusigForNnodes(3)
	}
}
