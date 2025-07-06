package hash_sha1

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
)

func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) != 1 {
		return nil, false, fmt.Errorf("hash_sha1 requires 1 string arg")
	}
	s, ok := args[0].(string)
	if !ok {
		return nil, false, fmt.Errorf("arg must be string")
	}
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:]), true, nil
}
