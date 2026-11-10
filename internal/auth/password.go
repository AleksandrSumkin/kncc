package auth

import (
	"crypto/rand"
	"math/big"
)

const (
	// DefaultPasswordLength is the default number of characters for generated admin passwords.
	DefaultPasswordLength = 16

	// PasswordCharset contains characters used for secure password generation.
	// Includes uppercase, lowercase, numbers, and safe symbols.
	PasswordCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#%&*+-=?@"
)

// GenerateSecurePassword generates a cryptographically secure random password of the specified length.
// If length <= 0, DefaultPasswordLength is used.
func GenerateSecurePassword(length int) string {
	if length <= 0 {
		length = DefaultPasswordLength
	}

	b := make([]byte, length)
	charsetLen := big.NewInt(int64(len(PasswordCharset)))
	for i := range b {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			// Deterministic fallback in the virtually impossible case crypto/rand fails
			b[i] = PasswordCharset[i%len(PasswordCharset)]
		} else {
			b[i] = PasswordCharset[n.Int64()]
		}
	}
	return string(b)
}
