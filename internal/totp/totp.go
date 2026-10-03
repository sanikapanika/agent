// Package totp implements time-based one-time passwords (RFC 6238) as
// authenticator apps use them: HMAC-SHA1, 6 digits, 30-second steps.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // G505: RFC 6238 and every authenticator app use HMAC-SHA1
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	period = 30 // seconds per step
	digits = 6
)

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret returns a random 160-bit secret, base32-encoded as apps expect.
func NewSecret() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return encoding.EncodeToString(b)
}

// Step is the time step t falls in.
func Step(t time.Time) int64 { return t.Unix() / period }

// Code is the code for secret at step.
func Code(secret string, step int64) (string, error) {
	key, err := encoding.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", fmt.Errorf("invalid secret: %w", err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step)) //nolint:gosec // G115: steps are positive
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", digits, n%1_000_000), nil
}

// Verify checks code against secret at now, allowing one step of clock
// drift either way, and returns the step it matched.
func Verify(secret, code string, now time.Time) (step int64, ok bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != digits {
		return 0, false
	}
	cur := Step(now)
	for _, s := range []int64{cur - 1, cur, cur + 1} {
		want, err := Code(secret, s)
		if err == nil && subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

// URI is the otpauth:// URI authenticator apps scan as a QR code.
func URI(issuer, account, secret string) string {
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", fmt.Sprint(digits))
	v.Set("period", fmt.Sprint(period))
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + v.Encode()
}
