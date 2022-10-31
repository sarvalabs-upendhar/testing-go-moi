package guna

type Trie interface {
	TryGet([]byte) ([]byte, error)
}
