package browser

import (
	"image/color"
	"log"
	"math"
	"strings"
	"sync"
	"time"
)

// A Recognizer turns speech into text, as it's spoken. Each platform has
// its own (see DefaultRecognizer), and a viewer can use another, such as
// one running a speech model of its own.
//
// Start starts listening, and calls onText with what's been said so far:
// the whole text each time, which can change as more is heard, not just
// what's new. Calls come one at a time and in order, on a goroutine other
// than the caller's. Exactly one call has final true, the last: once the
// Recording has been stopped and the last words are in, or with an error
// that ended it.
type Recognizer interface {
	// Available reports whether the Recognizer can listen here, as when
	// the platform has speech recognition and the app may use it.
	Available() bool
	Start(onText func(text string, final bool, err error)) (Recording, error)
}

// A Recording is a Recognizer listening: Stop stops it, after which the
// Recognizer delivers the final text. Stop can be called more than once.
type Recording interface {
	Stop()
}

// A Listener is a viewer's microphone: it turns what's said into the
// prompt's text, as it's said, with a Recognizer. A viewer starts it with
// the prompt's text, and puts Text in the prompt whenever it changes.
type Listener struct {
	wake       func() // called when there's new text, from any goroutine
	recognizer Recognizer

	mu        sync.Mutex
	rec       Recording
	before    string // the prompt's text when listening started
	heard     string // what's been recognized since
	changed   bool   // heard has changed since Text last returned it
	listening bool   // until Stop
	busy      bool   // until the final text is in
	cancelled bool   // results are dropped
}

// NewListener returns a Listener using r, which calls wake, if not nil,
// when it has new text, as a viewer that draws only when asked needs.
func NewListener(r Recognizer, wake func()) *Listener {
	return &Listener{recognizer: r, wake: wake}
}

// Available reports whether the Listener can listen here: a viewer shows
// its microphone only then.
func (l *Listener) Available() bool { return l.recognizer.Available() }

// Listening reports whether the Listener is listening: it's been started,
// and not yet stopped.
func (l *Listener) Listening() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.listening
}

// Busy reports whether the Listener is listening, or has been stopped and
// is still recognizing the last words: the microphone shows as on until
// they're in, and text to send should wait for them.
func (l *Listener) Busy() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.busy
}

// Start starts listening, adding what's said to prompt, the prompt's text.
// It does nothing if the Listener is busy. If recognition can't start, it
// logs why.
func (l *Listener) Start(prompt string) {
	l.mu.Lock()
	if l.busy {
		l.mu.Unlock()
		return
	}
	if prompt != "" && !strings.HasSuffix(prompt, " ") {
		prompt += " "
	}
	l.before, l.heard, l.changed = prompt, "", false
	l.listening, l.busy, l.cancelled = true, true, false
	l.mu.Unlock()

	rec, err := l.recognizer.Start(l.onText)
	l.mu.Lock()
	if err != nil {
		l.listening, l.busy = false, false
	} else {
		l.rec = rec
	}
	l.mu.Unlock()
	if err != nil {
		log.Printf("listening: %v", err)
		l.notify()
	}
}

// onText takes a recognition's results.
func (l *Listener) onText(text string, final bool, err error) {
	l.mu.Lock()
	if err != nil {
		log.Printf("listening: %v", err)
	}
	if !l.cancelled && text != "" {
		l.heard, l.changed = text, true
	}
	if final {
		l.listening, l.busy, l.rec = false, false, nil
	}
	l.mu.Unlock()
	l.notify()
}

func (l *Listener) notify() {
	if l.wake != nil {
		l.wake()
	}
}

// Stop stops listening. The last words are still recognized, and Busy
// stays true until they're in.
func (l *Listener) Stop() {
	l.mu.Lock()
	rec := l.rec
	l.listening = false
	l.mu.Unlock()
	if rec != nil {
		rec.Stop()
	}
}

// Cancel stops listening and drops what's still to come, as when the prompt
// goes away.
func (l *Listener) Cancel() {
	l.mu.Lock()
	l.cancelled = true
	l.mu.Unlock()
	l.Stop()
}

// MicColor returns the microphone button's background, at now, and
// whether it has one: red, pulsing, while the Listener listens, so that
// it's plain that it's recording; steady and dimmer while it finishes
// recognizing the last words; none otherwise. The icon goes white on it.
func (l *Listener) MicColor(now time.Time) (color.RGBA, bool) {
	l.mu.Lock()
	listening, busy := l.listening, l.busy
	l.mu.Unlock()
	switch {
	case listening:
		const period = 1.2 // seconds
		phase := math.Mod(float64(now.UnixNano())/1e9, period) / period
		level := 0.55 + 0.45*(0.5+0.5*math.Cos(2*math.Pi*phase))
		return color.RGBA{uint8(0xD3 * level), uint8(0x2F * level), uint8(0x2F * level), 0xFF}, true
	case busy:
		return color.RGBA{0x7A, 0x22, 0x22, 0xFF}, true
	}
	return color.RGBA{}, false
}

// Text returns the prompt's text, if it has changed since Text last
// returned it: the text the Listener started with, then what's been said.
func (l *Listener) Text() (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.changed {
		return "", false
	}
	l.changed = false
	return l.before + l.heard, true
}
