package monitor

import (
	"testing"
	"time"
)

const testToken = "abcdefghijklmnop1234"

func heartbeat(h Heartbeat) Monitor {
	if h.Token == "" {
		h.Token = testToken
	}
	return Monitor{Kind: KindHeartbeat, Name: "job", Heartbeat: &h}
}

func TestHeartbeatNormalize(t *testing.T) {
	m := heartbeat(Heartbeat{Cron: "  0 3  * * * "})
	if err := m.Normalize(); err != nil {
		t.Fatal(err)
	}
	h := m.Heartbeat
	if h.Cron != "0 3 * * *" || h.Timezone != "UTC" || h.GraceSeconds != 300 {
		t.Fatalf("defaults: %+v", h)
	}

	bad := map[string]Heartbeat{
		"no schedule":     {},
		"both":            {EverySeconds: 3600, Cron: "@daily"},
		"too frequent":    {EverySeconds: 30},
		"bad cron":        {Cron: "every day"},
		"seconds in cron": {Cron: "0 0 3 * * *"},
		"tz in cron":      {Cron: "CRON_TZ=Europe/Berlin 0 3 * * *"},
		"bad zone":        {Cron: "@daily", Timezone: "Mars/Olympus"},
		"short grace":     {EverySeconds: 3600, GraceSeconds: 10},
		"short token":     {EverySeconds: 3600, Token: "abc"},
		"token chars":     {EverySeconds: 3600, Token: "abcdefghijklmnop/../x"},
	}
	for name, h := range bad {
		m := heartbeat(h)
		if err := m.Normalize(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestNextDue(t *testing.T) {
	at := time.Date(2026, 10, 1, 1, 30, 0, 0, time.UTC)

	every := Heartbeat{EverySeconds: 3600}
	if got := every.NextDue(at); !got.Equal(at.Add(time.Hour)) {
		t.Errorf("every hour: %v", got)
	}

	// 03:00 in Berlin is 01:00 UTC in October (CEST, UTC+2), so at 01:30 UTC
	// today's run has passed and the next is tomorrow.
	berlin := Heartbeat{Cron: "0 3 * * *", Timezone: "Europe/Berlin"}
	want := time.Date(2026, 10, 2, 1, 0, 0, 0, time.UTC)
	if got := berlin.NextDue(at); !got.Equal(want) {
		t.Errorf("cron in Berlin: got %v, want %v", got, want)
	}
	if up := berlin.Upcoming(at, 3); len(up) != 3 || !up[2].Equal(want.AddDate(0, 0, 2)) {
		t.Errorf("upcoming: %v", up)
	}
}

func TestHeartbeatDescribe(t *testing.T) {
	cases := map[string]Heartbeat{
		"every 5 minutes":           {EverySeconds: 300},
		"every hour":                {EverySeconds: 3600},
		"every 2 days":              {EverySeconds: 172800},
		"0 3 * * * (Europe/Berlin)": {Cron: "0 3 * * *", Timezone: "Europe/Berlin"},
		"@daily (UTC)":              {Cron: "@daily", Timezone: "UTC"},
	}
	for want, h := range cases {
		if got := h.Describe(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestHeartbeatStateStatus(t *testing.T) {
	for state, want := range map[HeartbeatState]Status{
		StateWaiting: StatusPending, StateOnTime: StatusUp, StateLate: StatusUp,
		StateMissed: StatusDown, StateFailed: StatusDown, StatePaused: StatusPaused,
	} {
		if got := state.Status(); got != want {
			t.Errorf("%s: got %s, want %s", state, got, want)
		}
	}
}
