package common

import (
	"strconv"
	"time"

	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/crypto"
)

type Config struct {
	NodeType       int              `json:"node_type"`
	KramaIDVersion int              `json:"ḭd_version"`
	Vault          VaultConfig      `json:"vault"`
	Network        NetworkConfig    `json:"network"`
	Syncer         SyncerConfig     `json:"syncer"`
	IxPool         IxPoolConfig     `json:"ixpool"`
	Consensus      ConsensusConfig  `json:"consensus"`
	Execution      ExecutionConfig  `json:"execution"`
	DB             DBConfig         `json:"database"`
	Telemetry      Telemetry        `json:"telemetry"`
	LogFilePath    string           `json:"logfile"`
	JSONRPC        JSONRPCConfig    `json:"jsonrpc"`
	NetworkID      config.NetworkID `json:"network_id"`
	State          StateConfig      `json:"state"`
	GenesisTime    uint64           `json:"genesis_time"`
}

func DefaultBabylonConfig(path string) *Config {
	return &Config{
		NodeType:       7,
		KramaIDVersion: 1,
		Vault: VaultConfig{
			DataDir: path,
			Mode:    crypto.GuardianMode,
		},
		Network: NetworkConfig{
			Libp2pAddr: []string{
				"/ip4/0.0.0.0/tcp/" + strconv.Itoa(config.DefaultP2PPort),
				"/ip4/0.0.0.0/udp/" + strconv.Itoa(config.DefaultP2PPort) + "/quic-v1",
				"/ip6/::/tcp/" + strconv.Itoa(config.DefaultP2PPort),
				"/ip6/::/udp/" + strconv.Itoa(config.DefaultP2PPort) + "/quic-v1",
			},
			BootStrapPeers: []string{
				"/ip4/65.109.138.198/tcp/5000/p2p/16Uiu2HAmNPceqBKGNWXGTKTtWDPty4UhncdhB84VbDEPpn1H11Cb",
				"/ip4/135.181.206.93/tcp/5000/p2p/16Uiu2HAmFXiKHS3GWgdS1V36uUBDUjigf3RZRJCrjDFFMjexR3V8",
			},
			MaxPeers:           0, // current we don't limit the no.of peers
			InboundConnLimit:   config.DefaultInboundConnLimit,
			OutboundConnLimit:  config.DefaultOutboundConnLimit,
			MinimumConnections: config.DefaultMinimumConnections,
			MaximumConnections: config.DefaultMaximumConnections,
			AllowIPv6Addresses: false,
			DisablePrivateIP:   true,
			DiscoveryInterval:  config.DefaultDiscoveryInterval,
			JSONRPCAddr:        "0.0.0.0:" + strconv.Itoa(config.DefaultJSONRPCPort),
			CorsAllowedOrigins: []string{"*"},
			RefreshSenatus:     true,
		},
		Syncer: SyncerConfig{
			ShouldExecute: true,
			//nolint
			TrustedPeers: []PeerInfo{
				{
					ID:      "3Wz4nx3XTjxyNBABhNudKhFFSnBow5Rbh1ZRpP75sY9o56BLcM9y.16Uiu2HAm56BPrW52mcHBpKrCDxwNNxvGQsHzD7omnJHDUEyo2wAh",
					Address: "/ip4/167.235.70.122/udp/6000/quic-v1/p2p/16Uiu2HAm56BPrW52mcHBpKrCDxwNNxvGQsHzD7omnJHDUEyo2wAh",
				},
				{
					ID:      "3WyLs9iSAcrNyrQsJNuzY1GeufWjXCkb1jrSDh1p1rXEwK5ahSYf.16Uiu2HAm7R8Y9rKCzJozkY8xE4t9m3ip4GYJBEUpNx25RsAfPUcF",
					Address: "/ip4/5.75.185.184/udp/6000/quic-v1/p2p/16Uiu2HAm7R8Y9rKCzJozkY8xE4t9m3ip4GYJBEUpNx25RsAfPUcF",
				},
				{
					ID:      "3WyNekAU1hGS1yQM88hisSWxDeTqKK9yQKyPsSCfLqFMSCYXVNUF.16Uiu2HAmNira27KKSxGm3LypePmXUf2Pfp9LRVipRKyCq8TtvLrm",
					Address: "/ip4/128.140.40.214/udp/6000/quic-v1/p2p/16Uiu2HAmNira27KKSxGm3LypePmXUf2Pfp9LRVipRKyCq8TtvLrm",
				},
				{
					ID:      "3WzQ9wb1UpEdagSuEifUMJNKiMftKNQfKkPXSFrovaASWdoXqris.16Uiu2HAm7z3B21YAZKD5mA4vVVS3xctgR5D35w7JGucfDfz37ZeA",
					Address: "/ip4/5.75.165.143/udp/6000/quic-v1/p2p/16Uiu2HAm7z3B21YAZKD5mA4vVVS3xctgR5D35w7JGucfDfz37ZeA",
				},
				{
					ID:      "3WxRSx9PbKoJm3ip4Trgd93zHeRVmg3RR4SiqzwUuajdvUrqAXC3.16Uiu2HAmSo2iJfYt9fhkVQWq8QE862HskLFxBaNQoEuid2YBVEjF",
					Address: "/ip4/5.75.146.72/udp/6000/quic-v1/p2p/16Uiu2HAmSo2iJfYt9fhkVQWq8QE862HskLFxBaNQoEuid2YBVEjF",
				},
				{
					ID:      "3WwRmZZTmsBj9FXbCVbtSLgPhhToPeN9ByHNCvAqdCYrFtcfKwjM.16Uiu2HAm1JHmFrLDxCa7JTqcoVBprG9sbfR95vq4XcBUeyQzTw61",
					Address: "/ip4/116.203.28.200/udp/6000/quic-v1/p2p/16Uiu2HAm1JHmFrLDxCa7JTqcoVBprG9sbfR95vq4XcBUeyQzTw61",
				},
				{
					ID:      "3Wx4oVJjr27eQ6ZQX6XsoGLHSC4cWWUhL3gkfn5TFpLivFRCFKaX.16Uiu2HAmAVqSNvVXUhMcL51Gt1T4NDDCFJtE9VczTwXBsZkmoNtq",
					Address: "/ip4/159.69.144.151/udp/6000/quic-v1/p2p/16Uiu2HAmAVqSNvVXUhMcL51Gt1T4NDDCFJtE9VczTwXBsZkmoNtq",
				},
				{
					ID:      "3WwfBvxEWjbKUogWcpiNW6fzry4ZHyjRtxWpJVVLbCTthCPfNyE7.16Uiu2HAkxFaWbtmg6c92Rhntm7nbPe7Z2oCGS1YiLythnTVxe9HH",
					Address: "/ip4/78.47.143.239/udp/6000/quic-v1/p2p/16Uiu2HAkxFaWbtmg6c92Rhntm7nbPe7Z2oCGS1YiLythnTVxe9HH",
				},
				{
					ID:      "3WxJRiu31Xwis9SHt4qJNteDbLFp2q1WqUYiFSbKQ17LNv7zMFHy.16Uiu2HAmRVTo8GSz6AUe11ZDwSe3WB9tp8gPbq29pVJrChk7aYR3",
					Address: "/ip4/88.198.215.85/udp/6000/quic-v1/p2p/16Uiu2HAmRVTo8GSz6AUe11ZDwSe3WB9tp8gPbq29pVJrChk7aYR3",
				},
				{
					ID:      "3WynBwJ7fKRJscLQ3SZZHaBnzDukAcwYMv2b2ejexyrjWdZ9pF43.16Uiu2HAmUxUX8YQfkFfbPxdefrK6Fetbg4gkk9N9xtM1KNiptf9N",
					Address: "/ip4/23.88.98.167/udp/6000/quic-v1/p2p/16Uiu2HAmUxUX8YQfkFfbPxdefrK6Fetbg4gkk9N9xtM1KNiptf9N",
				},
				{
					ID:      "3WwyfJFGiFY98VauTw4WFJucHVZCk4Cwt8fJWSs6v7i8DHVexYUX.16Uiu2HAmGJBc1tQ7Y7dvBjFgnbmrV4UVVSkGN4oSDNP5Cd3NZhtc",
					Address: "/ip4/91.107.225.8/udp/6000/quic-v1/p2p/16Uiu2HAmGJBc1tQ7Y7dvBjFgnbmrV4UVVSkGN4oSDNP5Cd3NZhtc",
				},
				{
					ID:      "3WwJrjzPryiv3jwE4wK8vY4Z2Q4nhwWnu9xtkHToZtwkrpytYvw9.16Uiu2HAmGZJoUogh1Kaz4yug7RK3yncJkgJEKjVRX6YhWWiQG6xb",
					Address: "/ip4/128.140.40.84/udp/6000/quic-v1/p2p/16Uiu2HAmGZJoUogh1Kaz4yug7RK3yncJkgJEKjVRX6YhWWiQG6xb",
				},
				{
					ID:      "3Wvu4RS1DiZhwV5oDsZvoRcDtwYaNVk6usvpzck7wnHtwaUNmwPV.16Uiu2HAmGs8gzEeqLwLDhMjuTMbpthK2EeZFxKnE9zQiaMBxGM7a",
					Address: "/ip4/168.119.168.116/udp/6000/quic-v1/p2p/16Uiu2HAmGs8gzEeqLwLDhMjuTMbpthK2EeZFxKnE9zQiaMBxGM7a",
				},
				{
					ID:      "3WxRXXHbb7rdJBZt8ic1jnJTRhBDMBcxFqvnAcMtiNfkkfJdYBbm.16Uiu2HAmCFuAKedTqiGTqdqAccMZJDYPfagQJfqDubnXEZLh73yW",
					Address: "/ip4/167.235.51.161/udp/6000/quic-v1/p2p/16Uiu2HAmCFuAKedTqiGTqdqAccMZJDYPfagQJfqDubnXEZLh73yW",
				},
				{
					ID:      "3WwwGHsBFYQCQWwfFCcJvNSvdRrwMzNHNPZNARGag6Jcpm7Wfbhh.16Uiu2HAmLMuz2NJgivWk8KAMVJpBDTRhyEu7cP4umAQMUhhB6DcH",
					Address: "/ip4/116.203.126.89/udp/6000/quic-v1/p2p/16Uiu2HAmLMuz2NJgivWk8KAMVJpBDTRhyEu7cP4umAQMUhhB6DcH",
				},
				{
					ID:      "3WxnkCbg3rbMhJpynAp1o3TFhnLe43cVSxXUWeAVPnuXFnmmu3h9.16Uiu2HAmB1FZiUpDeQS5Hyt95C9HWKMLz5Ct3bUsQoWtg6yygsHi",
					Address: "/ip4/116.203.209.16/udp/6000/quic-v1/p2p/16Uiu2HAmB1FZiUpDeQS5Hyt95C9HWKMLz5Ct3bUsQoWtg6yygsHi",
				},
				{
					ID:      "3WxKanKGPGzS9rvgYUEvNsvWHvPJMEVPn7CRWMYVr89A4jRm5cCw.16Uiu2HAmTYcr2xefFn34iXmfCWBLhZfDhvopC43uJvdnYrzMnu6d",
					Address: "/ip4/5.75.172.35/udp/6000/quic-v1/p2p/16Uiu2HAmTYcr2xefFn34iXmfCWBLhZfDhvopC43uJvdnYrzMnu6d",
				},
				{
					ID:      "3WwxUbgCaSm1aRfFodqe9ctYSSFevc3zaJR2NXwyyVsoNvDAftzj.16Uiu2HAm5CmW3fyxfUaW3qpq9u5AKzvVjDU68bEhAJh8YwFhAhxb",
					Address: "/ip4/116.203.66.72/udp/6000/quic-v1/p2p/16Uiu2HAm5CmW3fyxfUaW3qpq9u5AKzvVjDU68bEhAJh8YwFhAhxb",
				},
				{
					ID:      "3Wwz5hhRJvHp2F3TWC2V7xiFqZnRifUnq1BsGukTb8NvVbApFbzF.16Uiu2HAmEgKrTfzY3C7jYcDWEUwdiFL9SKvgSj37CLwVYFSSNUFZ",
					Address: "/ip4/49.12.13.101/udp/6000/quic-v1/p2p/16Uiu2HAmEgKrTfzY3C7jYcDWEUwdiFL9SKvgSj37CLwVYFSSNUFZ",
				},
				{
					ID:      "3Wyg8o6PiYptLcAJgP4EXLgLcLZoWvGtWq4A7jfb3KaUahAheMDH.16Uiu2HAmVdC5qbMannLjtaSxqLjASWqeEpUryUVPcGYVnBpWQNPb",
					Address: "/ip4/168.119.53.127/udp/6000/quic-v1/p2p/16Uiu2HAmVdC5qbMannLjtaSxqLjASWqeEpUryUVPcGYVnBpWQNPb",
				},
				{
					ID:      "3WwkD36QwqQZ7c6AE37Z9jGbVDHtbJA35zqerE4pgbWyBR8sx6Aj.16Uiu2HAmSoGDAaa4nrYLXQ7P5sUXrD3dcxdhNNJoKPL48JodWMkb",
					Address: "/ip4/116.203.249.189/udp/6000/quic-v1/p2p/16Uiu2HAmSoGDAaa4nrYLXQ7P5sUXrD3dcxdhNNJoKPL48JodWMkb",
				},
				{
					ID:      "3WxCEKwe3rxQPAFr5at6Xnsq5qaP395KTbWL7FWSRM5VwYPBtKBV.16Uiu2HAmCTySsRNCfy88yGb668kT8NYzH7uB2Jfbffk4fJCXfrEk",
					Address: "/ip4/128.140.56.242/udp/6000/quic-v1/p2p/16Uiu2HAmCTySsRNCfy88yGb668kT8NYzH7uB2Jfbffk4fJCXfrEk",
				},
				{
					ID:      "3WvvC6guxrXbDfjUvrWDpJe8gkRBFtQnRktjkoq22hDQbj5Pp967.16Uiu2HAmTq7nUxTmT4RLv1J2Vi1RD9uHvfqvJteJZmDNPPTVv5wX",
					Address: "/ip4/91.107.196.146/udp/6000/quic-v1/p2p/16Uiu2HAmTq7nUxTmT4RLv1J2Vi1RD9uHvfqvJteJZmDNPPTVv5wX",
				},
				{
					ID:      "3Wwg3V1naHMZ7k4zsxJMutNRXqj4gShZ8L9oFgUcZTkvk4QDnaTH.16Uiu2HAmVGTw6x84ixyrssmkULsqoiWxbganFLNJ8kT7bgfAZiNg",
					Address: "/ip4/159.69.241.184/udp/6000/quic-v1/p2p/16Uiu2HAmVGTw6x84ixyrssmkULsqoiWxbganFLNJ8kT7bgfAZiNg",
				},
				{
					ID:      "3Wz9Hd4JSWWhhUJN1kEc251Z7xeazvvqxjGg5WuPpAsGMWcLzL1V.16Uiu2HAkx4PcFJXvHGFuGLgCC61CBMHRo9QsyxhQDPpUQnnbyQrh",
					Address: "/ip4/5.75.240.164/udp/6000/quic-v1/p2p/16Uiu2HAkx4PcFJXvHGFuGLgCC61CBMHRo9QsyxhQDPpUQnnbyQrh",
				},
				{
					ID:      "3WwHSw6zAAnUVokH54EUMG2g1i3aDVed96p4MzxC8DkCzSHgDTQK.16Uiu2HAm2ssMEMvUhmMtRdChnPYPrehAFVXifkTAGhK4ULRc6vtr",
					Address: "/ip4/49.13.6.63/udp/6000/quic-v1/p2p/16Uiu2HAm2ssMEMvUhmMtRdChnPYPrehAFVXifkTAGhK4ULRc6vtr",
				},
				{
					ID:      "3WyuvfLNNjyFT4MEGc7ztgv41mwB4kSe5xdpKeZEa3wHwfM7fuFu.16Uiu2HAm85feSFbaNHp4by1cXpFpXHCzbfj24gzShaXMcTXWBrsT",
					Address: "/ip4/23.88.42.39/udp/6000/quic-v1/p2p/16Uiu2HAm85feSFbaNHp4by1cXpFpXHCzbfj24gzShaXMcTXWBrsT",
				},
				{
					ID:      "3Wz4bgm3CXYb9mHo1X9GvfBNoNaGE6CeW4v6YEnBb5CfdpzNRdxB.16Uiu2HAmKUimCTgVU1jyeLxL2genyUH9bJnAN35b2bGjsAM4Mrur",
					Address: "/ip4/5.75.228.78/udp/6000/quic-v1/p2p/16Uiu2HAmKUimCTgVU1jyeLxL2genyUH9bJnAN35b2bGjsAM4Mrur",
				},
				{
					ID:      "3WwRrPqk3fUWLUxfwxyPfEFnSgeVi81e7e3woENmEHL3w8ZbtvWb.16Uiu2HAmUCQ96EQHZpui6Z9d7RGcT8U7ZWejkSrQDt7tguwNC8Ms",
					Address: "/ip4/5.75.226.199/udp/6000/quic-v1/p2p/16Uiu2HAmUCQ96EQHZpui6Z9d7RGcT8U7ZWejkSrQDt7tguwNC8Ms",
				},
				{
					ID:      "3WwvfxjoV8Q4hnaGfesokU5PDx1QcGSYSWeembBnoDFzzNF4MV27.16Uiu2HAkxJDmJscTtweJLXHNDQRvVQAiXgxKagD4kT6byixBDbZE",
					Address: "/ip4/168.119.109.0/udp/6000/quic-v1/p2p/16Uiu2HAkxJDmJscTtweJLXHNDQRvVQAiXgxKagD4kT6byixBDbZE",
				},
				{
					ID:      "3WzQzEBdL6Z26gFXvgHXgCDgbiLjkJCrhKjCjHGg6dQ7Dpt1pffM.16Uiu2HAm4c4jRfoW5rPaTPJ2Fxf6G41XSqsg1ZtsuZp7N8aqb77g",
					Address: "/ip4/157.90.157.88/udp/6000/quic-v1/p2p/16Uiu2HAm4c4jRfoW5rPaTPJ2Fxf6G41XSqsg1ZtsuZp7N8aqb77g",
				},
				{
					ID:      "3WzGUmGCNK4Gk5rVmevxrXG6CBKR7RQsKxVL95JNUzDxrXTPGK3D.16Uiu2HAm35WtwaFwvLEhk8D32hrrjL3A2CLrkTjojAznQs23gGxJ",
					Address: "/ip4/23.88.115.118/udp/6000/quic-v1/p2p/16Uiu2HAm35WtwaFwvLEhk8D32hrrjL3A2CLrkTjojAznQs23gGxJ",
				},
				{
					ID:      "3WxbCwz1YKNbWKNaib97b1fA3xd22J2fD1vB6aQKVEDHgBjesNxw.16Uiu2HAmRiPMCtgNAdsgtf1vFiv51kwA952dW3mExK6PaZEVzm61",
					Address: "/ip4/49.12.208.123/udp/6000/quic-v1/p2p/16Uiu2HAmRiPMCtgNAdsgtf1vFiv51kwA952dW3mExK6PaZEVzm61",
				},
				{
					ID:      "3WxTqAYdGbQ6aq8A3vVJgAY9yKPR5zNZrQYp8etqwKAs1zSD8KpT.16Uiu2HAmR9aAz7Vf8HtoqeSvkbKbpzFsK16xA2ow2FxCtdi8SFhJ",
					Address: "/ip4/162.55.171.14/udp/6000/quic-v1/p2p/16Uiu2HAmR9aAz7Vf8HtoqeSvkbKbpzFsK16xA2ow2FxCtdi8SFhJ",
				},
				{
					ID:      "3Wwz1gJRdf8NXDnGe83E9dz3ZKeCCzCkuoQAJjKteEQjZuNK2VH9.16Uiu2HAmUtgkbXpKHt6pMCLf9fFGb7w1avgKUhsAP4dbD3Kvofbc",
					Address: "/ip4/142.132.233.137/udp/6000/quic-v1/p2p/16Uiu2HAmUtgkbXpKHt6pMCLf9fFGb7w1avgKUhsAP4dbD3Kvofbc",
				},
				{
					ID:      "3WyMV9HQYzYkpHUDa1PNmH4ihw4Hqe4gzDBgpzD23h61RLHmajsV.16Uiu2HAm5rA4pnpbaWov3PGzVHbbGE73yeFx5SYYxGaErHJNheXe",
					Address: "/ip4/88.99.81.26/udp/6000/quic-v1/p2p/16Uiu2HAm5rA4pnpbaWov3PGzVHbbGE73yeFx5SYYxGaErHJNheXe",
				},
				{
					ID:      "3Wy615dV6BB8m5YsvsasyqLy81pLcZVZydjH3mCvm5BqFqHdPatX.16Uiu2HAmKdTR9QQZrmsss3GwVMpbCoW46bRfusLAYW8BuNB6eyjf",
					Address: "/ip4/65.21.191.10/udp/6000/quic-v1/p2p/16Uiu2HAmKdTR9QQZrmsss3GwVMpbCoW46bRfusLAYW8BuNB6eyjf",
				},
				{
					ID:      "3WxQutLbt693jgGbDWU1hUyWHGHLrD8yQ6imq2EaTiAbqWUgb1KD.16Uiu2HAmUCa7xhFfEFY6mRrJEMfdQPdKkccLLf7P7Ssh3T3ey3pu",
					Address: "/ip4/65.109.161.197/udp/6000/quic-v1/p2p/16Uiu2HAmUCa7xhFfEFY6mRrJEMfdQPdKkccLLf7P7Ssh3T3ey3pu",
				},
				{
					ID:      "3WxUsYdDmFTTkmEzLkLBLQGbJGzg94e6ryq2fd9vt8YL5pbontJK.16Uiu2HAmKvUbLVuWiKSazs7Z1zQjpMz5P73xo6a5MJHDGFsi7vZr",
					Address: "/ip4/95.217.14.244/udp/6000/quic-v1/p2p/16Uiu2HAmKvUbLVuWiKSazs7Z1zQjpMz5P73xo6a5MJHDGFsi7vZr",
				},
				{
					ID:      "3WxPiwixNCGtN7mJcbZJ1jtmj992GWZz2o2kWEcpbgMsMLjXAiXh.16Uiu2HAmJ5vhphUh5iMbZUWHurtB6HvW5p4EbBSQJt1heGz3wHoy",
					Address: "/ip4/65.21.157.9/udp/6000/quic-v1/p2p/16Uiu2HAmJ5vhphUh5iMbZUWHurtB6HvW5p4EbBSQJt1heGz3wHoy",
				},
				{
					ID:      "3WvqEsFjQt4kFdy1s3uZbDWkYjeYwYqnbW5aY9Y4Akqh2NtsqFpb.16Uiu2HAmQbERmmgwvThRqnRgNhDMZ26a8ffLxenspFy6dK9w2dr1",
					Address: "/ip4/65.108.88.233/udp/6000/quic-v1/p2p/16Uiu2HAmQbERmmgwvThRqnRgNhDMZ26a8ffLxenspFy6dK9w2dr1",
				},
				{
					ID:      "3WwwoKKR9XgpkMipqwC8nQa5ewzsT7HbzSKyQtjanjjcKqWDXcGX.16Uiu2HAmSYHZhxG1DMU4AKoNstF43iSYP9JQjYREgq351h3LyamM",
					Address: "/ip4/65.108.155.83/udp/6000/quic-v1/p2p/16Uiu2HAmSYHZhxG1DMU4AKoNstF43iSYP9JQjYREgq351h3LyamM",
				},
				{
					ID:      "3Wxo2yAD5qFzHpQyFX5aNg1Vpxx8uHDsa1gdAGqS5oxss6FHT5ZZ.16Uiu2HAmKX4ke53HsxofkwYxqqJz2d43bx2BvLQzjaumtMXjNiwx",
					Address: "/ip4/37.27.2.110/udp/6000/quic-v1/p2p/16Uiu2HAmKX4ke53HsxofkwYxqqJz2d43bx2BvLQzjaumtMXjNiwx",
				},
				{
					ID:      "3Wynq2K2P8JKrD27AY1SmNQH1A2By86MJ3xS6enR8EFtjkRwoXio.16Uiu2HAmMTkrdQTRyPZpjMWytZX9x4QzSEoH6rpew9xhCNCbhZC9",
					Address: "/ip4/65.21.48.209/udp/6000/quic-v1/p2p/16Uiu2HAmMTkrdQTRyPZpjMWytZX9x4QzSEoH6rpew9xhCNCbhZC9",
				},
				{
					ID:      "3WwEExxNSpGiS2yyX2zw5Twcijw3kTLrwdiJm5TsCXoUG9Ekoxfh.16Uiu2HAmEdNKRMze4nbA6KVodXyXH5VCTWfva7eYymAUfaqUM37V",
					Address: "/ip4/95.217.216.148/udp/6000/quic-v1/p2p/16Uiu2HAmEdNKRMze4nbA6KVodXyXH5VCTWfva7eYymAUfaqUM37V",
				},
				{
					ID:      "3WxbHSqfdwkmiCo6b32tW1Mheb7w9FiM7atK9tWTG82fi2txXZMy.16Uiu2HAmGtoo76vPa3ks3yeqb8kP4iB1M8ZB4B6mKHgeaY6nRugE",
					Address: "/ip4/65.21.250.156/udp/6000/quic-v1/p2p/16Uiu2HAmGtoo76vPa3ks3yeqb8kP4iB1M8ZB4B6mKHgeaY6nRugE",
				},
				{
					ID:      "3Wz4xNH5oKvSFhPnVH8arazRNLLP4F4ipBoRmUYeSiaYjzo9ucVV.16Uiu2HAmHeJwX2CAuD274yFmt7UyMQUCsW2Q4r5XteKTBbSYiYTr",
					Address: "/ip4/95.217.176.222/udp/6000/quic-v1/p2p/16Uiu2HAmHeJwX2CAuD274yFmt7UyMQUCsW2Q4r5XteKTBbSYiYTr",
				},
				{
					ID:      "3WxZdAWK9sfSw3EcAztE54gDeVgfapo7tn4uBxqED6jkitgxy6sh.16Uiu2HAmC3xD7oGBiTDMoNMqjeAHhzKYZLo8saXPhbfdnpdeS4qS",
					Address: "/ip4/65.109.160.208/udp/6000/quic-v1/p2p/16Uiu2HAmC3xD7oGBiTDMoNMqjeAHhzKYZLo8saXPhbfdnpdeS4qS",
				},
				{
					ID:      "3Wwk4ez7q3JMDs78q3gTDdQBZ7c3An4YciubsdcEnSpTu6W7RXFV.16Uiu2HAmESwTUEArJKduBgWVECxQCKzYRGSV4mJVV4hYCqTxaq9i",
					Address: "/ip4/65.21.186.91/udp/6000/quic-v1/p2p/16Uiu2HAmESwTUEArJKduBgWVECxQCKzYRGSV4mJVV4hYCqTxaq9i",
				},
				{
					ID:      "3WyjewqNMCqxZ9Pam8eB2ZjAusbrd3EHBq59WdojenZDLfhZwsoV.16Uiu2HAmN3qFV8JWF338g571CH7FSR4cy2EMt6WhVTX56Gvv9egz",
					Address: "/ip4/95.216.222.229/udp/6000/quic-v1/p2p/16Uiu2HAmN3qFV8JWF338g571CH7FSR4cy2EMt6WhVTX56Gvv9egz",
				},
				{
					ID:      "3Ww5Uvpv1qPyZu1LpZK88Hn88Ufn4YYvB5JVtmB7rtH9hWrsZCLf.16Uiu2HAmUEmob9gwKe9f6ephymDgQzCjn4b6Xi8NkRtmyGVPxpUR",
					Address: "/ip4/37.27.17.0/udp/6000/quic-v1/p2p/16Uiu2HAmUEmob9gwKe9f6ephymDgQzCjn4b6Xi8NkRtmyGVPxpUR",
				},
				{
					ID:      "3WxYJ6DuCYueHkhaqewwKDeaqXJG9pJ1AZYEFhFhESFrHTGbHeFZ.16Uiu2HAmJ8ZBRshqTuZPxGH2mgbuopa9borcTS2JHdxZ4ZcdmRCi",
					Address: "/ip4/95.216.190.174/udp/6000/quic-v1/p2p/16Uiu2HAmJ8ZBRshqTuZPxGH2mgbuopa9borcTS2JHdxZ4ZcdmRCi",
				},
				{
					ID:      "3Wz9smfiMrYUmN57D666GzeiMhWbhVVeTydD84Y3dbqDPtNA2uPV.16Uiu2HAmQmTMwAffMEg2Km1bSH8QSNmswsiBe14ju5vF2Q2UXEKh",
					Address: "/ip4/65.109.162.0/udp/6000/quic-v1/p2p/16Uiu2HAmQmTMwAffMEg2Km1bSH8QSNmswsiBe14ju5vF2Q2UXEKh",
				},
				{
					ID:      "3Wwg75ZYJ3TRCPHqx7LYDKrJMjnbjaTRPsnuT5hdu6isjivWoEHm.16Uiu2HAmNRvdrA72pr2H9rFDhkAEQkfj3PmzaLxbSy9BNcF3VaNL",
					Address: "/ip4/65.108.57.198/udp/6000/quic-v1/p2p/16Uiu2HAmNRvdrA72pr2H9rFDhkAEQkfj3PmzaLxbSy9BNcF3VaNL",
				},
				{
					ID:      "3WwMS8ASAdxGqe7owcZEKQ48L6wRemDnheLbr8MxohNi4yKBj6Fq.16Uiu2HAm6BCaZQ8pgQyxQs39Ys4dqXGVD9sxpdjVQ7Uxw34sXQkg",
					Address: "/ip4/5.78.105.3/udp/6000/quic-v1/p2p/16Uiu2HAm6BCaZQ8pgQyxQs39Ys4dqXGVD9sxpdjVQ7Uxw34sXQkg",
				},
				{
					ID:      "3Wx2E5L9fT1821TGsfX6Ju7NmBWmSMqByNuBiDqsrBp4q6ENgRyh.16Uiu2HAmBKCp84Nfyq9cJ88hGpgjuxMDJXWrtWnKX6UqCDef1Pxj",
					Address: "/ip4/5.78.65.177/udp/6000/quic-v1/p2p/16Uiu2HAmBKCp84Nfyq9cJ88hGpgjuxMDJXWrtWnKX6UqCDef1Pxj",
				},
				{
					ID:      "3WzUkWfKjER1LJn9g7bBuvPg9ho4z3WTUDpkwXE99VNoYitpKz87.16Uiu2HAkyFGMHVedU7kMFRGYtovtmDuABy7CfK1XnVodEVZtWxx6",
					Address: "/ip4/5.78.65.64/udp/6000/quic-v1/p2p/16Uiu2HAkyFGMHVedU7kMFRGYtovtmDuABy7CfK1XnVodEVZtWxx6",
				},
				{
					ID:      "3WwpJCMQz5X4LU1EKZ6waE4Z442ck3gV2dwzaKXKtRDNzMP5L1Vq.16Uiu2HAkybuawfwYfb2wfAV6SJmE4Nx2XD2qTPZz7FzjfWqi1Bx3",
					Address: "/ip4/5.78.71.107/udp/6000/quic-v1/p2p/16Uiu2HAkybuawfwYfb2wfAV6SJmE4Nx2XD2qTPZz7FzjfWqi1Bx3",
				},
				{
					ID:      "3WxEh4HqnHPm7d7CYHHa1SVWywZEWXBKfqW443GNKPWTT7fnDJp7.16Uiu2HAm9s1ks61UGemeZ2RpAT9uKmFmjyBsuQdMT6TPpNaC9NPv",
					Address: "/ip4/5.78.88.32/udp/6000/quic-v1/p2p/16Uiu2HAm9s1ks61UGemeZ2RpAT9uKmFmjyBsuQdMT6TPpNaC9NPv",
				},
				{
					ID:      "3WxbRi4GZ4Jbc6ZPdZCUDVhTQVs2zEpvyfffVJkYKsLVpJPU8djd.16Uiu2HAmGR7rfUquYukB3SNNcSphanXQhysajrhmEAY5wrwZxLzp",
					Address: "/ip4/5.78.93.48/udp/6000/quic-v1/p2p/16Uiu2HAmGR7rfUquYukB3SNNcSphanXQhysajrhmEAY5wrwZxLzp",
				},
			},
			SyncMode:       int(config.DefaultSyncMode),
			EnableSnapSync: true,
		},
		Consensus: ConsensusConfig{
			TimeoutPropose:        30000,
			TimeoutProposeDelta:   50000,
			TimeoutPrevote:        10000,
			TimeoutPrevoteDelta:   50000,
			TimeoutPrecommit:      10000,
			TimeoutPrecommitDelta: 50000,
			TimeoutCommit:         10000,
			Precision:             1000,
			MessageDelay:          5500,
			AccountWaitTime:       1500,
			OperatorSlots:         1,
			ValidatorSlots:        5,
			MaxGossipPeers:        5,
			MinGossipPeers:        3,
			EnableSortition:       true,
			GenesisTime:           config.DefaultGenesisTime,
			GenesisPath:           path + "/genesis.json",
			GenesisSeed:           config.DefaultGenesisSeed,
			GenesisProof:          config.DefaultGenesisProof,
		},
		DB: DBConfig{
			CleanDB:     false,
			DBFolder:    path + config.DefaultDBDirectory,
			MaxSnapSize: config.DefaultSnapSize, // 6GB limit
		},
		Execution: ExecutionConfig{
			FuelLimit: hexutil.Uint64(config.DefaultFuelLimit),
		},
		IxPool: IxPoolConfig{
			Mode:       config.DefaultIxPoolMode,
			PriceLimit: hexutil.Big(*config.DefaultIxPriceLimit),
			MaxSlots:   config.DefaultMaxIXPoolSlots,
		},
		Telemetry: Telemetry{
			PrometheusAddr: "",
			OtlpAddress:    "",
			Token:          "",
		},
		LogFilePath: path + config.DefaultLogDirectory,
		JSONRPC: JSONRPCConfig{
			TesseractRangeLimit: config.DefaultTesseractRangeLimit,
			BatchLengthLimit:    config.DefaultBatchLengthLimit,
		},
		NetworkID: config.Babylon,
		State: StateConfig{
			TreeCacheSize: config.DefaultTreeCacheSize,
		},
	}
}

func DefaultDevnetConfig(path string) *Config {
	return &Config{
		NodeType:       7,
		KramaIDVersion: 1,
		Vault: VaultConfig{
			DataDir: path,
			Mode:    crypto.GuardianMode,
		},
		Network: NetworkConfig{
			Libp2pAddr: []string{
				"/ip4/0.0.0.0/tcp/" + strconv.Itoa(config.DefaultP2PPort),
				"/ip4/0.0.0.0/udp/" + strconv.Itoa(config.DefaultP2PPort) + "/quic-v1",
				"/ip6/::/tcp/" + strconv.Itoa(config.DefaultP2PPort),
				"/ip6/::/udp/" + strconv.Itoa(config.DefaultP2PPort) + "/quic-v1",
			},
			BootStrapPeers:     make([]string, 0),
			MaxPeers:           0, // current we don't limit the no.of peers
			InboundConnLimit:   config.DefaultInboundConnLimit,
			OutboundConnLimit:  config.DefaultOutboundConnLimit,
			MinimumConnections: config.DefaultMinimumConnections,
			MaximumConnections: config.DefaultMaximumConnections,
			AllowIPv6Addresses: false,
			DisablePrivateIP:   false,
			DiscoveryInterval:  config.DefaultDiscoveryInterval,
			JSONRPCAddr:        "0.0.0.0:" + strconv.Itoa(config.DefaultJSONRPCPort),
			CorsAllowedOrigins: []string{"*"},
			RefreshSenatus:     true,
		},
		Syncer: SyncerConfig{
			ShouldExecute:  true,
			SyncMode:       int(config.DefaultSyncMode),
			EnableSnapSync: true,
		},
		Consensus: ConsensusConfig{
			TimeoutPropose:        30000,
			TimeoutProposeDelta:   50000,
			TimeoutPrevote:        10000,
			TimeoutPrevoteDelta:   50000,
			TimeoutPrecommit:      10000,
			TimeoutPrecommitDelta: 50000,
			TimeoutCommit:         10000,
			Precision:             1000,
			MessageDelay:          5500,
			AccountWaitTime:       1500,
			OperatorSlots:         -1,
			ValidatorSlots:        3,
			EnableSortition:       false,
			GenesisSeed:           config.DefaultGenesisSeed,
			GenesisProof:          config.DefaultGenesisProof,
			GenesisTime:           0,
			GenesisPath:           path + "/genesis.json",
		},
		DB: DBConfig{
			CleanDB:     false,
			DBFolder:    path + config.DefaultDBDirectory,
			MaxSnapSize: config.DefaultSnapSize, // 6GB limit
		},
		Execution: ExecutionConfig{
			FuelLimit: hexutil.Uint64(config.DefaultFuelLimit),
		},
		IxPool: IxPoolConfig{
			Mode:       config.DefaultIxPoolMode,
			PriceLimit: hexutil.Big(*config.DefaultIxPriceLimit),
			MaxSlots:   config.DefaultMaxIXPoolSlots,
		},
		Telemetry: Telemetry{
			PrometheusAddr: "",
			OtlpAddress:    "",
			Token:          "",
		},
		LogFilePath: path + config.DefaultLogDirectory,
		JSONRPC: JSONRPCConfig{
			TesseractRangeLimit: config.DefaultTesseractRangeLimit,
			BatchLengthLimit:    config.DefaultBatchLengthLimit,
		},
		NetworkID: config.Devnet,
		State: StateConfig{
			TreeCacheSize: config.DefaultTreeCacheSize,
		},
	}
}

type NetworkConfig struct {
	BootStrapPeers     []string      `json:"bootnodes"`
	TrustedPeers       []PeerInfo    `json:"trusted_peers"`
	StaticPeers        []PeerInfo    `json:"static_peers"`
	MaxPeers           uint          `json:"max_peers"`
	RelayNodeAddr      string        `json:"relay_node_addr"`
	Libp2pAddr         []string      `json:"libp2p_addr"`
	PublicP2pAddr      []string      `json:"public_p2p_addr"`
	P2PHostPort        int           `json:"p2p_host_port"`
	JSONRPCAddr        string        `json:"jsonrpc_addr"`
	MTQ                float64       `json:"mtq"`
	CorsAllowedOrigins []string      `json:"cors_allowed_origins"`
	NetworkSize        uint64        `json:"network_size"`
	NoDiscovery        bool          `json:"no_discovery"`
	RefreshSenatus     bool          `json:"refresh_senatus"`
	InboundConnLimit   int64         `json:"inbound_conn_limit"`
	OutboundConnLimit  int64         `json:"outbound_conn_limit"`
	MinimumConnections int           `json:"minimum_connections"`
	MaximumConnections int           `json:"maximum_connections"`
	AllowIPv6Addresses bool          `json:"allow_ipv6_addresses"`
	DisablePrivateIP   bool          `json:"disable_private_ip"`
	DiscoveryInterval  time.Duration `json:"discovery_interval"`
}

type SyncerConfig struct {
	ShouldExecute  bool
	TrustedPeers   []PeerInfo `json:"trusted_peers"`
	EnableSnapSync bool
	SyncMode       int
}

type IxPoolConfig struct {
	Mode       int         `json:"mode"`
	PriceLimit hexutil.Big `json:"price_limit"`
	MaxSlots   uint64      `json:"max_slots"`
}

type DBConfig struct {
	DBFolder    string `json:"db_folder"`
	CleanDB     bool   `json:"clean_db"`
	MaxSnapSize uint64 `json:"max_snap_size"`
}

type Telemetry struct {
	PrometheusAddr string `json:"prometheus_addr"`
	OtlpAddress    string `json:"otlp_addr"`
	Token          string `json:"token"`
}

type ConsensusConfig struct {
	TimeoutPropose        int64  `json:"timeout_propose"`
	TimeoutProposeDelta   int64  `json:"timeout_propose_delta"`
	TimeoutPrevote        int64  `json:"timeout_prevote"`
	TimeoutPrevoteDelta   int64  `json:"timeout_prevote_delta"`
	TimeoutPrecommit      int64  `json:"timeout_precommit"`
	TimeoutPrecommitDelta int64  `json:"timeout_precommit_delta"`
	TimeoutCommit         int64  `json:"timeout_commit"`
	SkipTimeoutCommit     bool   `json:"skip_timeout_commit"`
	AccountWaitTime       int    `json:"wait_time"`
	MessageDelay          int64  `json:"message_delay"`
	Precision             int64  `json:"precision"`
	OperatorSlots         int    `json:"operator_slots"`
	ValidatorSlots        int    `json:"validator_slots"`
	EnableDebugMode       bool   `json:"enable_debug_mode"`
	MaxGossipPeers        int    `json:"max_gossip_peers"`
	MinGossipPeers        int    `json:"min_gossip_peers"`
	GenesisTime           uint64 `json:"genesis_time"`
	GenesisPath           string `json:"genesis_path"`
	EnableSortition       bool   `json:"enable_sortition"`
	GenesisSeed           string `json:"genesis_seed"`
	GenesisProof          string `json:"genesis_proof"`
}

type ExecutionConfig struct {
	FuelLimit hexutil.Uint64 `json:"fuel_limit"`
}

type VaultConfig struct {
	DataDir      string
	NodePassword string
	SeedPhrase   string
	Mode         int8   // 0: Server, 1: Register/User mode
	NodeIndex    uint32 // Requires only in Register mode
	InMemory     bool
}

type JSONRPCConfig struct {
	TesseractRangeLimit uint8  `json:"tesseract_range_limit"`
	BatchLengthLimit    uint64 `json:"batch_length_limit"`
}

type StateConfig struct {
	TreeCacheSize uint64 `json:"tree_cache_size"`
}
