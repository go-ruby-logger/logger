// Copyright (c) the go-ruby-logger/logger authors
//
// SPDX-License-Identifier: BSD-3-Clause

package logger

import (
	"strconv"
	"strings"
	"time"
)

// strftime renders t with the subset of Ruby's Time#strftime directives that
// Logger's formatter and rotation-period suffix exercise. The default datetime
// format ("%Y-%m-%dT%H:%M:%S.%6N") and the default shift-period suffix
// ("%Y%m%d") are both covered, as are the directives a host is likely to pass
// via datetime_format=.
//
// The clock is injected as an ordinary time.Time, so the formatter is a pure
// function of (severity, time, pid, progname, msg): the host wires the real
// wall clock, the tests a fixed instant.
func strftime(t time.Time, format string) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' || i+1 >= len(format) {
			b.WriteByte(c)
			continue
		}
		i++
		// Optional width for the fractional-seconds directive, e.g. %6N.
		width := 0
		hasWidth := false
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			width = width*10 + int(format[i]-'0')
			hasWidth = true
			i++
		}
		if i >= len(format) {
			// A trailing '%' (possibly with digits) is emitted verbatim, as Ruby
			// leaves an incomplete directive untouched.
			b.WriteByte('%')
			if hasWidth {
				b.WriteString(strconv.Itoa(width))
			}
			break
		}
		switch format[i] {
		case 'Y':
			b.WriteString(strconv.Itoa(t.Year()))
		case 'y':
			b.WriteString(pad2(t.Year() % 100))
		case 'm':
			b.WriteString(pad2(int(t.Month())))
		case 'd':
			b.WriteString(pad2(t.Day()))
		case 'e':
			b.WriteString(padSpace2(t.Day()))
		case 'H':
			b.WriteString(pad2(t.Hour()))
		case 'I':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			b.WriteString(pad2(h))
		case 'M':
			b.WriteString(pad2(t.Minute()))
		case 'S':
			b.WriteString(pad2(t.Second()))
		case 'j':
			b.WriteString(pad3(t.YearDay()))
		case 'p':
			if t.Hour() < 12 {
				b.WriteString("AM")
			} else {
				b.WriteString("PM")
			}
		case 'P':
			if t.Hour() < 12 {
				b.WriteString("am")
			} else {
				b.WriteString("pm")
			}
		case 'A':
			b.WriteString(t.Weekday().String())
		case 'a':
			b.WriteString(t.Weekday().String()[:3])
		case 'B':
			b.WriteString(t.Month().String())
		case 'b', 'h':
			b.WriteString(t.Month().String()[:3])
		case 'z':
			b.WriteString(zoneOffset(t, false))
		case 'Z':
			name, _ := t.Zone()
			b.WriteString(name)
		case 'N', 'L':
			// Fractional seconds. %N defaults to 9 digits, %L to 3; an explicit
			// width (e.g. %6N) overrides. Ruby right-pads/truncates the
			// nanosecond count to the requested width.
			n := 9
			if format[i] == 'L' {
				n = 3
			}
			// Ruby treats a zero width as "no width", so %0N behaves like %N.
			if hasWidth && width != 0 {
				n = width
			}
			b.WriteString(fracSeconds(t.Nanosecond(), n))
		case '%':
			b.WriteByte('%')
		default:
			// Unknown directive: emit it verbatim (% + the char), matching Ruby's
			// lenient handling of unrecognised conversions.
			b.WriteByte('%')
			b.WriteByte(format[i])
		}
	}
	return b.String()
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func padSpace2(n int) string {
	if n < 10 {
		return " " + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func pad3(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

// fracSeconds renders the nanosecond count as exactly n digits, padding with
// trailing zeros when n > 9 and truncating (not rounding) when n < 9, which is
// what Ruby's %<n>N does.
func fracSeconds(nanos, n int) string {
	if n <= 0 {
		return ""
	}
	s := strconv.Itoa(nanos)
	for len(s) < 9 {
		s = "0" + s
	}
	if n <= 9 {
		return s[:n]
	}
	for len(s) < n {
		s += "0"
	}
	return s
}

// zoneOffset renders the numeric timezone offset as +HHMM (or +HH:MM when
// colon is true), matching Ruby's %z.
func zoneOffset(t time.Time, colon bool) string {
	_, off := t.Zone()
	sign := "+"
	if off < 0 {
		sign = "-"
		off = -off
	}
	h := off / 3600
	m := (off % 3600) / 60
	if colon {
		return sign + pad2(h) + ":" + pad2(m)
	}
	return sign + pad2(h) + pad2(m)
}
