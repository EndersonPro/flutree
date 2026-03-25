package ui

import (
	"os"

	"github.com/mattn/go-isatty"
)

func SupportsInteractiveWizard() bool {
	return isTerminalFile(os.Stdin) && isTerminalFile(os.Stdout)
}

func isTerminalFile(file *os.File) bool {
	if file == nil {
		return false
	}
	fd := file.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
