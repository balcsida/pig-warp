package pig_warp

// Tab-title activity spinner. upstream: rpiv-warp/title-spinner.ts. Warp's
// per-tab "moving dots" come from the foreground process rewriting its title
// (OSC 0); the title stack push/pop restores Pi's own title on stop.

import (
	"sync"
	"time"
)

// PiG's own working spinner (PiG tui/loader.go DefaultSpinnerFrames), not upstream's ⠴ ⠦ ⠖ ⠲.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// frameInterval is a variable so tests can shorten it.
var frameInterval = 80 * time.Millisecond // PiG: tui/loader.go DefaultLoaderIntervalMs

type spinner struct {
	mu   sync.Mutex
	stop chan struct{}
	done chan struct{}
}

// start is idempotent: a running ticker is kept (title-spinner.ts:97).
func (s *spinner) start(suffix string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop != nil {
		return
	}
	writeRaw(pushTitleStack)
	stop, done := make(chan struct{}), make(chan struct{})
	s.stop, s.done = stop, done
	go func() {
		defer close(done)
		t := time.NewTicker(frameInterval)
		defer t.Stop()
		for frame := 0; ; frame++ {
			select {
			case <-stop:
				return
			case <-t.C:
				writeRaw(formatOSC0(spinnerFrames[frame%len(spinnerFrames)] + suffix))
			}
		}
	}()
}

func (s *spinner) end() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop == nil {
		return
	}
	close(s.stop)
	<-s.done
	s.stop, s.done = nil, nil
	writeRaw(popTitleStack)
}
