package conformance

import (
	"fmt"
	"io"
	"testing"
)

// T is the part of *testing.T the suite uses. It exists so the same checks run under `go test`
// (through RunShape) and inside the acton-plugin CLI (through Verify), where there is no test
// binary and testing.RunTests would silently run nothing.
type T interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Fatal(args ...any)
	Run(name string, f func(t T)) bool
}

type testingT struct{ *testing.T }

func (w testingT) Run(name string, f func(t T)) bool {
	return w.T.Run(name, func(tt *testing.T) { f(testingT{tt}) })
}

// stop unwinds a Reporter's Fatalf the way runtime.Goexit unwinds a *testing.T.
type stop struct{}

// Reporter is a T that prints failures to a writer and remembers whether anything failed.
type Reporter struct {
	w      io.Writer
	name   string
	parent *Reporter
	failed bool
}

// NewReporter returns a Reporter that writes to w.
func NewReporter(w io.Writer) *Reporter { return &Reporter{w: w} }

// Failed reports whether this reporter, or any check run through it, failed.
func (r *Reporter) Failed() bool { return r.failed }

func (r *Reporter) Helper() {}

func (r *Reporter) fail(msg string) {
	for p := r; p != nil; p = p.parent {
		p.failed = true
	}
	fmt.Fprintf(r.w, "FAIL %s: %s\n", r.label(), msg)
}

func (r *Reporter) label() string {
	if r.name == "" {
		return "shape"
	}
	return r.name
}

func (r *Reporter) Errorf(format string, args ...any) { r.fail(fmt.Sprintf(format, args...)) }

func (r *Reporter) Fatalf(format string, args ...any) {
	r.fail(fmt.Sprintf(format, args...))
	panic(stop{})
}

func (r *Reporter) Fatal(args ...any) { r.Fatalf("%s", fmt.Sprint(args...)) }

// Run executes f as a named sub-check; a Fatalf inside it ends only that sub-check.
func (r *Reporter) Run(name string, f func(t T)) (ok bool) {
	child := &Reporter{w: r.w, name: name, parent: r}
	defer func() {
		if rec := recover(); rec != nil {
			if _, isStop := rec.(stop); !isStop {
				panic(rec)
			}
		}
		ok = !child.failed
	}()
	f(child)
	return
}
