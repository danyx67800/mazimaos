package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	totpPeriod = 30
	totpDigits = 6
)

func NewTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

func TOTPURI(issuer, account, secret string) string {
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=%d&period=%d",
		urlEscape(issuer), urlEscape(account), secret, urlEscape(issuer), totpDigits, totpPeriod)
}

func urlEscape(s string) string {
	r := strings.NewReplacer("%", "%25", ":", "%3A", "/", "%2F", "?", "%3F", "&", "%26", "=", "%3D", " ", "%20", "@", "%40")
	return r.Replace(s)
}

func TOTPCode(secret string, at time.Time) (string, error) {
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}
	counter := uint64(at.Unix() / totpPeriod)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, raw)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, code%1e6), nil
}

func VerifyTOTP(secret, code string, at time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false
	}
	for _, delta := range []int{-1, 0, 1} {
		want, err := TOTPCode(secret, at.Add(time.Duration(delta*totpPeriod)*time.Second))
		if err != nil {
			return false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

func NewRecoveryCodes(n int) ([]string, []string, error) {
	plain := make([]string, 0, n)
	hashes := make([]string, 0, n)
	for i := 0; i < n; i++ {
		b := make([]byte, 6)
		if _, err := rand.Read(b); err != nil {
			return nil, nil, err
		}
		code := strings.ToUpper(hex.EncodeToString(b))
		code = code[:5] + "-" + code[5:10] + "-" + code[10:]
		plain = append(plain, code)
		hashes = append(hashes, HashRecoveryCode(code))
	}
	return plain, hashes, nil
}

func HashRecoveryCode(code string) string {
	norm := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	sum := sha256.Sum256([]byte("mazima-recovery:" + norm))
	return hex.EncodeToString(sum[:])
}

func ConsumeRecoveryCode(code string, hashes []string) ([]string, bool) {
	h := HashRecoveryCode(code)
	for i, candidate := range hashes {
		if subtle.ConstantTimeCompare([]byte(h), []byte(candidate)) == 1 {
			return append(append([]string{}, hashes[:i]...), hashes[i+1:]...), true
		}
	}
	return hashes, false
}
