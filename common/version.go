package common

import (
	"fmt"

	"github.com/libp2p/go-libp2p/core/protocol"
)

const (
	VersionMajor = 0 // Major version component of the current release
	VersionMinor = 0 // Minor version component of the current release
	VersionPatch = 1 // Patch version component of the current release
)

var ProtocolVersion = func() string {
	return fmt.Sprintf("%d.%d.%d", VersionMajor, VersionMinor, VersionPatch)
}()

var (
	FluxProtocolStream = protocol.ID("moi/flux/stream/" + ProtocolVersion)
	FluxProtocolRPC    = protocol.ID("moi/flux/rpc/" + ProtocolVersion)
)

var (
	ICSProtocolStream = protocol.ID("moi/ics/stream/" + ProtocolVersion)
	ICSProtocolRPC    = protocol.ID("moi/ics/rpc/" + ProtocolVersion)
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
