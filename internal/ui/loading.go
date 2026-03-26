package ui

import (
	"fmt"
	"os"
	"strings"
	"time"
)

var loadingFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func StartLoading(message string) func(success bool) {
	if !isTerminalFile(os.Stdout) {
		fmt.Println(message)
		return func(success bool) {}
	}

	stop := make(chan bool, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		ticker := time.NewTicker(90 * time.Millisecond)
		defer ticker.Stop()

		frame := 0
		for {
			select {
			case success := <-stop:
				line := strings.Repeat(" ", len(message)+4)
				fmt.Fprintf(os.Stdout, "\r%s\r", line)
				if success {
					fmt.Fprintf(os.Stdout, "✔ %s\n", message)
				} else {
					fmt.Fprintf(os.Stdout, "✖ %s\n", message)
				}
				return
			case <-ticker.C:
				fmt.Fprintf(os.Stdout, "\r%s %s", loadingFrames[frame%len(loadingFrames)], message)
				frame++
			}
		}
	}()

	return func(success bool) {
		stop <- success
		<-done
	}
}
