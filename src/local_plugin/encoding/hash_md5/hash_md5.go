package hash_md5

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
)

// Eval returns MD5 hex of string. Args: input string.
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) != 1 {
		return nil, false, fmt.Errorf("hash_md5 requires 1 string arg")
	}
	s, ok := args[0].(string)
	if !ok {
		return nil, false, fmt.Errorf("arg must be string")
	}
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:]), true, nil
}
