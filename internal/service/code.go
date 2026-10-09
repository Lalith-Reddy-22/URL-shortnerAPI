package service

import (
	"crypto/rand"
	"math/big"
)

const (
	base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	shortCodeLen   = 7
)

var alphabetLen = big.NewInt(int64(len(base62Alphabet)))

// RandomCode returns n cryptographically random base62 characters.
// crypto/rand + big.Int avoids the modulo bias of `randByte % 62`.
func RandomCode(n int) (string, error) {
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			return "", err
		}
		out[i] = base62Alphabet[idx.Int64()]
	}
	return string(out), nil
}
