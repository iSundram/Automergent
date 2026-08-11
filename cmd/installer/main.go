package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/iSundram/Automergent/internal/installer"
)

func main() {
	// Enter alternate screen immediately before TUI starts to prevent
	// any flash of existing terminal content
	fmt.Print("\x1b[?1049h") // Enter alt screen
	fmt.Print("\x1b[H")      // Move cursor to home position
	fmt.Print("\x1b[2J")     // Clear entire screen

	model := installer.NewModel()
	// Get the installer path from os.Args[0]
	model.InstallerPath = os.Args[0]

	p := tea.NewProgram(model)
	_, err := p.Run()

	// Exit alternate screen after TUI ends
	fmt.Print("\x1b[?1049l")

	if err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
