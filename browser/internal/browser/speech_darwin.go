//go:build darwin && !ios

package browser

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// Speech recognition with Apple's Speech framework, through purego: the
// viewer records and transcribes itself, rather than relying on the
// system's dictation, which can't be stopped reliably from an app.

// DefaultRecognizer returns this platform's Recognizer: Apple's Speech
// framework, which works when the viewer runs from an app bundle, as its
// permission needs.
func DefaultRecognizer() Recognizer { return speechFramework{} }

type speechFramework struct{}

func (speechFramework) Available() bool { return speechAvailable }

func (speechFramework) Start(onText func(string, bool, error)) (Recording, error) {
	r, err := StartRecognition(onText)
	if err != nil {
		return nil, err // not a nil *Recognition in a Recording
	}
	return r, nil
}

var speechAvailable = inAppBundle()

// inAppBundle reports whether the running executable is an app bundle's.
func inAppBundle() bool {
	exe, err := os.Executable()
	return err == nil && strings.Contains(exe, ".app/Contents/MacOS/")
}

var (
	loadSpeechOnce sync.Once
	loadSpeechErr  error
)

// loadSpeech loads the frameworks speech recognition needs, once.
func loadSpeech() error {
	loadSpeechOnce.Do(func() {
		for _, f := range []string{
			"/System/Library/Frameworks/Foundation.framework/Foundation",
			"/System/Library/Frameworks/AVFoundation.framework/AVFoundation",
			"/System/Library/Frameworks/Speech.framework/Speech",
		} {
			if _, err := purego.Dlopen(f, purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
				loadSpeechErr = fmt.Errorf("loading %s: %w", f, err)
				return
			}
		}
	})
	return loadSpeechErr
}

func sel(name string) objc.SEL { return objc.RegisterName(name) }

func class(name string) objc.ID { return objc.ID(objc.GetClass(name)) }

func newObject(className string) objc.ID {
	return class(className).Send(sel("alloc")).Send(sel("init"))
}

// nsString returns s as an NSString, autoreleased.
func nsString(s string) objc.ID {
	return class("NSString").Send(sel("stringWithUTF8String:"), s)
}

// goString returns the NSString s as a Go string.
func goString(s objc.ID) string {
	if s == 0 {
		return ""
	}
	return objc.Send[string](s, sel("UTF8String"))
}

// errorText returns the NSError err's description.
func errorText(err objc.ID) string {
	return goString(err.Send(sel("localizedDescription")))
}

// Speech recognizer authorization statuses.
const (
	speechNotDetermined = 0
	speechDenied        = 1
	speechRestricted    = 2
	speechAuthorized    = 3
)

// authorizeSpeech asks the user, the first time, to let the app recognize
// speech, waiting up to timeout for the answer.
func authorizeSpeech(timeout time.Duration) error {
	recognizer := class("SFSpeechRecognizer")
	status := objc.Send[int](recognizer, sel("authorizationStatus"))
	if status == speechNotDetermined {
		answer := make(chan int, 1)
		block := objc.NewBlock(func(_ objc.Block, status int) {
			answer <- status
		})
		defer block.Release()
		recognizer.Send(sel("requestAuthorization:"), block)
		select {
		case status = <-answer:
		case <-time.After(timeout):
			return errors.New("no answer to the speech recognition permission request")
		}
	}
	switch status {
	case speechAuthorized:
		return nil
	case speechDenied:
		return errors.New("speech recognition isn't allowed: see System Settings, Privacy & Security, Speech Recognition")
	case speechRestricted:
		return errors.New("speech recognition is restricted on this Mac")
	}
	return fmt.Errorf("speech recognition authorization status %d", status)
}

// A Recognition turns speech into text, as it's spoken, until it's
// stopped: see StartRecognition.
type Recognition struct {
	engine, input, request, task objc.ID
	tap, handler                 objc.Block

	mu      sync.Mutex
	done    chan struct{} // closed with the final result, or an error
	stopped bool
}

// StartRecognition starts recognizing what's said into the microphone.
// onText receives the text so far, and again as it changes, with final
// true for the last time, once Stop has been called and the last words
// are in; or an error instead. onText is called on a goroutine of its own.
func StartRecognition(onText func(text string, final bool, err error)) (*Recognition, error) {
	request, err := newSpeechRequest(func() objc.ID {
		return newObject("SFSpeechAudioBufferRecognitionRequest")
	})
	if err != nil {
		return nil, err
	}
	r := &Recognition{request: request, done: make(chan struct{})}
	r.engine = newObject("AVAudioEngine")
	r.input = r.engine.Send(sel("inputNode"))
	format := r.input.Send(sel("outputFormatForBus:"), uint64(0))
	r.tap = objc.NewBlock(func(_ objc.Block, buffer, when objc.ID) {
		request.Send(sel("appendAudioPCMBuffer:"), buffer)
	})
	r.input.Send(sel("installTapOnBus:bufferSize:format:block:"), uint64(0), uint32(1024), format, r.tap)
	r.tap.Release() // the input node has its own copy
	r.engine.Send(sel("prepare"))
	var nserr objc.ID
	if !objc.Send[bool](r.engine, sel("startAndReturnError:"), &nserr) {
		r.input.Send(sel("removeTapOnBus:"), uint64(0))
		return nil, fmt.Errorf("starting the microphone: %s", errorText(nserr))
	}
	if err := r.recognize(onText); err != nil {
		r.Stop()
		return nil, err
	}
	return r, nil
}

// RecognizeFile recognizes the speech in the audio file at path, calling
// onText as StartRecognition does, and returns once the final text is in.
func RecognizeFile(path string, onText func(text string, final bool, err error)) error {
	url := class("NSURL").Send(sel("fileURLWithPath:"), nsString(path))
	request, err := newSpeechRequest(func() objc.ID {
		return class("SFSpeechURLRecognitionRequest").Send(sel("alloc")).Send(sel("initWithURL:"), url)
	})
	if err != nil {
		return err
	}
	r := &Recognition{request: request, done: make(chan struct{}), stopped: true}
	if err := r.recognize(onText); err != nil {
		return err
	}
	<-r.done
	return nil
}

// newSpeechRequest returns a recognition request made by newRequest,
// reporting partial results, once speech recognition is allowed.
func newSpeechRequest(newRequest func() objc.ID) (objc.ID, error) {
	if err := loadSpeech(); err != nil {
		return 0, err
	}
	if err := authorizeSpeech(time.Minute); err != nil {
		return 0, err
	}
	request := newRequest()
	if request == 0 {
		return 0, errors.New("speech recognition isn't available")
	}
	request.Send(sel("setShouldReportPartialResults:"), true)
	if objc.Send[bool](request, sel("respondsToSelector:"), sel("setAddsPunctuation:")) {
		// Sentences, rather than a stream of words: macOS 13 and later.
		request.Send(sel("setAddsPunctuation:"), true)
	}
	return request, nil
}

// recognize starts the recognition task for r's request.
func (r *Recognition) recognize(onText func(text string, final bool, err error)) error {
	recognizer := newObject("SFSpeechRecognizer")
	if recognizer == 0 || !objc.Send[bool](recognizer, sel("isAvailable")) {
		return errors.New("speech recognition isn't available for this language, or right now")
	}
	// Results on a queue of their own, rather than the main thread's: one
	// at a time, in order.
	queue := newObject("NSOperationQueue")
	queue.Send(sel("setMaxConcurrentOperationCount:"), 1)
	recognizer.Send(sel("setQueue:"), queue)
	onDevice := objc.Send[bool](recognizer, sel("supportsOnDeviceRecognition"))
	if onDevice {
		// Nothing leaves the Mac.
		r.request.Send(sel("setRequiresOnDeviceRecognition:"), true)
	}
	punctuation := objc.Send[bool](r.request, sel("respondsToSelector:"), sel("addsPunctuation")) &&
		objc.Send[bool](r.request, sel("addsPunctuation"))
	log.Printf("speech recognition: on the Mac %v, punctuation %v", onDevice, punctuation)
	// The recognizer ends an utterance at a pause, and its results then
	// start afresh with the next one: earlier utterances are kept in said,
	// and the text is said followed by the current one. A result ending
	// an utterance carries metadata.
	var said string
	hasMetadata := false
	r.handler = objc.NewBlock(func(_ objc.Block, result, nserr objc.ID) {
		var text string
		final := false
		if result != 0 {
			text = goString(result.Send(sel("bestTranscription")).Send(sel("formattedString")))
			final = objc.Send[bool](result, sel("isFinal"))
			if said != "" && !strings.HasPrefix(text, said) {
				// Some versions give the whole session: only a new
				// utterance goes after the earlier ones, as a sentence of
				// its own.
				text = joinSentences(said, text)
			}
			if !hasMetadata {
				hasMetadata = objc.Send[bool](result, sel("respondsToSelector:"), sel("speechRecognitionMetadata"))
			}
			if hasMetadata && result.Send(sel("speechRecognitionMetadata")) != 0 {
				said = text // an utterance ended
			}
		}
		var err error
		if nserr != 0 {
			err = errors.New(errorText(nserr))
		}
		r.mu.Lock()
		stopped := r.stopped
		r.mu.Unlock()
		if err != nil && stopped && text == "" {
			// Stopping before anything was said ends with an error.
			err = nil
			final = true
		}
		// The queue delivers results one at a time, in order.
		if result != 0 || err != nil || final {
			onText(text, final || err != nil, err)
		}
		if final || err != nil {
			r.finish()
		}
	})
	r.task = recognizer.Send(sel("recognitionTaskWithRequest:resultHandler:"), r.request, r.handler)
	r.handler.Release() // the task has its own copy
	if r.task == 0 {
		return errors.New("speech recognition didn't start")
	}
	return nil
}

// joinSentences returns a followed by b, with a full stop between them
// unless a ends with punctuation already.
func joinSentences(a, b string) string {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if a == "" || b == "" {
		return a + b
	}
	if !strings.ContainsAny(a[len(a)-1:], ".?!,;:") {
		a += "."
	}
	return a + " " + b
}

// finish closes done, once.
func (r *Recognition) finish() {
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case <-r.done:
	default:
		close(r.done)
	}
}

// Stop stops listening: the last words are recognized, and onText gets the
// final text. It can be called more than once.
func (r *Recognition) Stop() {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	r.mu.Unlock()
	if r.engine != 0 {
		r.engine.Send(sel("stop"))
		r.input.Send(sel("removeTapOnBus:"), uint64(0))
	}
	r.request.Send(sel("endAudio"))
	go func() {
		// Give up on a final result that never comes.
		select {
		case <-r.done:
		case <-time.After(5 * time.Second):
			if r.task != 0 {
				r.task.Send(sel("cancel"))
			}
			r.finish()
		}
	}()
}

// Done is closed once the final text has been delivered.
func (r *Recognition) Done() <-chan struct{} { return r.done }
