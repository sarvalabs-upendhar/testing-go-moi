package kbft

//type Tesseracts struct {
//	total uint32
//	mtx   sync.Mutex
//	hash  ktypes.Hash
//	parts []*common.Tesseract
//	size  int64
//}
//
//func (ts *Tesseracts) IsComplete() bool {
//	for i := 0; i < len(ts.parts); i++ {
//		if ts.parts[i] == nil {
//			return false
//		}
//	}
//	return true
//}
//
//func (ts *Tesseracts) getTesseractParts() TesseractParts {
//	ps := TesseractParts{
//		Total:  int32(ts.total),
//		Hashes: make([]ktypes.Hash, len(ts.parts)),
//	}
//
//	i := 0
//	for _, v := range ts.parts {
//		ps.Hashes[i] = ktypes.BytesToHash(v.Bytes())
//		i++
//	}
//
//	return ps
//}
//
//func (ts *Tesseracts) AddPart(index int64, t *common.Tesseract) (bool, error) {
//	if ts == nil {
//		return false, nil
//	}
//	ts.mtx.Lock()
//	defer ts.mtx.Unlock()
//	if index >= int64(ts.total) {
//		return false, errors.New("invalid Index")
//	}
//	if ts.parts[index] != nil {
//		return false, errors.New("part already available")
//	}
//
//	// TODO:Check proof
//	ts.parts[index] = t
//	return true, nil
//}
