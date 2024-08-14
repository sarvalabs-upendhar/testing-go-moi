package p2p

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/network/message"
	"github.com/stretchr/testify/require"
)

func TestVerifyHelloMsg(t *testing.T) {
	kramaID := tests.RandomKramaID(t, 0)

	h := NewSubHandler(kramaID, hclog.NewNullLogger(), nil, nil, nil,
		&utils.TypeMux{}, nil, nil, false)
	helloMsg := createSignedHelloMsg(t)

	testcases := []struct {
		name          string
		msg           *message.HelloMsg
		expectedError error
	}{
		{
			name: "invalid krama id",
			msg: &message.HelloMsg{
				KramaID: "",
			},
			expectedError: errors.New("Failed to get peer id from krama id"),
		},
		{
			name: "Signature verification failed",
			msg: &message.HelloMsg{
				KramaID:   tests.RandomKramaID(t, 1),
				Signature: helloMsg.Signature,
			},
			expectedError: errors.New("Signature verification failed"),
		},
		{
			name: "Signature verification successful",
			msg: &message.HelloMsg{
				KramaID:   helloMsg.KramaID,
				Address:   helloMsg.Address,
				Signature: helloMsg.Signature,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := h.verifyHelloMsg(test.msg)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestReputationEngine_SenatusHandler(t *testing.T) {
	h := NewSubHandler(
		tests.RandomKramaID(t, 0),
		hclog.NewNullLogger(),
		nil,
		nil,
		nil,
		&utils.TypeMux{},
		nil,
		nil,
		false,
	)
	testcases := []struct {
		name              string
		message           *pubsub.Message
		expectedErr       error
		expectedQueueSize int
	}{
		{
			name: "pubsub message with invalid data",
			message: &pubsub.Message{
				Message: &pubsubpb.Message{
					Data: []byte{200},
				},
			},
			expectedErr: errors.New("malformed tag: varint terminated prematurely"),
		},
		{
			name: "pubsub message with valid data",
			message: &pubsub.Message{
				Message: &pubsubpb.Message{
					Data: getHelloMessage(t, utils.MultiAddrToString(tests.GetListenAddresses(t, 1)...)[0]),
				},
			},
			expectedQueueSize: 1,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			//	reputationEngine, _, _ := createTestReputationEngine(t)
			err := h.helloMsgHandler(test.message)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedQueueSize, h.pendingMessageQueue.Len())
		})
	}
}

func getHelloMessage(t *testing.T, addr string) []byte {
	t.Helper()

	hmsg := &message.HelloMsg{
		KramaID: tests.RandomKramaID(t, 1),
		Address: []string{addr},
	}

	data, err := hmsg.Bytes()
	require.NoError(t, err)

	return data
}
