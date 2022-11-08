package jug

//func ProcessTesseract(exec *Exec, ts map[types.Address]*types.Tesseract,
//ix types.Interactions, contextDelta map[types.Address]*types.ContextDelta) (types.Receipts, error) {
//	reciepts := make(types.Receipts, 0, len(ix))
//
//for k, v := range ix {
//	if k == 0 {
//		fmt.Println("executing in phase 1")
//		r, err := exec.ExecuteInteraction(v, contextDelta)
//		if err != nil {
//			return nil, err
//		}
//		reciepts = append(reciepts, r)
//
//	} else {
//		fmt.Println("executing in phase 2")
//		r, err := exec.ExecuteInteraction(v, nil)
//		if err != nil {
//			return nil, err
//		}
//		reciepts = append(reciepts, r)
//	}
//}

// if bytes.Compare(sender.Body.StateHash, reciepts[len(reciepts)-1].SenderStateHash) != 0 {
// 	return errors.New("State Hash mismatch revert the db")
// }

// if bytes.Compare(reciever.Body.StateHash, reciepts[len(reciepts)-1].RecieverStateHash) != 0 {
// 	return errors.New("State Hash mismatch revert the db")
// }

//	return reciepts, nil
//}
