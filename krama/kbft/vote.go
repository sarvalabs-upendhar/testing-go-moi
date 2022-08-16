package kbft

/*
// Vote is a struct that represents a Vote in the consensus mechanism
type Vote struct {
	// Represents the type of vote consensus message
	Type consensusProtos.ConsensusMsgType `json:"type"`

	// Represents the height for which the vote applies
	Height []int64 `json:"height"`

	// Represents the round of the vote. Assume maximum 2_147_483_647 rounds
	Round int32 `json:"round"`

	// Represents the tesseract grid id for which the applies. 0 if the vote is nil
	GridID TesseractGridID `json:"grid_id"`

	// Represents the index of the validator on the round's validator set
	ValidatorIndex int32 `json:"validator_index"`

	// Represents the signature of the vote
	Signature []byte `json:"signature"`
}

// VoteFromProto is a constructor function that generates a new Vote from a ktypes.Vote proto
func VoteFromProto(protomsg *consensusProtos.Vote) (*Vote, error) {
	// Generate a new Vote and assign fields from the proto
	vote := &Vote{
		Type:           protomsg.Type,
		Height:         protomsg.Height,
		Round:          protomsg.Round,
		Signature:      protomsg.Signature,
		ValidatorIndex: protomsg.Validatorindex,
	}

	// Attempt to parse the Tesseract Grid id from the proto
	t, err := TesseractGridIDFromProto(protomsg.Gridid)
	if err != nil {
		return nil, err
	}

	// Attach the tesseract grid id to the vote and return it
	vote.GridID = *t
	return vote, nil
}

// ToProto is a method of Vote that marshals it into a ktypes.Vote proto message
func (vote *Vote) ToProto() *consensusProtos.Vote {
	return &consensusProtos.Vote{
		Height:         vote.Height,
		Round:          vote.Round,
		Gridid:         vote.GridID.ToProto(),
		Type:           vote.Type,
		Validatorindex: vote.ValidatorIndex,
		Signature:      vote.Signature,
	}
}

// Validate is a method of Vote that validates it.
// Ensures that vote heights and rounds are within acceptable bounds and that the
// signature and validator address lengths is less than 64 and 32 respectively.
//
// Note: timestamp validation is subtle and handled elsewhere.
func (vote *Vote) Validate() error {
	// Check that vote heights are 0 or greater
	if vote.Height[0] < 0 || vote.Height[1] < 0 {
		return errors.New("negative vote height")
	}

	// Check that vote rounds is 0 or greater
	if vote.Round < 0 {
		return errors.New("negative vote round")
	}

	// Check if validator index is 0 or greater
	if vote.ValidatorIndex < 0 {
		return errors.New("negative validator index")
	}

	// Check if vote signature is null
	if len(vote.Signature) == 0 {
		return errors.New("vote signature is missing")
	}

	// Check if vote signature is too large
	if len(vote.Signature) > 64 {
		return fmt.Errorf("signature is too big (max: %d)", 64)
	}

	// no errors
	return nil
}
*/
