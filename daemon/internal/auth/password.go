package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

var ErrInvalidHash = errors.New("invalid password hash")

const (
	argonMemory      = 64 * 1024
	argonIterations  = 3
	argonParallelism = 2
	argonKeyLength   = 32
	argonSaltLength  = 16
)

func HashPassword(password string) (string, error) {
	if len(password) < 12 {
		return "", errors.New("password must contain at least 12 characters")
	}
	if len(password) > 1024 {
		return "", errors.New("password is too long")
	}
	salt := make([]byte, argonSaltLength)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrInvalidHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, ErrInvalidHash
	}
	if version != argon2.Version {
		return false, ErrInvalidHash
	}
	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false, ErrInvalidHash
	}
	if memory < 8*1024 || memory > 1024*1024 || iterations < 1 || iterations > 16 || parallelism < 1 || parallelism > 16 {
		return false, ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrInvalidHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 || len(want) > 64 {
		return false, ErrInvalidHash
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func PasswordStrengthError(password string) error {
	if len(password) < 12 {
		return errors.New("password must contain at least 12 characters")
	}
	var classes [4]bool
	for _, r := range password {
		switch {
		case r >= 'a' && r <= 'z':
			classes[0] = true
		case r >= 'A' && r <= 'Z':
			classes[1] = true
		case r >= '0' && r <= '9':
			classes[2] = true
		default:
			classes[3] = true
		}
	}
	n := 0
	for _, c := range classes {
		if c {
			n++
		}
	}
	if n < 3 {
		return errors.New("password must use at least three of: lowercase, uppercase, digits, symbols")
	}
	if isCommonPassword(password) {
		return errors.New("password is too common")
	}
	return nil
}

func isCommonPassword(p string) bool {
	common := []string{
		"password1234", "password12345", "qwerty123456", "123456789012",
		"mazima123456", "admin1234567", "letmein12345", "iloveyou1234",
	}
	lp := strings.ToLower(p)
	for _, c := range common {
		if lp == c {
			return true
		}
	}
	return false
}

var _ = strconv.Itoa
