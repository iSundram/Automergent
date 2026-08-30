package shell

// Live output accessors for the TUI.
//
// The dock samples every running shell once a second. Before this file it did
// so by taking the session lock, copying the entire stdout buffer, splitting it
// on newlines and walking backwards for the newest non-empty line — per shell,
// per second. For a build that prints ten thousand lines that is ten thousand
// lines rescanned every tick, with the output pump blocked behind the lock the
// whole time.
//
// The fix is to compute the interesting facts once, on write, where the data is
// already in hand: the pump records the last non-empty line as it arrives and
// the dock reads a string.

import (
	"bytes"
	"strings"
	"sync/atomic"
)

// lastNonEmptyLine returns the last line of s with non-space content. Kept
// local rather than borrowed from the TUI's render package: a tool has no
// business depending on the presentation layer, and the function is four lines.
func lastNonEmptyLine(s string) string {
	s = strings.TrimRight(s, "\r\n")
	for {
		i := strings.LastIndexByte(s, '\n')
		if line := strings.TrimSpace(s[i+1:]); line != "" {
			return line
		}
		if i < 0 {
			return ""
		}
		s = s[:i]
	}
}

// tailLines returns the last n lines of s, oldest first, plus how many were
// dropped from the front.
func tailLines(s string, n int) (lines []string, hidden int) {
	if n <= 0 {
		return nil, 0
	}
	all := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(all) <= n {
		return all, 0
	}
	return all[len(all)-n:], len(all) - n
}

// noteStdout appends p to the session's stdout view and refreshes the cached
// tail. The write goes to the durable output file first, then the bounded
// RAM buffer (see trimBufferLocked). Callers must NOT hold the session lock.
func (s *AsyncSession) noteStdout(p []byte) {
	s.noteFileOutput(p)
	s.mu.Lock()
	s.Stdout.Write(p)
	s.trimBufferLocked(s.Stdout, &s.stdoutReadPos)
	if line := lastNonEmptyLine(string(p)); line != "" {
		s.lastLine.Store(&line)
	}
	s.mu.Unlock()
	s.noteGrowth()
}

// noteStderr appends p to the session's stderr view, refreshes the cached
// tail and raises the stderr flag. Callers must NOT hold the session lock.
func (s *AsyncSession) noteStderr(p []byte) {
	s.noteFileOutput(p)
	s.mu.Lock()
	s.Stderr.Write(p)
	s.trimBufferLocked(s.Stderr, &s.stderrReadPos)
	s.sawStderr.Store(true)
	if line := lastNonEmptyLine(string(p)); line != "" {
		s.lastLine.Store(&line)
	}
	s.mu.Unlock()
	s.noteGrowth()
}

// trimBufferLocked bounds ONE RAM output buffer: when it exceeds
// maxSessionBufferBytes the front is dropped down to half the cap and the
// stream's read position is adjusted (clamped at zero — the reader is told
// about the truncation and pointed at the output file). The durable copy in
// the file is untouched. Caller must hold the session lock.
func (s *AsyncSession) trimBufferLocked(buf *bytes.Buffer, readPos *int) {
	if buf.Len() <= maxSessionBufferBytes {
		return
	}
	drop := buf.Len() - maxSessionBufferBytes/2
	rest := append([]byte(nil), buf.Bytes()[drop:]...)
	buf.Reset()
	buf.Write(rest)
	s.truncated = true
	if *readPos > 0 {
		*readPos -= drop
		if *readPos < 0 {
			*readPos = 0
		}
	}
}

// LastLine returns the newest non-empty output line seen on either stream, or
// "" when the process has not said anything yet. It takes no lock and does no
// scanning, so the dock can call it for every session on every tick.
func (s *AsyncSession) LastLine() string {
	if p := s.lastLine.Load(); p != nil {
		return *p
	}
	return ""
}

// SawStderr reports whether anything has been written to stderr. Cheap for the
// same reason LastLine is.
func (s *AsyncSession) SawStderr() bool { return s.sawStderr.Load() }

// TailLines returns the last n lines of the merged output, oldest first, plus
// how many lines were dropped from the front. stderr is interleaved after
// stdout rather than merged chronologically — the two buffers carry no
// timestamps, so any claim to have restored the true order would be a guess.
func (s *AsyncSession) TailLines(n int) (lines []string, hidden int) {
	s.mu.Lock()
	out := s.Stdout.String()
	errOut := s.Stderr.String()
	s.mu.Unlock()

	combined := out
	if strings.TrimSpace(errOut) != "" {
		if combined != "" && !strings.HasSuffix(combined, "\n") {
			combined += "\n"
		}
		combined += errOut
	}
	if strings.TrimSpace(combined) == "" {
		return nil, 0
	}
	return tailLines(combined, n)
}

// atomicString is the pointer-swap holder LastLine reads from. A plain
// atomic.Value would work but rejects a type change on later stores; a typed
// pointer keeps the contract obvious.
type atomicString = atomic.Pointer[string]
