package utils

/*
This file has all the utility function required for KIP
*/
import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/sarvalabs/go-moi/common"
)

const charset = "abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func NewUint64(val uint64) *uint64 {
	return &val
}

// RandUint64 returns a random 64-bit unsigned integer
func RandUint64() uint64 {
	var eightbytes [8]byte

	_, err := crand.Read(eightbytes[:])
	if err != nil {
		log.Println("error reading random bytes", err)
	}

	return binary.LittleEndian.Uint64(eightbytes[:])
}

var seededRand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

// RandString returns a random string from tha charset supplied
func RandString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}

	return string(b)
}

// GetRandomNumbers returns unique random numbers in the half-open interval [0,maxNum)
func GetRandomNumbers(count, maxNum int) map[int]struct{} {
	rand.New(rand.NewSource(time.Now().UnixNano()))

	randomNumbers := make(map[int]struct{})

	// Add values to the map to track random numbers and ensure uniqueness
	for i := 0; i < count; {
		num := rand.Intn(maxNum)

		// Check if the number has already been selected
		if _, exists := randomNumbers[num]; !exists {
			randomNumbers[num] = struct{}{}
			i++
		}
	}

	return randomNumbers
}

// EnsureDir ensures the given directory exists, or creates if required
func EnsureDir(dir string, mode os.FileMode) error {
	if err := os.MkdirAll(dir, mode); err != nil {
		return fmt.Errorf("could not create directory %q: %w", dir, err)
	}

	return nil
}

// ItemExists is used to check whether the given element is present in the interface ...
func ItemExists(arrayType interface{}, item interface{}) bool {
	arr := reflect.ValueOf(arrayType)
	if arr.Kind() != reflect.Slice {
		panic("Invalid data-type")
	}

	for i := 0; i < arr.Len(); i++ {
		if arr.Index(i).Interface() == item {
			return true
		}
	}

	return false
}

func ExponentialTimeout(baseTime time.Duration, exponent int32) time.Duration {
	var timeout time.Duration

	if exponent > 0 {
		s1 := rand.NewSource(time.Now().UnixNano())
		reg := rand.New(s1)
		r := 1 + reg.Float64()*(2-1) // Range 0-1
		x := r * float64(baseTime.Milliseconds()) * math.Pow(2, float64(exponent))
		timeout = time.Duration(x) * time.Millisecond
	}

	return timeout
}

// AcquireMultiAddr is a function that acquires a multiaddr for the node.
// Returns the multiaddr struct and an error.
//
// Retrieves the host network interfaces and creates a multiaddr from the
// first IPv4 address it encounters that is not also a loopback address.
func AcquireMultiAddr() (multiaddr.Multiaddr, error) {
	// Declare a variable for the IP ID
	var ipaddrss string

	// Retrieve the network interface address of the host machine
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		// Return the error
		return nil, fmt.Errorf("unable to read network interfaces")
	}

	// Iterate over the network interface addresses
	for _, a := range addrs {
		// Check that the address is an IP address and not a loopback address
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			// Check if the IP address is an IPv4 address
			if ipnet.IP.To4() != nil {
				// Convert into a string IP address and store to variable
				ipaddrss = ipnet.IP.String()

				break
			}
		}
	}

	// Create and return the multiaddr
	return multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ipaddrss, 0))
}

func MultiAddrToString(maddr ...multiaddr.Multiaddr) (addrs []string) {
	for _, v := range maddr {
		addrs = append(addrs, v.String())
	}

	return
}

func MultiAddrFromString(maddr ...string) (addrs []multiaddr.Multiaddr) {
	for _, v := range maddr {
		maddr, err := multiaddr.NewMultiaddr(v)
		if err != nil {
			log.Println("Error parsing multi address")

			continue
		}

		addrs = append(addrs, maddr)
	}

	return
}

func ContainsKramaID(set []identifiers.KramaID, id identifiers.KramaID) bool {
	for _, v := range set {
		if v == id {
			return true
		}
	}

	return false
}

func GetNetworkID(id identifiers.KramaID) (peer.ID, error) {
	networkID, err := id.PeerID()
	if err != nil {
		return "", nil
	}

	peerID, err := peer.Decode(networkID)
	if err != nil {
		return "", nil
	}

	return peerID, nil
}

func ValidateAccountType(acc common.AccountType) error {
	switch acc {
	case common.SystemAccount, common.RegularAccount, common.LogicAccount:
		return nil
	default:
		return common.ErrInvalidAccountType
	}
}

func ValidateHash(hash string) (string, error) {
	hash = strings.TrimPrefix(hash, "0x")
	if len(hash) != 64 {
		return hash, common.ErrInvalidHash
	}

	r := regexp.MustCompile(`[^a-fA-F\d]`)
	if invalid := r.MatchString(hash); invalid {
		return hash, common.ErrInvalidHash
	}

	return hash, nil
}

func KramaIDFromString(nodes []string) []identifiers.KramaID {
	ids := make([]identifiers.KramaID, 0, len(nodes))

	for _, v := range nodes {
		ids = append(ids, identifiers.KramaID(v))
	}

	return ids
}

func KramaIDToString(peers []identifiers.KramaID) []string {
	ids := make([]string, 0, len(peers))

	for _, v := range peers {
		ids = append(ids, string(v))
	}

	return ids
}

func AreSlicesOfStringEqual(addr []string, addr1 []string) bool {
	if len(addr) != len(addr1) {
		return false
	}

	for i := range addr {
		if addr[i] != addr1[i] {
			return false
		}
	}

	return true
}

// ResolveAddr resolves the passed in TCP address
func ResolveAddr(raw string) (*net.TCPAddr, error) {
	addr, err := net.ResolveTCPAddr("tcp", raw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse addr '%s': %w", raw, err)
	}

	if addr.IP == nil {
		addr.IP = net.ParseIP("0.0.0.0")
	}

	return addr, nil
}

func ConvertMapToSlice(m map[identifiers.KramaID]struct{}) []identifiers.KramaID {
	slice := make([]identifiers.KramaID, 0)

	for k := range m {
		slice = append(slice, k)
	}

	return slice
}

// RetryUntilTimeout retries the given function until the timeout is reached.
func RetryUntilTimeout(timeout time.Duration, retryInterval time.Duration, fn func() error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryInterval):
			if err := fn(); err == nil {
				return
			}
		}
	}
}

// WrappedVal represents a gossip validator which also returns an error along with the result.
type WrappedVal func(context.Context, peer.ID, *pubsub.Message) (pubsub.ValidationResult, error)
