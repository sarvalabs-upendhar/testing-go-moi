package chain

/*
import (
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"testing"
)

type MockDB struct {
	data map[string][]byte
	acc  map[]
}

func (m *MockDB) CreateEntry(key, value []byte, diskWrite bool) error {
	encodedKey := hex.EncodeToString(key)
	m.data[encodedKey] = value
	return nil
}
func (m *MockDB) ReadEntry(key []byte) ([]byte, error) {
	encodedKey := hex.EncodeToString(key)
	return m.data[encodedKey], nil
}
func (m *MockDB) Contains(key []byte) (bool, error) {
	encodedKey := hex.EncodeToString(key)
	_, ok := m.data[encodedKey]
	return ok, nil
}
func (m *MockDB) Flush(keys [][]byte) error {
	return nil
}
func (m *MockDB) UpdateAccDetails(id []byte, height []byte, tesseractHash []byte, latticeExsists bool,
tesseractExsits bool) (int32, int64, error) {
*/
/*	key, bucket := dhruva.BucketIDFromAddress(id)
		data, err := m.ReadEntry(key)
		fmt.Println("Printing bucket id @@!!!", bucket)
		if err != nil && status.Code(err) != codes.NotFound {
			fmt.Println("here is the error", status.Code(err))
			return 0, 0, err
		} else if status.Code(err) == codes.NotFound {
			msg := new(netprotos.AccountDetails)
			msg.TesseractExists = tesseractExsits
			msg.LatticeExsits = latticeExsists
			msg.TesseractHash = tesseractHash
			msg.id = id
			msg.Height = height
			data, err := proto.Marshal(msg)
			if err != nil {
				return -1, -1, err
			}
			if err = m.CreateEntry(key, data, true); err != nil {
				return -1, -1, err
			}
			if err = p.incrementBucketCount(bucket.getIDBytes(), 1); err != nil {
				log.Panic(err)
			}
			return int32(bucket.getID()), 1, nil
		}
		fmt.Println("Printing account details", data)
		msg := new(netprotos.AccountDetails)
		if err := proto.Unmarshal(data, msg); err != nil {
			return -1, -1, err
		}
		if bytes.Compare(height, msg.Height) >= 0 {
			msg.LatticeExsits = latticeExsists
			msg.TesseractExists = tesseractExsits
			msg.TesseractHash = tesseractHash
			msg.id = id
			msg.Height = height

			//return -1, -1, errors.New("rejecting messages with less height")
		}
		if msg.LatticeExsits {
			msg.LatticeExsits = latticeExsists

			updated, err := proto.Marshal(msg)
			if err != nil {
				return -1, -1, err
			}
			return int32(bucket.getID()), 1, p.UpdateEntry(key, updated, true)
		}
		updated, err := proto.Marshal(msg)
		if err != nil {
			return -1, -1, err
		}

		return -1, -1, p.UpdateEntry(key, updated, true)

}
func CreateMockGenesisFile(t *testing.T) string {
	genesis := new(Genesis)
	genesis.Accounts = append(genesis.Accounts, AccountInfo{
		Address:          "0xrahul",
		MOIId:            "rahul_moi_id",
		BehaviourContext: []string{"Node1", "Node2"},
		RandomContext:    []string{"Node3", "Node4"},
	})
	data, err := json.MarshalIndent(genesis, "", "")
	if err != nil {
		t.Fatal(err)
	}
	file, err := ioutil.TempFile("", "*config.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
	return file.Name()
}
func TestChainManager_AddGenesis(t *testing.T) {

}
*/
