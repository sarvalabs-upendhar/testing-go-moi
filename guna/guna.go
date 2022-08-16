package guna

// TokenID is a unique identifier generated for each token in KIP network
// type TokenID string

// // TokenInfo is used to persist info on each token, indexed by it's id
// type TokenInfo struct {
// 	id          TokenID `json:"id"`
// 	Name        string  `json:"name"`
// 	Dimension   string  `json:"dimension"` // Out of 7 TDU dimensions
// 	Metadata    []byte  `json:"metadata"`
// 	TotalSupply uint    `json:"total_supply"`
// }

// // TDU defines all the multidimensional points associated with the account
// type TDU struct {
// 	EconomicTokens   TokenBalanceMap `json:"economic_tokens"`   // Economic dimensions of the account quantified
// 	PrivilegeTokens  TokenBalanceMap `json:"privilege_tokens"`  // Privilege dimensions of the account quantified
// 	TimeTokens       TokenBalanceMap `json:"time_tokens"`       // Time dimensions of the account quantified
// 	SocialTokens     TokenBalanceMap `json:"social_tokens"`     // Social dimensions of the account quantified
// 	BarterTokens     TokenBalanceMap `json:"barter_tokens"`     // Barter dimensions of the account quantified
// 	EmotionTokens    TokenBalanceMap `json:"emotion_tokens"`    // Emotion dimensions of the account quantified
// 	PossessionTokens TokenBalanceMap `json:"possession_tokens"` // Possession dimensions of the account quantified
// }

// // SimpleTransfer is used to transfer any token between accounts
// func (from Account) SimpleTransfer(to Address, _id TokenID, value uint, data []byte) {
// 	// 1. Decrease the balance from the sender account's state
// 	// 2. Increase the balance to the receiver account's state
// 	// 3. Create new account states for sender and receiver accounts
// 	// 4. Return the new account states back to Exec(), which in turn returns it to KRAMA
// }

// // BatchTransfer is used to transfer tokens to more than one account
// func (from Account) BatchTransfer(to Address, _id []TokenID, value []uint, data []byte) {
// 	// 1. Decrease the balance of each token from the sender account's state
// 	// 2. Increase the balance of each token to the receiver account's state
// 	// 3. Create new account states for sender and receiver accounts
// 	// 4. Return the new account states back to Exec(), which in turn returns it to KRAMA
// }

// // BalanceOf returns the balance of the requested token id
// func (acc Account) BalanceOf(_id TokenID) uint {
// 	// 1. Return the balance of the account
// 	return 0
// }

// // BatchBalanceOf returns the balance of the requested token id
// func (acc Account) BatchBalanceOf(_id []TokenID) []uint {
// 	// 1. Return the balance of the account
// 	return []uint{}
// }
