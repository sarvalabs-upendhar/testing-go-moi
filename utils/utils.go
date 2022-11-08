package utils

/*
This file has all the utility function required for KIP
*/
import (
	"encoding/hex"
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

	"github.com/libp2p/go-libp2p/core/peer"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"

	"gitlab.com/sarvalabs/moichain/types"

	"github.com/multiformats/go-multiaddr"
)

const charset = "abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var seededRand = rand.New( //nolint
	rand.NewSource(time.Now().UnixNano()))

// stringWithCharset returns a random string from tha charset supplied
func stringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}

	return string(b)
}

// EnsureDir ensures the given directory exists, or creates if required
func EnsureDir(dir string, mode os.FileMode) error {
	if err := os.MkdirAll(dir, mode); err != nil {
		return fmt.Errorf("could not create directory %q: %w", dir, err)
	}

	return nil
}

// RandString generated random string of given length
func RandString(length int) string {
	return stringWithCharset(length, charset)
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

func HexToByte(str string) []byte {
	data, err := hex.DecodeString(str)
	if err == nil {
		return data
	}

	return nil
}

func ExponentialTimeout(baseTime time.Duration, exponent int32) time.Duration {
	var timeout time.Duration

	if exponent > 0 {
		s1 := rand.NewSource(time.Now().UnixNano())
		reg := rand.New(s1) //nolint
		r := 1 + reg.Float64()*(2-1)
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
	// Declare a variable for the IP Address
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
		madd, err := multiaddr.NewMultiaddr(v)
		if err != nil {
			log.Println("Error parsing multi address")

			addrs = append(addrs, madd)
		}
	}

	return
}

func ContainsKramaID(set []id.KramaID, id id.KramaID) bool {
	for _, v := range set {
		if v == id {
			return true
		}
	}

	return false
}

func GetNetworkID(id id.KramaID) (peer.ID, error) {
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

func ValidateAddress(address string) (string, error) {
	address = strings.TrimPrefix(address, "0x")
	if len(address) != 64 {
		return address, types.ErrInvalidAddress
	}

	r, err := regexp.Compile(`[^a-fA-F\d]`)
	if err != nil {
		return address, err
	}

	if invalid := r.MatchString(address); invalid {
		return address, types.ErrInvalidAddress
	}

	return address, nil
}

func ValidateHash(hash string) (string, error) {
	hash = strings.TrimPrefix(hash, "0x")
	if len(hash) != 64 {
		return hash, types.ErrInvalidHash
	}

	r, err := regexp.Compile(`[^a-fA-F\d]`)
	if err != nil {
		return hash, err
	}

	if invalid := r.MatchString(hash); invalid {
		return hash, types.ErrInvalidHash
	}

	return hash, nil
}

func ValidateAssetID(aID string) (string, error) {
	aID = strings.TrimPrefix(aID, "0x")
	if len(aID) != 68 {
		return aID, types.ErrInvalidAssetID
	}

	r, err := regexp.Compile(`[^a-fA-F\d]`)
	if err != nil {
		return aID, err
	}

	if invalid := r.MatchString(aID); invalid {
		return aID, types.ErrInvalidAssetID
	}

	return aID, nil
}
