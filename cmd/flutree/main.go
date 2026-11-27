package main

import (
	"os"

	"github.com/EndersonPro/flutree/internal/runtime"
)

func main() {
	state := &cliState{}
	root := newRootCmd(state)
	err := root.Execute()
	runtime.ExitOnErrorJSON(toAppError(err, state), jsonRequested(root, os.Args[1:]))
}
