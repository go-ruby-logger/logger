// Copyright (c) the go-ruby-logger/logger authors
//
// SPDX-License-Identifier: BSD-3-Clause

package logger

import (
	"fmt"
	"strings"
	"time"
)

// Period is a calendar rotation cadence, MRI's :shift_age string/symbol form
// ("daily"/"weekly"/"monthly", plus "now"/"everytime").
type Period string

// The rotation periods MRI's Logger::Period accepts.
const (
	Daily     Period = "daily"
	Weekly    Period = "weekly"
	Monthly   Period = "monthly"
	Now       Period = "now"
	Everytime Period = "everytime"
)

// DefaultShiftSize is MRI's default @shift_size (1 MiB).
const DefaultShiftSize = 1048576

// DefaultShiftAge is MRI's default integer @shift_age (keep 7 old files).
const DefaultShiftAge = 7

// DefaultPeriodSuffix is MRI's default @shift_period_suffix ("%Y%m%d").
const DefaultPeriodSuffix = "%Y%m%d"

// siD is MRI's Logger::Period::SiD — the number of seconds in a day.
const siD = 24 * 60 * 60

// ShouldRotateBySize reports whether a size-based (integer shift_age) rotation is
// due. It mirrors Logger::LogDevice#check_shift_log's integer branch:
//
//	@filename && (@shift_age > 0) && (@dev.stat.size > @shift_size)
//
// The filesize is injected (the host stat'd the device); this is the pure
// decision only.
func ShouldRotateBySize(currentSize, shiftSize int64, shiftAge int) bool {
	return shiftAge > 0 && currentSize > shiftSize
}

// ShouldRotateByPeriod reports whether a calendar rotation is due, mirroring the
// period branch of check_shift_log: now >= @next_rotate_time.
func ShouldRotateByPeriod(now, nextRotate time.Time) bool {
	return !now.Before(nextRotate)
}

// ShiftAgeSequence returns the rename moves a size-based rotation performs, in
// order, mirroring Logger::LogDevice#shift_log_age:
//
//	(@shift_age-3).downto(0) { |i| rename "f.i" -> "f.i+1" if exist }
//	rename "f" -> "f.0"
//
// Each move is {From, To}; the host applies them (renaming only those whose From
// exists), then opens a fresh @filename. For a shift_age of n, the moves cover
// suffixes n-3 .. 0 plus the base file, keeping at most n-1 numbered backups.
// A shift_age of 3 or less yields just the base -> "filename.0" move.
func ShiftAgeSequence(filename string, shiftAge int) []ShiftMove {
	var moves []ShiftMove
	for i := shiftAge - 3; i >= 0; i-- {
		moves = append(moves, ShiftMove{
			From: fmt.Sprintf("%s.%d", filename, i),
			To:   fmt.Sprintf("%s.%d", filename, i+1),
		})
	}
	moves = append(moves, ShiftMove{From: filename, To: fmt.Sprintf("%s.0", filename)})
	return moves
}

// ShiftMove is one rename a rotation performs: From is renamed to To. From may
// not exist (the host skips those, as MRI's `if FileTest.exist?` does).
type ShiftMove struct {
	From string
	To   string
}

// PeriodAgeFile returns the name a period rotation would rename the current log
// to, mirroring Logger::LogDevice#shift_log_period. The base name is
// "<filename>.<suffix>", where suffix is periodEnd formatted with the period
// suffix; if that name is in the host-supplied taken set, MRI appends ".1",
// ".2", … (up to 99) until a free name is found. exists reports whether a
// candidate name is already taken (the host's FileTest.exist?).
func PeriodAgeFile(filename string, periodEnd time.Time, periodSuffix string, exists func(string) bool) string {
	if periodSuffix == "" {
		periodSuffix = DefaultPeriodSuffix
	}
	suffix := strftime(periodEnd, periodSuffix)
	ageFile := fmt.Sprintf("%s.%s", filename, suffix)
	if exists != nil && exists(ageFile) {
		for idx := 1; idx < 100; idx++ {
			ageFile = fmt.Sprintf("%s.%s.%d", filename, suffix, idx)
			if !exists(ageFile) {
				break
			}
		}
	}
	return ageFile
}

// NextRotateTime is a pure port of Logger::Period.next_rotate_time: it returns
// the next instant at which a calendar rotation of cadence period becomes due,
// relative to now. The arithmetic is performed in now's own location, so a host
// that wires local time gets MRI's Time.mktime behaviour. "now"/"everytime"
// return now unchanged. An unrecognised period is an error.
func NextRotateTime(now time.Time, period Period) (time.Time, error) {
	loc := now.Location()
	switch period {
	case Daily:
		t := midnight(now).AddDate(0, 0, 1)
		return normalizeRotate(t), nil
	case Weekly:
		t := midnight(now).Add(time.Duration(siD*(7-int(now.Weekday()))) * time.Second)
		return normalizeRotate(t), nil
	case Monthly:
		// Time.mktime(year, month, 1) + SiD*32, then truncated to the first of
		// that month — i.e. the first day of next month.
		first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		t := first.Add(time.Duration(siD*32) * time.Second)
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc), nil
	case Now, Everytime:
		return now, nil
	default:
		return time.Time{}, fmt.Errorf("invalid :shift_age %q, should be daily, weekly, monthly, or everytime", period)
	}
}

// normalizeRotate mirrors the tail of next_rotate_time that snaps a non-midnight
// candidate (produced when a DST jump lands the +SiD arithmetic off midnight)
// back to a day boundary, rounding up to the next day when past noon.
func normalizeRotate(t time.Time) time.Time {
	if t.Hour() != 0 || t.Minute() != 0 || t.Second() != 0 {
		hour := t.Hour()
		t = midnight(t)
		if hour > 12 {
			t = t.AddDate(0, 0, 1)
		}
	}
	return t
}

// PreviousPeriodEnd is a pure port of Logger::Period.previous_period_end: it
// returns 23:59:59 on the last day of the period preceding now, used to name the
// rotated file. "now"/"everytime" return now unchanged; an unknown period errors.
func PreviousPeriodEnd(now time.Time, period Period) (time.Time, error) {
	loc := now.Location()
	var t time.Time
	switch period {
	case Daily:
		t = midnight(now).Add(-time.Duration(siD/2) * time.Second)
	case Weekly:
		t = midnight(now).Add(-time.Duration(siD*int(now.Weekday())+siD/2) * time.Second)
	case Monthly:
		first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		t = first.Add(-time.Duration(siD/2) * time.Second)
	case Now, Everytime:
		return now, nil
	default:
		return time.Time{}, fmt.Errorf("invalid :shift_age %q, should be daily, weekly, monthly, or everytime", period)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, loc), nil
}

// midnight returns the start of t's calendar day in t's location, mirroring
// Time.mktime(t.year, t.month, t.mday).
func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// ParsePeriod coerces a host-supplied shift_age string/symbol into a Period,
// accepting the same spellings MRI's case statements do.
func ParsePeriod(s string) (Period, error) {
	switch strings.ToLower(s) {
	case "daily":
		return Daily, nil
	case "weekly":
		return Weekly, nil
	case "monthly":
		return Monthly, nil
	case "now":
		return Now, nil
	case "everytime":
		return Everytime, nil
	default:
		return "", fmt.Errorf("invalid :shift_age %q, should be daily, weekly, monthly, or everytime", s)
	}
}
