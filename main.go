// main.go
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getlantern/systray"
)

func main() {
	pathFlag := flag.String("path", "", "project directory to run claude in (required)")
	delayFlag := flag.Duration("delay", 5*time.Second, "pause between end of run and next start")
	timeoutFlag := flag.Duration("timeout", 10*time.Minute, "max duration per command run")
	flag.Parse()

	if *pathFlag == "" {
		fmt.Fprintln(os.Stderr, "error: --path is required")
		flag.Usage()
		os.Exit(1)
	}
	if *timeoutFlag <= *delayFlag {
		fmt.Fprintln(os.Stderr, "warning: --timeout should be greater than --delay")
	}

	validateIcons()

	ctx, cancel := context.WithCancel(context.Background())

	systray.Run(func() {
		systray.SetIcon(checkPNG)
		mQuit := systray.AddMenuItem("Quit", "Quit Perpetum Debile")

		runner := &Runner{path: *pathFlag, delay: *delayFlag, timeout: *timeoutFlag}
		states := make(chan State)

		go runner.Run(ctx, states)
		go NewAnimator(states).Run(ctx)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

		go func() {
			select {
			case <-mQuit.ClickedCh:
			case <-sigCh:
			}
			cancel()
			systray.Quit()
		}()
	}, func() {
		cancel()
	})
}

// validateIcons decodes all embedded PNGs at startup, fataling on any invalid icon.
func validateIcons() {
	icons := map[string][]byte{
		"flame1": flame1PNG,
		"flame2": flame2PNG,
		"flame3": flame3PNG,
		"flame4": flame4PNG,
		"check":  checkPNG,
		"bang":   bangPNG,
	}
	for name, data := range icons {
		if _, err := png.Decode(bytes.NewReader(data)); err != nil {
			log.Fatalf("invalid icon %q: %v", name, err)
		}
	}
}
