package rules

import (
	"testing"
	"time"
)

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

func TestNextAllowedInsideQuietHours(t *testing.T) {
	loc := mustLoad(t, "Asia/Jakarta")
	// 13:30 local sits inside a same-day 13:00-14:00 window.
	at := time.Date(2026, 8, 20, 6, 30, 0, 0, time.UTC)
	next, deferred := NextAllowed(at, loc, "13:00", "14:00")
	if !deferred {
		t.Fatal("13:30 local was not treated as quiet")
	}
	if want := time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Fatalf("next allowed = %s, want %s", next, want)
	}
}

func TestNextAllowedOutsideQuietHours(t *testing.T) {
	loc := mustLoad(t, "Asia/Jakarta")
	// 20:00 local is outside a 22:00-07:00 window, so nothing is deferred.
	at := time.Date(2026, 8, 20, 13, 0, 0, 0, time.UTC)
	next, deferred := NextAllowed(at, loc, "22:00", "07:00")
	if deferred {
		t.Fatalf("20:00 local was treated as quiet, deferred to %s", next)
	}
	if !next.Equal(at) {
		t.Fatalf("next allowed = %s, want the original instant %s", next, at)
	}
}

// A window that wraps past midnight has two legs, and both must defer to the same
// morning instant.
func TestNextAllowedWrapsMidnight(t *testing.T) {
	loc := mustLoad(t, "Asia/Jakarta")
	morning := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC) // 07:00 on 2026-08-21 local

	// Evening leg: 23:30 local on 2026-08-20 defers to 07:00 the next local day.
	evening := time.Date(2026, 8, 20, 16, 30, 0, 0, time.UTC)
	next, deferred := NextAllowed(evening, loc, "22:00", "07:00")
	if !deferred {
		t.Fatal("23:30 local was not treated as quiet")
	}
	if !next.Equal(morning) {
		t.Fatalf("evening leg deferred to %s, want %s", next, morning)
	}

	// Morning leg: 02:00 local on 2026-08-21 is still inside the same window.
	early := time.Date(2026, 8, 20, 19, 0, 0, 0, time.UTC)
	next, deferred = NextAllowed(early, loc, "22:00", "07:00")
	if !deferred {
		t.Fatal("02:00 local was not treated as quiet")
	}
	if !next.Equal(morning) {
		t.Fatalf("morning leg deferred to %s, want %s", next, morning)
	}
}

// The boundaries are half-open: a notification at the opening minute is quiet, one at
// the closing minute is not.
func TestNextAllowedBoundaries(t *testing.T) {
	loc := time.UTC
	if _, deferred := NextAllowed(time.Date(2026, 8, 20, 22, 0, 0, 0, loc), loc, "22:00", "07:00"); !deferred {
		t.Error("the opening minute was not treated as quiet")
	}
	if _, deferred := NextAllowed(time.Date(2026, 8, 20, 7, 0, 0, 0, loc), loc, "22:00", "07:00"); deferred {
		t.Error("the closing minute was treated as quiet")
	}
}

func TestNextAllowedRejectsUnusableWindows(t *testing.T) {
	loc := time.UTC
	at := time.Date(2026, 8, 20, 23, 0, 0, 0, loc)
	for _, window := range [][2]string{{"", ""}, {"22:00", ""}, {"bad", "07:00"}, {"22:00", "22:00"}} {
		if _, deferred := NextAllowed(at, loc, window[0], window[1]); deferred {
			t.Errorf("window %q-%q was treated as quiet", window[0], window[1])
		}
		if ValidQuietHours(window[0], window[1]) {
			t.Errorf("window %q-%q passed validation", window[0], window[1])
		}
	}
	if !ValidQuietHours("22:00", "07:00") {
		t.Error("a valid overnight window was rejected")
	}
}
