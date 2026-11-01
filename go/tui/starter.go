package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type starterPicker struct {
	selected          int
	choice            string
	notice            string
	destination       string
	destinationStatus string
	blocked           bool
}

func starterForDirectory(path string) starterPicker {
	m := starterPicker{destination: path}
	if absolute, err := filepath.Abs(path); err == nil {
		m.destination = absolute
	}
	entries, err := os.ReadDir(m.destination)
	switch {
	case os.IsNotExist(err):
		m.destinationStatus = "New folder · ready to create"
	case err != nil:
		m.destinationStatus, m.blocked = "Cannot read this folder", true
	case len(entries) == 0:
		m.destinationStatus = "Empty folder · ready to create"
	default:
		m.destinationStatus, m.blocked = "Folder is not empty · choose an empty folder", true
	}
	return m
}

type starterOption struct {
	id, label string
	available bool
}

var starterOptions = []starterOption{
	{id: "nuxt", label: "Nuxt   · Vue desktop starter", available: true},
	{id: "react", label: "React  · starter repository not yet verified", available: true},
	{id: "vue", label: "Vue    · host coming later"},
	{id: "svelte", label: "Svelte · host coming later"},
	{id: "angular", label: "Angular · host coming later"},
	{id: "astro", label: "Astro  · host coming later"},
}

func (m starterPicker) Init() tea.Cmd { return nil }

func (m starterPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "up", "k":
		m.selected = (m.selected + len(starterOptions) - 1) % len(starterOptions)
		m.notice = ""
	case "down", "j":
		m.selected = (m.selected + 1) % len(starterOptions)
		m.notice = ""
	case "enter":
		if m.blocked {
			m.notice = "Nothing was changed. Choose an empty folder and try again."
			return m, nil
		}
		option := starterOptions[m.selected]
		if !option.available {
			m.notice = "This host starter is not available yet. Choose Nuxt or React."
			return m, nil
		}
		m.choice = option.id
		return m, tea.Quit
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m starterPicker) View() string {
	var lines []string
	for i, option := range starterOptions {
		marker := "  "
		if i == m.selected {
			marker = "› "
		}
		lines = append(lines, marker+option.label)
	}
	return fmt.Sprintf("\n  Open Web Desktop / Create workspace\n\n  Path: %s\n  %s\n\n  Choose your desktop\n\n%s\n\n  %s\n  ↑ ↓ choose  ·  Enter continue  ·  Esc cancel\n", m.destination, m.destinationStatus, strings.Join(lines, "\n"), m.notice)
}

// ChooseStarter uses the terminal directly; an empty choice means cancellation.
func ChooseStarter(target string) (string, error) {
	result, err := tea.NewProgram(starterForDirectory(target), tea.WithAltScreen()).Run()
	if err != nil {
		return "", err
	}
	return result.(starterPicker).choice, nil
}
