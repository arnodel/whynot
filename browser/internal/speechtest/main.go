// Command speechtest tries the viewers' speech recognition (see
// browser.StartRecognition): from an audio file with -file, or else from
// the microphone for a few seconds. On macOS, it must run from an app
// bundle, for the permission to recognize speech:
//
//	go build -o /tmp/speechtest ./internal/speechtest
//	go run ./internal/macapp -binary /tmp/speechtest -name speechtest -id io.github.arnodel.whynot.speechtest -out /tmp
//	/tmp/speechtest.app/Contents/MacOS/speechtest -file speech.aiff
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/arnodel/whynot/browser/internal/browser"
)

func main() {
	file := flag.String("file", "", "an audio file to recognize, instead of the microphone")
	listen := flag.Duration("for", 5*time.Second, "how long to listen to the microphone")
	flag.Parse()
	start := time.Now()
	onText := func(text string, final bool, err error) {
		at := time.Since(start).Seconds()
		switch {
		case err != nil:
			fmt.Printf("%5.2fs error: %v\n", at, err)
		case final:
			fmt.Printf("%5.2fs final: %q\n", at, text)
		default:
			fmt.Printf("%5.2fs partial: %q\n", at, text)
		}
	}
	if *file != "" {
		if err := browser.RecognizeFile(*file, onText); err != nil {
			log.Fatal(err)
		}
		return
	}
	r, err := browser.StartRecognition(onText)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("listening for %v\n", *listen)
	time.Sleep(*listen)
	fmt.Printf("%5.2fs stopping\n", time.Since(start).Seconds())
	stopped := time.Now()
	r.Stop()
	<-r.Done()
	fmt.Printf("final text %v after stopping\n", time.Since(stopped).Round(time.Millisecond))
}
