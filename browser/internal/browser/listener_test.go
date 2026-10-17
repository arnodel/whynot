package browser

import (
	"errors"
	"testing"
	"time"
)

// scriptedRecognition is a recognition a test drives: say delivers
// results, as the Speech framework would.
type scriptedRecognition struct {
	onText  func(string, bool, error)
	stopped bool
}

func (r *scriptedRecognition) Stop() { r.stopped = true }

func (r *scriptedRecognition) say(text string, final bool) { r.onText(text, final, nil) }

// scriptedRecognizer starts its one scriptedRecognition, or fails with err.
type scriptedRecognizer struct {
	rec *scriptedRecognition
	err error
}

func (r scriptedRecognizer) Available() bool { return true }

func (r scriptedRecognizer) Start(onText func(string, bool, error)) (Recording, error) {
	if r.err != nil {
		return nil, r.err
	}
	r.rec.onText = onText
	return r.rec, nil
}

func scriptedListener() (*Listener, *scriptedRecognition, *int) {
	rec := &scriptedRecognition{}
	woken := 0
	l := NewListener(scriptedRecognizer{rec: rec}, func() { woken++ })
	return l, rec, &woken
}

// TestListener checks a Listener's life: the prompt's text grows with
// what's heard, after what was typed; Stop leaves it busy until the final
// text is in; and Cancel drops what's still to come.
func TestListener(t *testing.T) {
	l, rec, woken := scriptedListener()
	if _, ok := l.Text(); ok || l.Busy() || l.Listening() {
		t.Fatal("a new Listener has text, or is busy")
	}
	l.Start("Buy")
	if !l.Listening() || !l.Busy() {
		t.Fatal("a started Listener isn't listening")
	}
	rec.say("milk", false)
	if text, ok := l.Text(); !ok || text != "Buy milk" {
		t.Errorf("the text is %q, %v, want what was typed, then what was heard: \"Buy milk\"", text, ok)
	}
	if _, ok := l.Text(); ok {
		t.Error("Text says the text changed again, when it didn't")
	}
	rec.say("milk and eggs", false)
	l.Stop()
	if !rec.stopped || l.Listening() || !l.Busy() {
		t.Errorf("after Stop: recognition stopped %v, listening %v, busy %v; want true, false, true", rec.stopped, l.Listening(), l.Busy())
	}
	rec.say("milk and eggs tomorrow", true)
	if text, _ := l.Text(); text != "Buy milk and eggs tomorrow" {
		t.Errorf("the final text is %q", text)
	}
	if l.Busy() {
		t.Error("still busy after the final text")
	}
	if *woken != 3 {
		t.Errorf("woken %d times, want once per result", *woken)
	}

	l.Start("")
	rec.say("hello", false)
	l.Text()
	l.Cancel()
	rec.say("hello there", true)
	if text, ok := l.Text(); ok {
		t.Errorf("after Cancel, the text changed to %q", text)
	}
	if l.Busy() {
		t.Error("still busy after a cancelled recognition ended")
	}
}

// TestListenerFailing checks that a recognition that can't start leaves
// the Listener idle.
func TestListenerFailing(t *testing.T) {
	l := NewListener(scriptedRecognizer{err: errors.New("no microphone")}, nil)
	l.Start("x")
	if l.Busy() || l.Listening() {
		t.Error("a Listener whose recognition failed to start is busy")
	}
}

// TestMicColor checks the microphone's background: pulsing red while
// listening, steady while finishing, none when idle.
func TestMicColor(t *testing.T) {
	l, rec, _ := scriptedListener()
	if _, ok := l.MicColor(time.Now()); ok {
		t.Error("an idle microphone has a background")
	}
	l.Start("")
	t0 := time.Unix(0, 0)
	a, okA := l.MicColor(t0)
	b, okB := l.MicColor(t0.Add(600 * time.Millisecond))
	if !okA || !okB || a == b || a.R <= a.G || b.R <= b.G {
		t.Errorf("listening: %v %v, %v %v; want two different reds", a, okA, b, okB)
	}
	l.Stop()
	c, okC := l.MicColor(t0)
	d, _ := l.MicColor(t0.Add(600 * time.Millisecond))
	if !okC || c != d {
		t.Errorf("finishing: %v %v, then %v; want a steady color", c, okC, d)
	}
	rec.say("done", true)
	if _, ok := l.MicColor(time.Now()); ok {
		t.Error("a finished microphone still has a background")
	}
}
