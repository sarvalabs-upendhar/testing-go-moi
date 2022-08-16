# Dhruva

## Objective of the project/repo/package

The goal of dhruva package is to offer network-wide content-addressed persistence of data.

The package implements the `PersistenceService` interface, which is satisfied by `PersistenceManager` and its methods.

## Environment variables needed

Dhruva stores all data in `badgerdb` folder under `MOI_PATH`. It also stores the private key of libp2p host here.

Before using this package, be sure to add this to your `~/.bashrc`, `~/.zshrc` or equivalent file:

```shell
export MOI_PATH=$HOME/.moi
```

If not exported, the package will export it with default values and create configurations.

## Example

Here's a scratch file which serves as an example to use the package:

### Code to store data and announce to p2p network

```go
package main

import (
	"context"
	"fmt"
	"github.com/ipfs/go-ipns"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	record "github.com/libp2p/go-libp2p-record"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/sarvalabs/dhruva"
	"gitlab.com/sarvalabs/moiconfig"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

func main() {
	// 1. First you need to create a fresh instance of Persistence Manager
	println("// Create Persistence Manager")
	// 1a. Custom PersistenceConfig can be sent, and if not sent default values will be used
	pmc := moiconfig.PersistenceConfig{
		DBFolderPath:      strings.TrimRight(os.Getenv("MOI_PATH"), "/") + "/badger",
		CidPrefixVersion:  1,
		CidPrefixCodec:    0x50,
		CidPrefixMhType:   0xb220,
		CidPrefixMhLength: -1,
	}
	// 1b. Setting up a lib2p host
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host, err := libp2p.New(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println("Server's host ID: ", host.ID().String())
	// 1c. Setting up a DHT for the host
	dhtOpts := []dht.Option{
		dht.NamespacedValidator("pk", record.PublicKeyValidator{}),
		dht.NamespacedValidator("ipns", ipns.Validator{KeyBook: host.Peerstore()}),
		dht.Concurrency(10),
		dht.Mode(dht.ModeAuto),
	}
	ServerDht, err := dht.New(context.Background(), host, dhtOpts...)
	if err != nil {
		log.Fatal("Failed to create Server host's DHT: ", err)
	}

	// 2. Creating a MOI-specific bootstrap nodes list
	MoiBootstrapAddr, err := multiaddr.NewMultiaddr("/ip4/139.59.73.20/tcp/4001/p2p/QmdSyhb8eR9dDSR5jjnRoTDBwpBCSAjT7WueKJ9cQArYoA")
	if err != nil {
		panic(err)
	}
	bootstrapPeers := []multiaddr.Multiaddr{MoiBootstrapAddr} // Use this to connect with public libp2p net: bootstrapPeers := dht.DefaultBootstrapPeers

	// 3. Connecting to each MOI-specific bootstrap nodes
	var wg sync.WaitGroup
	for _, bootstrapPeer := range bootstrapPeers {
		peerInfo, err := peer.AddrInfoFromP2pAddr(bootstrapPeer)
		if err != nil {
			panic(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := host.Connect(context.Background(), *peerInfo); err != nil {
				panic(err)
			} else {
				fmt.Println("Connection established with bootstrap node:", *peerInfo)
			}
		}()
	}
	wg.Wait()

	singleAnnounce := make(chan []byte)
	defer close(singleAnnounce)

	batchAnnounce := make(chan [][]byte)
	defer close(batchAnnounce)

	// 4. After all the configurations are complete in step 1, 2, and 3 - initiate PersistenceManager
	println("// Initializing persistence manager by passing config, host and dht.")
	p, err := dhruva.NewPersistenceManager(&pmc, host, ServerDht, singleAnnounce, batchAnnounce)
	if err != nil {
		panic(err)
	}
	defer func(p *dhruva.PersistenceManager) {
		err := p.Close()
		if err != nil {
			panic(err)
		}
	}(&p)

	// 5. Create a new CID based k-v entry using the PersistenceManager
	var cids [][]byte
	println("// Generate a managed CID-as-key-v entry")
	messages := []string{"My Own Internet", "MOI", "My Own Internet (MOI)"}
	counter := 0
	for _, msg := range messages {
		counter++
		println(counter)
		payload := []byte(msg)
		c, err := p.CreateCidEntry(payload)
		if err != nil {
			log.Fatalf("Error from CreateCidEntry():\n%v\n", err)
		}
		fmt.Printf("Key created: %v\n", c)

		// 5a. OPTIONAL: Pass the individual CID to the singleAnnounce channel
		//singleAnnounce <- c

		cids = append(cids, c)
	}
	// 5b. Pass the session specific batch of CIDs to the batchAnnounce channel
	batchAnnounce <- cids

	// 6. Let's provide two minutes of lead time for client snippet to read the data ;)
	time.Sleep(2 * time.Minute)

	// 7. Delete the entry locally before winding up
	println("// Delete the entry")
	for _, c := range cids {
		err = p.DeleteEntry(c)
		if err != nil {
			log.Fatalf("Error from DeleteEntry():\n%v\n", err)
		}
	}
}
```

### Code to read data from a remote node

```go
package main

import (
	"context"
	"fmt"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipns"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	record "github.com/libp2p/go-libp2p-record"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/sarvalabs/dhruva"
	"gitlab.com/sarvalabs/moiconfig"
	"log"
	"os"
	"strings"
	"sync"
)

func main() {
	// 1. First you need to create a fresh instance of Persistence Manager
	println("// Create Persistence Manager")
	// 1a. Custom PersistenceConfig can be sent, and if not sent default values will be used
	pmc := moiconfig.PersistenceConfig{
		DBFolderPath:      strings.TrimRight(os.Getenv("MOI_PATH"), "/") + "/badger",
		CidPrefixVersion:  1,
		CidPrefixCodec:    0x50,
		CidPrefixMhType:   0xb220,
		CidPrefixMhLength: -1,
	}
	// 1b. Setting up a lib2p host
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host, err := libp2p.New(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println("Client's host ID: ", host.ID().String())
	// 1c. Setting up a DHT for the host
	dhtOpts := []dht.Option{
		dht.NamespacedValidator("pk", record.PublicKeyValidator{}),
		dht.NamespacedValidator("ipns", ipns.Validator{KeyBook: host.Peerstore()}),
		dht.Concurrency(10),
		dht.Mode(dht.ModeAuto),
	}
	ServerDht, err := dht.New(context.Background(), host, dhtOpts...)
	if err != nil {
		log.Fatal("Failed to create client host's DHT: ", err)
	}

	// 2. Creating a MOI-specific bootstrap nodes list
	MoiBootstrapAddr, err := multiaddr.NewMultiaddr("/ip4/139.59.73.20/tcp/4001/p2p/QmdSyhb8eR9dDSR5jjnRoTDBwpBCSAjT7WueKJ9cQArYoA")
	if err != nil {
		panic(err)
	}
	bootstrapPeers := []multiaddr.Multiaddr{MoiBootstrapAddr} // Use this to connect with public libp2p net: bootstrapPeers := dht.DefaultBootstrapPeers

	// 3. Connecting to each MOI-specific bootstrap nodes
	var wg sync.WaitGroup
	for _, bootstrapPeer := range bootstrapPeers {
		peerInfo, err := peer.AddrInfoFromP2pAddr(bootstrapPeer)
		if err != nil {
			panic(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := host.Connect(context.Background(), *peerInfo); err != nil {
				panic(err)
			} else {
				fmt.Println("Connection established with bootstrap node:", *peerInfo)
			}
		}()
	}
	wg.Wait()

	singleAnnounce := make(chan []byte)
	batchAnnounce := make(chan [][]byte)

	// 4. After all the configurations are complete in step 1, 2, and 3 - initiate PersistenceManager
	println("// Initializing persistence manager by passing config, host and dht.")
	p, err := dhruva.NewPersistenceManager(&pmc, host, ServerDht, singleAnnounce, batchAnnounce)
	if err != nil {
		panic(err)
	}
	defer func(p *dhruva.PersistenceManager) {
		err := p.Close()
		if err != nil {
			panic(err)
		}
	}(&p)

	cidKeys := []string{"bafikbzacechlfqjpdzzbhmgeq2wtebyhacxsb7ezv465ppx2y65rabqqy6nj2",
		"bafikbzacecm6utmv3r7whun3y2fndd5aqg2dbjhqjwusvh2gv34v5ma536u7y",
		"bafikbzacedfy74w3yo5tih4nhp7qzajbqrctglx7ghbq7izrcip6w33b5tvhs",
	}
	var cidKeysBytes [][]byte
	for _, cidKey := range cidKeys {
		c, _ := cid.Decode(cidKey)
		cidKeysBytes = append(cidKeysBytes, c.Bytes())
	}

	// 5. Blatantly try to read data that does not exist in local BadgerDB
	for _, cidKey := range cidKeysBytes {
		value, err := p.ReadEntry(cidKey)
		if err != nil {
			println(err)
		}
		fmt.Println("Key: ", cidKey)
		fmt.Println("Value: ", value)
	}

	// 6. Request for remote data from the server by passing the CIDs
	err = p.FetchCidEntries(cidKeysBytes)
	if err != nil {
		panic(err)
	}
	println("Fetched all entries")

	// 7. Now, read data again
	for _, cidKey := range cidKeysBytes {
		value, err := p.ReadEntry(cidKey)
		if err != nil {
			println(err)
		}
		fmt.Println("Key: ", cidKey)
		fmt.Println("Value: ", value)
	}
}
```

**NOTE:** To test this repo in a same machine, you may want to export two distinct `MOI_PATH` values across two terminals.

## Code Owner

This project/repository/package is maintained by the following team members:

- Ganesh Prasad Kumble

## Code Reviewers

This repo/package is reviewed by following team members:

- Pankaj Nayak
- Rahul Lenkala
