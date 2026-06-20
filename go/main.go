package main

import (
	"fmt"
	"os"

	"owd-cli/bridge"
	"owd-cli/tui"

	tea "github.com/charmbracelet/bubbletea"
)

var p *tea.Program

func main() {
	root, err := bridge.FindWorkspaceRoot()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	model := tui.NewModel(root)
	p = tea.NewProgram(model, tea.WithAltScreen())
	tui.Program = p

	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
