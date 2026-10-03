package totp

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// RFC 6238, appendix B: the SHA-1 vectors, last 6 digits.
func TestRFCVectors(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for unix, want := range map[int64]string{
		59: "287082", 1111111109: "081804", 1111111111: "050471",
		1234567890: "005924", 2000000000: "279037", 20000000000: "353130",
	} {
		got, err := Code(secret, Step(time.Unix(unix, 0)))
		if err != nil || got != want {
			t.Errorf("t=%d: got %s %v, want %s", unix, got, err, want)
		}
	}
}

func TestVerify(t *testing.T) {
	secret := NewSecret()
	now := time.Unix(1_700_000_000, 0)
	code, _ := Code(secret, Step(now))
	if step, ok := Verify(secret, code[:3]+" "+code[3:], now); !ok || step != Step(now) {
		t.Fatal("the current code, typed with a space, should verify")
	}
	prev, _ := Code(secret, Step(now)-1)
	if _, ok := Verify(secret, prev, now); !ok {
		t.Fatal("one step of drift should be allowed")
	}
	old, _ := Code(secret, Step(now)-3)
	if _, ok := Verify(secret, old, now); ok {
		t.Fatal("a code from 90 seconds ago must not verify")
	}
	if _, ok := Verify(secret, "12345", now); ok {
		t.Fatal("short codes must not verify")
	}
	if u := URI("Uptimy Agent", "admin", secret); !strings.HasPrefix(u, "otpauth://totp/Uptimy%20Agent:admin?") || !strings.Contains(u, "secret="+secret) {
		t.Fatalf("uri %s", u)
	}
}
