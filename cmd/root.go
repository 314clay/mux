package cmd

import (
	"fmt"
	"os"

	"mux/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mux",
	Short: "Tmux dispatch + Franklin grid control",
	Long:  `Interactive TUI for composing text, picking tmux sessions (local or Franklin), and sending. Also drives the Grid Server for monitor cell assignments.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		p := tea.NewProgram(tui.NewApp(), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("TUI error: %w", err)
		}
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
