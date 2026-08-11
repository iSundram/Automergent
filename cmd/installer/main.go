package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/iSundram/Automergent/internal/installer"
)

func main() {
	p := tea.NewProgram(installer.NewModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
