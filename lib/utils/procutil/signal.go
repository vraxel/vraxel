//go:build !windows

package procutil

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"vraxel.io/vraxel/lib/logger"
)

var onlyOneSignalHandler = make(chan struct{})

// SetupSignalContext registers for SIGTERM and SIGINT.
// A context is returned which is cancelled on one of these signals.
// If a second signal is caught, the program is terminated with exit code 1.
func SetupSignalContext() context.Context {
	close(onlyOneSignalHandler) // panics if called twice

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-ch
		logger.Infof("received signal: %v, shutting down", sig)
		cancel()
		sig = <-ch
		logger.Infof("received second signal: %v, forcing exit", sig)
		os.Exit(1)
	}()
	return ctx
}

// WaitForSigterm waits for either SIGTERM or SIGINT
// Returns the caught signal
//
// It also prevents from program termination on SIGHUP signal,
// since this signal is frequently used for config reloading
func WaitForSigterm() os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	for {
		sig := <-ch
		if sig == syscall.SIGHUP {
			// Prevent from the program stop on SIGHUP
			continue
		}
		// Stop listening for SIGINT and SIGTERM signals,
		// so the app could be interrupted be sending these signals again
		// in the case if the caller doesn't finish the app gracefully
		signal.Stop(ch)
		return sig
	}
}

// SelfSIGHUP sends SIGHUP signal to the current process
func SelfSIGHUP() {
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
		logger.Panicf("FATAL: cannot send SIGHUP to itself: %s", err)
	}
}

// NewSighupChan returns a channel, which is triggered on every SIGHUP
func NewSighupChan() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	return ch
}
