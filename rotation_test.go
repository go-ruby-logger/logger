// Copyright (c) the go-ruby-logger/logger authors
//
// SPDX-License-Identifier: BSD-3-Clause

package logger

import (
	"testing"
	"time"
)

func TestShouldRotateBySize(t *testing.T) {
	cases := []struct {
		size, shiftSize int64
		shiftAge        int
		want            bool
	}{
		{2000, 1000, 7, true},   // over the limit
		{1000, 1000, 7, false},  // exactly the limit (> is strict)
		{500, 1000, 7, false},   // under
		{2000, 1000, 0, false},  // shift_age 0 never rotates by size
		{2000, 1000, -1, false}, // negative never rotates
	}
	for _, c := range cases {
		if got := ShouldRotateBySize(c.size, c.shiftSize, c.shiftAge); got != c.want {
			t.Errorf("ShouldRotateBySize(%d,%d,%d) = %v, want %v",
				c.size, c.shiftSize, c.shiftAge, got, c.want)
		}
	}
}

func TestShouldRotateByPeriod(t *testing.T) {
	next := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 6, 24, 23, 59, 59, 0, time.UTC)
	if ShouldRotateByPeriod(before, next) {
		t.Error("before next_rotate_time must not rotate")
	}
	if !ShouldRotateByPeriod(next, next) {
		t.Error("at next_rotate_time must rotate (>=)")
	}
	after := next.Add(time.Second)
	if !ShouldRotateByPeriod(after, next) {
		t.Error("after next_rotate_time must rotate")
	}
}

func TestShiftAgeSequence(t *testing.T) {
	// shift_age 7: rename .4->.5 .3->.4 .2->.3 .1->.2 .0->.1, then base->.0.
	moves := ShiftAgeSequence("app.log", 7)
	want := []ShiftMove{
		{"app.log.4", "app.log.5"},
		{"app.log.3", "app.log.4"},
		{"app.log.2", "app.log.3"},
		{"app.log.1", "app.log.2"},
		{"app.log.0", "app.log.1"},
		{"app.log", "app.log.0"},
	}
	if len(moves) != len(want) {
		t.Fatalf("got %d moves, want %d: %+v", len(moves), len(want), moves)
	}
	for i := range want {
		if moves[i] != want[i] {
			t.Errorf("move %d = %+v, want %+v", i, moves[i], want[i])
		}
	}

	// shift_age 3: (3-3).downto(0) is just i=0, so .0->.1 then base->.0.
	three := ShiftAgeSequence("app.log", 3)
	wantThree := []ShiftMove{{"app.log.0", "app.log.1"}, {"app.log", "app.log.0"}}
	if len(three) != 2 || three[0] != wantThree[0] || three[1] != wantThree[1] {
		t.Errorf("shift_age 3 sequence = %+v, want %+v", three, wantThree)
	}
	// shift_age 1 and 2: the downto range is empty, so only base->.0.
	for _, age := range []int{1, 2} {
		seq := ShiftAgeSequence("app.log", age)
		if len(seq) != 1 || seq[0] != (ShiftMove{"app.log", "app.log.0"}) {
			t.Errorf("shift_age %d sequence = %+v", age, seq)
		}
	}
}

func TestPeriodAgeFile(t *testing.T) {
	pe := time.Date(2026, 6, 23, 23, 59, 59, 0, time.UTC)

	// No collision: <filename>.<suffix>.
	got := PeriodAgeFile("app.log", pe, DefaultPeriodSuffix, func(string) bool { return false })
	if got != "app.log.20260623" {
		t.Errorf("no-collision = %q", got)
	}

	// Default suffix applied when empty.
	got = PeriodAgeFile("app.log", pe, "", func(string) bool { return false })
	if got != "app.log.20260623" {
		t.Errorf("empty-suffix default = %q", got)
	}

	// nil exists checker -> no collision handling.
	got = PeriodAgeFile("app.log", pe, "%Y%m%d", nil)
	if got != "app.log.20260623" {
		t.Errorf("nil-exists = %q", got)
	}

	// Collision: the base name and .1 are taken, .2 is free.
	taken := map[string]bool{"app.log.20260623": true, "app.log.20260623.1": true}
	got = PeriodAgeFile("app.log", pe, "%Y%m%d", func(s string) bool { return taken[s] })
	if got != "app.log.20260623.2" {
		t.Errorf("collision = %q, want app.log.20260623.2", got)
	}

	// Saturated: every candidate up to .99 taken -> loop exhausts at idx 99.
	got = PeriodAgeFile("app.log", pe, "%Y%m%d", func(string) bool { return true })
	if got != "app.log.20260623.99" {
		t.Errorf("saturated = %q, want app.log.20260623.99", got)
	}
}

// TestPeriodTimesVsMRI replays the daily/weekly/monthly next-rotate and
// previous-period-end values captured from MRI's Logger::Period across a spread
// of dates (computed in UTC so the arithmetic is timezone-stable). The "want"
// strings are the exact strftime("%Y-%m-%dT%H:%M:%S") MRI printed.
func TestPeriodTimesVsMRI(t *testing.T) {
	type row struct {
		y, mo, d, h, mi, s int
		period             Period
		wantNext           string
		wantPrev           string
	}
	rows := []row{
		// 2026-06-24 13:30 Wed
		{2026, 6, 24, 13, 30, 0, Daily, "2026-06-25T00:00:00", "2026-06-23T23:59:59"},
		{2026, 6, 24, 13, 30, 0, Weekly, "2026-06-28T00:00:00", "2026-06-20T23:59:59"},
		{2026, 6, 24, 13, 30, 0, Monthly, "2026-07-01T00:00:00", "2026-05-31T23:59:59"},
		// 2026-01-01 00:00:01 Thu (year/month boundary)
		{2026, 1, 1, 0, 0, 1, Daily, "2026-01-02T00:00:00", "2025-12-31T23:59:59"},
		{2026, 1, 1, 0, 0, 1, Weekly, "2026-01-04T00:00:00", "2025-12-27T23:59:59"},
		{2026, 1, 1, 0, 0, 1, Monthly, "2026-02-01T00:00:00", "2025-12-31T23:59:59"},
		// 2026-12-31 23:59:59 Thu
		{2026, 12, 31, 23, 59, 59, Daily, "2027-01-01T00:00:00", "2026-12-30T23:59:59"},
		{2026, 12, 31, 23, 59, 59, Weekly, "2027-01-03T00:00:00", "2026-12-26T23:59:59"},
		{2026, 12, 31, 23, 59, 59, Monthly, "2027-01-01T00:00:00", "2026-11-30T23:59:59"},
		// 2024-02-29 leap day Thu
		{2024, 2, 29, 8, 15, 0, Daily, "2024-03-01T00:00:00", "2024-02-28T23:59:59"},
		{2024, 2, 29, 8, 15, 0, Weekly, "2024-03-03T00:00:00", "2024-02-24T23:59:59"},
		{2024, 2, 29, 8, 15, 0, Monthly, "2024-03-01T00:00:00", "2024-01-31T23:59:59"},
		// 2026-06-28 00:00 Sunday (wday=0)
		{2026, 6, 28, 0, 0, 0, Daily, "2026-06-29T00:00:00", "2026-06-27T23:59:59"},
		{2026, 6, 28, 0, 0, 0, Weekly, "2026-07-05T00:00:00", "2026-06-27T23:59:59"},
		{2026, 6, 28, 0, 0, 0, Monthly, "2026-07-01T00:00:00", "2026-05-31T23:59:59"},
		// 2026-03-01 05:00 Sunday
		{2026, 3, 1, 5, 0, 0, Daily, "2026-03-02T00:00:00", "2026-02-28T23:59:59"},
		{2026, 3, 1, 5, 0, 0, Weekly, "2026-03-08T00:00:00", "2026-02-28T23:59:59"},
		{2026, 3, 1, 5, 0, 0, Monthly, "2026-04-01T00:00:00", "2026-02-28T23:59:59"},
	}
	const layout = "2006-01-02T15:04:05"
	for _, r := range rows {
		now := time.Date(r.y, time.Month(r.mo), r.d, r.h, r.mi, r.s, 0, time.UTC)
		nr, err := NextRotateTime(now, r.period)
		if err != nil {
			t.Fatalf("NextRotateTime(%v,%s): %v", now, r.period, err)
		}
		if got := nr.Format(layout); got != r.wantNext {
			t.Errorf("NextRotateTime(%v,%s) = %s, want %s", now, r.period, got, r.wantNext)
		}
		pe, err := PreviousPeriodEnd(now, r.period)
		if err != nil {
			t.Fatalf("PreviousPeriodEnd(%v,%s): %v", now, r.period, err)
		}
		if got := pe.Format(layout); got != r.wantPrev {
			t.Errorf("PreviousPeriodEnd(%v,%s) = %s, want %s", now, r.period, got, r.wantPrev)
		}
	}
}

func TestNextRotateTimeErrors(t *testing.T) {
	n := time.Date(2026, 6, 24, 13, 30, 0, 0, time.UTC)
	if _, err := NextRotateTime(n, "yearly"); err == nil {
		t.Error("NextRotateTime(yearly) should error")
	}
	if _, err := PreviousPeriodEnd(n, "yearly"); err == nil {
		t.Error("PreviousPeriodEnd(yearly) should error")
	}
}

func TestNowEverytimeReturnNow(t *testing.T) {
	n := time.Date(2026, 6, 24, 13, 30, 0, 0, time.UTC)
	for _, p := range []Period{Now, Everytime} {
		nr, err := NextRotateTime(n, p)
		if err != nil || !nr.Equal(n) {
			t.Errorf("NextRotateTime(%s) = %v err=%v, want now", p, nr, err)
		}
		pe, err := PreviousPeriodEnd(n, p)
		if err != nil || !pe.Equal(n) {
			t.Errorf("PreviousPeriodEnd(%s) = %v err=%v, want now", p, pe, err)
		}
	}
}

// TestNormalizeRotateDST exercises the day-boundary snap MRI applies when the
// +SiD arithmetic lands off midnight (the DST branch). We feed normalizeRotate
// directly because reproducing a real DST jump is location-dependent.
func TestNormalizeRotateDST(t *testing.T) {
	// 01:00 -> snaps back to that day's midnight (hour <= 12).
	early := time.Date(2026, 3, 8, 1, 0, 0, 0, time.UTC)
	if got := normalizeRotate(early); got.Hour() != 0 || got.Day() != 8 {
		t.Errorf("01:00 snap = %v, want 2026-03-08 00:00", got)
	}
	// 23:00 -> hour > 12, snaps forward to the next day's midnight.
	late := time.Date(2026, 3, 8, 23, 0, 0, 0, time.UTC)
	got := normalizeRotate(late)
	if got.Hour() != 0 || got.Day() != 9 {
		t.Errorf("23:00 snap = %v, want 2026-03-09 00:00", got)
	}
	// Already midnight -> unchanged.
	mid := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	if got := normalizeRotate(mid); !got.Equal(mid) {
		t.Errorf("midnight snap changed it: %v", got)
	}
}

func TestParsePeriod(t *testing.T) {
	cases := map[string]Period{
		"daily": Daily, "WEEKLY": Weekly, "Monthly": Monthly,
		"now": Now, "everytime": Everytime,
	}
	for in, want := range cases {
		got, err := ParsePeriod(in)
		if err != nil || got != want {
			t.Errorf("ParsePeriod(%q) = %v err=%v, want %v", in, got, err, want)
		}
	}
	if _, err := ParsePeriod("hourly"); err == nil {
		t.Error("ParsePeriod(hourly) should error")
	}
}

func TestDefaultsConstants(t *testing.T) {
	if DefaultShiftSize != 1048576 || DefaultShiftAge != 7 || DefaultPeriodSuffix != "%Y%m%d" {
		t.Error("rotation defaults drifted from MRI")
	}
}

func TestFracSecondsZeroWidth(t *testing.T) {
	// The defensive n<=0 branch of fracSeconds (unreachable via %0N, which Ruby
	// treats as full precision) returns empty.
	if got := fracSeconds(123, 0); got != "" {
		t.Errorf("fracSeconds(_,0) = %q, want empty", got)
	}
	if got := fracSeconds(123, -1); got != "" {
		t.Errorf("fracSeconds(_,-1) = %q, want empty", got)
	}
}
