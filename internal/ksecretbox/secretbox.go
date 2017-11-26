package ksecretbox

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
)

// GenerateKey creates a new key full of random data.
func GenerateKey() (*[32]byte, error) {
	var k [32]byte
	_, err := rand.Read(k[:])
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// ShowKey makes a string out of an encryption key.
func ShowKey(key *[32]byte) string {
	return base64.URLEncoding.EncodeToString(key[:])
}

// ParseKey decodes a key from a string.
func ParseKey(s string) (*[32]byte, error) {
	k := &[32]byte{}
	raw, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if n := copy(k[:], raw); n < len(k) {
		return nil, errors.New("not valid")
	}
	return k, nil
}
