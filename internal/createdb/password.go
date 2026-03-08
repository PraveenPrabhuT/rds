package createdb

import (
	"crypto/rand"
	"math/big"
)

const alphanumeric = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// GeneratePassword returns a cryptographically random alphanumeric password
// of the given length (A-Za-z0-9), matching the Ansible playbook's character set.
func GeneratePassword(length int) (string, error) {
	max := big.NewInt(int64(len(alphanumeric)))
	buf := make([]byte, length)
	for i := range buf {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		buf[i] = alphanumeric[idx.Int64()]
	}
	return string(buf), nil
}
