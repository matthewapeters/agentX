package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const widgetCorePIDEnv = "AGENTX_CORE_PID"

func widgetCommandContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
}

func resolveWidgetCorePIDFromEnv() int {
	raw := strings.TrimSpace(os.Getenv(widgetCorePIDEnv))
	if raw == "" {
		return 0
	}
	pid, err := strconv.Atoi(raw)
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	return true
}

func startWidgetCoreWatchdog(corePID int, interval time.Duration, logger io.Writer, onCoreExit func()) func() {
	if corePID <= 0 || onCoreExit == nil {
		return func() {}
	}
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}

	ctx, cancel := context.WithCancel(context.Background())
	var once sync.Once
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if isProcessAlive(corePID) {
					continue
				}
				if logger != nil {
					_, _ = io.WriteString(logger, "[widget] core process exited; shutting down widget\n")
				}
				once.Do(onCoreExit)
				return
			}
		}
	}()

	return func() {
		cancel()
		wg.Wait()
	}
}
