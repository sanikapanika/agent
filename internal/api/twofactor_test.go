package api

import (
	"testing"
	"time"

	"github.com/uptimy/agent/internal/totp"
)

func TestTwoFactor(t *testing.T) {
	c := newTestServer(t)
	c.login("admin", adminPassword)
	if _, st := c.do("GET", "/api/auth/2fa", nil); st["enabled"] != false {
		t.Fatalf("status %v", st)
	}
	_, tok := c.do("POST", "/api/auth/tokens", map[string]any{"name": "CI"})
	token := tok["token"].(string)

	code, setup := c.do("POST", "/api/auth/2fa/setup", map[string]any{})
	secret, _ := setup["secret"].(string)
	if code != 200 || secret == "" || setup["uri"] == nil {
		t.Fatalf("setup: %d %v", code, setup)
	}
	if code, _ := c.do("POST", "/api/auth/2fa/enable", map[string]any{"code": "000000"}); code != 400 {
		t.Fatalf("a wrong code enabled it: %d", code)
	}
	now := totp.Step(time.Now())
	first, _ := totp.Code(secret, now)
	code, en := c.do("POST", "/api/auth/2fa/enable", map[string]any{"code": first})
	codes, _ := en["recovery_codes"].([]any)
	if code != 200 || len(codes) != 10 {
		t.Fatalf("enable: %d %v", code, en)
	}
	// The session that turned it on stays signed in.
	if code, st := c.do("GET", "/api/auth/2fa", nil); code != 200 || st["enabled"] != true || st["recovery_codes_left"] != float64(10) {
		t.Fatalf("after enabling: %d %v", code, st)
	}

	other := c.another()
	if code, body := other.login("admin", adminPassword); code != 401 || body["two_factor_required"] != true {
		t.Fatalf("password alone: %d %v", code, body)
	}
	login := func(code string) int {
		st, _ := other.do("POST", "/api/auth/login", map[string]any{"username": "admin", "password": adminPassword, "code": code})
		return st
	}
	if st := login(first); st != 401 {
		t.Fatalf("the code used to enable it was accepted again: %d", st)
	}
	next, _ := totp.Code(secret, now+1) // one step of drift is allowed
	if st := login(next); st != 200 {
		t.Fatalf("a fresh code: %d", st)
	}
	if st := login(next); st != 401 {
		t.Fatalf("a code was accepted twice: %d", st)
	}
	recovery := codes[0].(string)
	if st := login(recovery); st != 200 {
		t.Fatalf("a recovery code: %d", st)
	}
	if st := login(recovery); st != 401 {
		t.Fatalf("a recovery code worked twice: %d", st)
	}

	// API tokens aren't affected.
	if code, _ := c.withToken(token).doList("GET", "/api/healthchecks"); code != 200 {
		t.Fatalf("token after 2FA: %d", code)
	}

	if code, _ := c.do("POST", "/api/auth/2fa/disable", map[string]any{"password": "nope"}); code != 400 {
		t.Fatalf("disabled with a wrong password: %d", code)
	}
	// An admin can reset it, e.g. for a user who lost their phone.
	if code, _ := c.do("POST", "/api/users/1/2fa/reset", map[string]any{}); code != 204 {
		t.Fatalf("reset: %d", code)
	}
	if code, _ := c.another().login("admin", adminPassword); code != 200 {
		t.Fatalf("after reset the password alone should do: %d", code)
	}
}
