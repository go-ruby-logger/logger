// Copyright (c) the go-ruby-logger/logger authors
//
// SPDX-License-Identifier: BSD-3-Clause

package logger

import "os"

// defaultPid is the real process id, MRI's Process.pid. It is wrapped behind the
// Logger.Pid seam so tests can inject a fixed value and produce byte-stable lines.
func defaultPid() int { return os.Getpid() }
