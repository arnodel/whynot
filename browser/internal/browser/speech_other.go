//go:build !darwin || ios

package browser

import "errors"

// DefaultRecognizer returns this platform's Recognizer: none, here, so
// far.
func DefaultRecognizer() Recognizer { return noRecognizer{} }

type noRecognizer struct{}

func (noRecognizer) Available() bool { return false }

func (noRecognizer) Start(func(string, bool, error)) (Recording, error) { return nil, errNoSpeech }

// A Recognition turns speech into text: only on macOS, for now.
type Recognition struct{}

var errNoSpeech = errors.New("speech recognition isn't available here")

// StartRecognition isn't available here.
func StartRecognition(func(text string, final bool, err error)) (*Recognition, error) {
	return nil, errNoSpeech
}

// RecognizeFile isn't available here.
func RecognizeFile(string, func(text string, final bool, err error)) error { return errNoSpeech }

func (*Recognition) Stop() {}

func (*Recognition) Done() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}
