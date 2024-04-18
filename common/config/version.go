package config

import (
	"fmt"

	"github.com/libp2p/go-libp2p/core/protocol"
)

const (
	VersionMajor = 0 // Major version component of the current release
	VersionMinor = 7 // Minor version component of the current release
	VersionPatch = 2 // Patch version component of the current release
)

var ProtocolVersion = func() string {
	return fmt.Sprintf("%d.%d.%d", VersionMajor, VersionMinor, VersionPatch)
}()

var (
	ICSProtocolDirectStream = protocol.ID("moi/ics/direct/stream/" + ProtocolVersion)
	ICSProtocolMeshStream   = protocol.ID("moi/ics/mesh/stream/" + ProtocolVersion)
	ICSProtocolRPC          = protocol.ID("moi/ics/rpc/" + ProtocolVersion)
)

var (
	SyncProtocolStream = protocol.ID("moi/sync/stream/" + ProtocolVersion)
	SyncProtocolRPC    = protocol.ID("moi/sync/rpc/" + ProtocolVersion)
)

var (
	AgoraProtocolStream = protocol.ID("moi/agora/stream/" + ProtocolVersion)
	AgoraProtocolRPC    = protocol.ID("moi/agora/rpc/" + ProtocolVersion)
)

var (
	MOIProtocolStream = protocol.ID("moi/core/stream/" + ProtocolVersion)
	MOIProtocolRPC    = protocol.ID("moi/core/rpc/" + ProtocolVersion)
)

var MOIPingStream = protocol.ID("moi/ping/stream/" + ProtocolVersion)

var (
	HelloTopic     = fmt.Sprintf("MOI_PUBSUB_HELLO_%s", ProtocolVersion)
	TesseractTopic = fmt.Sprintf("MOI_PUBSUB_TESSERACT_%s", ProtocolVersion)
)
