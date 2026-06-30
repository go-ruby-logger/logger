<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-logger/brand/main/social/go-ruby-logger-logger.png" alt="go-ruby-logger/logger" width="720"></p>

# logger — go-ruby-logger

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-logger.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of the deterministic core of Ruby's
stdlib [`Logger`](https://docs.ruby-lang.org/en/master/Logger.html)** — the
severity model, the default `Logger::Formatter`, level gating, and the
`LogDevice` rotation **policy** of MRI 4.0.5 (the `logger-1.7.0` gem). It decides
**what bytes a log line is**, **whether a message is gated out by the level**, and
**whether — and to which filename — a log file should rotate**, all as pure
functions over an injected clock, pid and filesize, with **no IO and no Ruby
runtime**.

It is the logging backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby): rbgo wires the IO
**sink** (the `$stdout` / `File` that receives the formatted bytes), the wall
clock and the process id; this library is the **standalone, reusable** compute
core — a sibling of [go-ruby-regexp](https://github.com/go-ruby-regexp/regexp),
[go-ruby-erb](https://github.com/go-ruby-erb/erb) and
[go-ruby-yaml](https://github.com/go-ruby-yaml/yaml).

> **What it is — and isn't.** Building a log line, deciding the severity gate, and
> computing the rotation schedule and rotated-file names are fully deterministic
> and need **no interpreter**, so they live here as pure Go, validated against the
> `ruby` binary byte-for-byte. Opening the file, writing the bytes, renaming on
> rotation, and reading the live wall clock are the **host's** job: the library
> hands back the bytes and the rename plan, and the host's IO device performs them.

## Features

A faithful port of `Logger`'s deterministic core, validated against the `ruby`
binary on every supported platform:

- **Severity model** — `DEBUG=0 INFO=1 WARN=2 ERROR=3 FATAL=4 UNKNOWN=5`, the
  `SeverityLabel` mapping (`"DEBUG"`..`"FATAL"`, then `"ANY"` for `UNKNOWN` and
  anything out of range), and `CoerceSeverity` (MRI's `Severity.coerce`).
- **Default formatter, byte-for-byte** — MRI's
  `"%.1s, [%s #%d] %5s -- %s: %s\n"` with the
  `"%Y-%m-%dT%H:%M:%S.%6N"` timestamp, the `datetime_format` override, and the
  `msg2str` coercion (`String` as-is, `Exception` → `"message (Class)\nbacktrace"`,
  else the host's `inspect`). The clock and pid are **injected**, so a fixed
  instant + pid reproduce MRI's exact bytes.
- **Level gating** — `Add` (alias `Log`) returns `true` and formats nothing when
  there is no sink or `severity < level`; the `Debug`/`Info`/`Warn`/`Error`/
  `Fatal`/`Unknown` helpers, the `<<`-style raw `Write`, and the `DebugQ`..`FatalQ`
  predicates all mirror MRI.
- **Rotation policy (pure decision, no IO)** — for an integer `shift_age` +
  `shift_size`: `ShouldRotateBySize` and the `ShiftAgeSequence` rename plan; for a
  calendar `shift_age` (`"daily"`/`"weekly"`/`"monthly"`, plus `"now"`/
  `"everytime"`): `NextRotateTime`, `PreviousPeriodEnd`, `ShouldRotateByPeriod`,
  and the `PeriodAgeFile` rotated-name (with MRI's `.1`..`.99` collision suffix).

CGO-free, dependency-free, **100% test coverage**, `gofmt` + `go vet` clean, and
green across the six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le,
s390x) and three operating systems (Linux, macOS, Windows).

## Install

```sh
go get github.com/go-ruby-logger/logger
```

## Usage

```go
package main

import (
	"fmt"
	"os"

	"github.com/go-ruby-logger/logger"
)

func main() {
	// The host wires the IO sink (here, stdout). The clock and pid default to
	// the real ones; pass fixed ones for deterministic output.
	log := logger.New(func(line string) { fmt.Print(line) })
	log.Progname = "myapp"
	log.Level = logger.INFO

	log.Debug("filtered out (below INFO)", "")
	log.Info("server started", "")
	log.Warn("disk almost full", "")
	// I, [2026-06-30T09:12:01.004212 #4242]  INFO -- myapp: server started
	// W, [2026-06-30T09:12:01.004271 #4242]  WARN -- myapp: disk almost full

	// Rotation is a pure decision the host acts on.
	if logger.ShouldRotateBySize(fileSize(), logger.DefaultShiftSize, logger.DefaultShiftAge) {
		for _, mv := range logger.ShiftAgeSequence("app.log", logger.DefaultShiftAge) {
			os.Rename(mv.From, mv.To) // host performs the rename plan
		}
	}
}

func fileSize() int64 { return 2 << 20 }
```

## API

```go
// Severity model (logger/severity.rb).
type Severity int
const (DEBUG Severity = 0; INFO; WARN; ERROR; FATAL; UNKNOWN) // 0..5
func SeverityLabel(s Severity) string            // "DEBUG".."FATAL", else "ANY"
func CoerceSeverity(v any) (Severity, error)     // int or "warn"/"FATAL"/…

// Formatter (logger/formatter.rb) — pure over injected clock + pid.
const DefaultDatetimeFormat = "%Y-%m-%dT%H:%M:%S.%6N"
type Inspector func(msg any) string              // host msg.inspect
type Exception struct { Message, Class string; Backtrace []string }
type Formatter struct { DatetimeFormat string; Inspect Inspector }
func (f *Formatter) Format(severityLabel string, t time.Time, pid int, progname string, msg any) string

// Logger (logger.rb) minus the IO device.
type Clock func() time.Time
type Logger struct {
	Level     Severity
	Progname  string
	Formatter *Formatter
	Sink      func(string) // the host IO device; nil = no device
	Now       Clock        // injected clock; defaults to time.Now
	Pid       func() int   // injected pid;   defaults to os.Getpid
}
func New(sink func(string)) *Logger
func (l *Logger) Add(severity Severity, message any, progname string) bool // alias Log
func (l *Logger) Write(msg string) int            // Logger#<< (raw, returns n or -1)
func (l *Logger) Debug/Info/Warn/Error/Fatal/Unknown(message any, progname string) bool
func (l *Logger) DebugQ/InfoQ/WarnQ/ErrorQ/FatalQ() bool
func (l *Logger) SetLevel(v any) error            // Logger#level=

// Rotation policy (logger/log_device.rb + logger/period.rb) — pure decisions.
type Period string // Daily, Weekly, Monthly, Now, Everytime
const (DefaultShiftSize = 1048576; DefaultShiftAge = 7; DefaultPeriodSuffix = "%Y%m%d")
func ShouldRotateBySize(currentSize, shiftSize int64, shiftAge int) bool
func ShouldRotateByPeriod(now, nextRotate time.Time) bool
type ShiftMove struct { From, To string }
func ShiftAgeSequence(filename string, shiftAge int) []ShiftMove
func PeriodAgeFile(filename string, periodEnd time.Time, periodSuffix string, exists func(string) bool) string
func NextRotateTime(now time.Time, period Period) (time.Time, error)
func PreviousPeriodEnd(now time.Time, period Period) (time.Time, error)
func ParsePeriod(s string) (Period, error)
```

## What rbgo binds (the sink stays host-side)

| Concern                                   | Where it lives                |
| ----------------------------------------- | ----------------------------- |
| Severity numbers + labels + coercion      | **this library**              |
| Default-format line bytes (`msg2str`, ts) | **this library**              |
| Level gating (`Add` / predicates)         | **this library**              |
| Rotation *decision* + rename plan         | **this library**              |
| Wall clock / process id                   | host (injected via `Now`/`Pid`) |
| The IO device (open / write / rename)     | host (the `Sink` + the renames) |

## Tests & coverage

The suite pairs deterministic, ruby-free tests (which alone hold coverage at
100%, so the qemu cross-arch and Windows lanes pass the gate) with a
**differential MRI oracle**: the default-format line, the `datetime_format`
override, the `Exception` coercion, end-to-end level gating, the full severity
label range, and the daily/weekly/monthly rotation schedule are each generated
here and compared byte-for-byte against the system `ruby`. The oracle scripts
`$stdout.binmode` and stub `Process.pid`/`Time.now` to fixed values so the bytes
are stable, and skip themselves where `ruby` is absent.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-logger/logger authors.
