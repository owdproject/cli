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
	if len(os.Args) > 1 && os.Args[1] == "--choose-starter" {
		target, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if len(os.Args) > 2 {
			target = os.Args[2]
		}
		choice, err := tui.ChooseStarter(target)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		switch choice {
		case "nuxt":
			os.Exit(20)
		case "react":
			os.Exit(21)
		default:
			os.Exit(22)
		}
	}
	root, err := bridge.FindWorkspaceRoot()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	model := tui.NewModel(root)
	p = tea.NewProgram(&model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	finalModel, err := p.Run()
	if err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}

	if m, ok := finalModel.(*tui.TuiModel); ok && m.ExitCode != 0 {
		os.Exit(m.ExitCode)
	}
}
