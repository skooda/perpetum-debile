package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"image/png"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getlantern/systray"
)

func main() {
	// Parse CLI flags
	cmdFlag := flag.String("cmd", "", "Shell command to execute repeatedly")
	delayFlag := flag.Duration("delay", 5*time.Second, "Pause between end of one run and start of next")
	timeoutFlag := flag.Duration("timeout", 10*time.Minute, "Max duration for a single command run")
	flag.Parse()

	// Validate required --cmd flag
	if *cmdFlag == "" {
		fmt.Fprintf(os.Stderr, "Error: --cmd flag is required\n")
		os.Exit(1)
	}

	// Warn if timeout <= delay
	if *timeoutFlag <= *delayFlag {
		fmt.Fprintf(os.Stderr, "Warning: --timeout (%v) is less than or equal to --delay (%v)\n", *timeoutFlag, *delayFlag)
	}

	// Validate all embedded PNG icons by attempting to decode
	iconBytes := map[string][]byte{
		"flame1": flame1PNG,
		"flame2": flame2PNG,
		"flame3": flame3PNG,
		"flame4": flame4PNG,
		"check":  checkPNG,
		"bang":   bangPNG,
	}

	for name, data := range iconBytes {
		if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to decode embedded PNG asset '%s': %v\n", name, err)
			os.Exit(1)
		}
	}

	// Create state channel for communication between Runner and Animator
	stateChan := make(chan State)

	// Set up signal handler for SIGTERM and SIGINT
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// Create a context that can be cancelled by the signal handler
	ctx, cancel := context.WithCancel(context.Background())

	// Launch Runner goroutine
	go runnerLoop(ctx, *cmdFlag, *delayFlag, *timeoutFlag, stateChan)

	// Initialize systray and start Animator goroutine
	systray.Run(
		func() {
			// On ready: start animator and set up menu
			go animatorLoop(ctx, stateChan)
			quitMenu := systray.AddMenuItem("Quit", "Quit the application")

			// Handle Quit menu item clicks
			go func() {
				<-quitMenu.ClickedCh
				cancel()
			}()
		},
		func() {
			// On exit: clean up
			cancel()
		},
	)

	// Handle signals
	go func() {
		sig := <-sigChan
		_ = sig // signal received

		// Cancel the context to kill any running child process
		cancel()

		// Wait for runner to exit (it will close the state channel)
		// Give it a short grace period
		time.Sleep(500 * time.Millisecond)

		// Call systray.Quit() to exit the application
		systray.Quit()
	}()
}
