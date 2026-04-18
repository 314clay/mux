package cmd

import (
	"fmt"
	"strings"

	"mux/internal/tmux"

	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send <session> <message...>",
	Short: "Send text to a tmux session",
	Long: `Sends text to the named tmux session followed by Enter.
Checks local sessions first, then Franklin.`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		text := strings.Join(args[1:], " ")

		session, err := findSession(target)
		if err != nil {
			return err
		}

		if err := tmux.Send(session, text); err != nil {
			return err
		}
		fmt.Printf("Sent to %s (%s): %s\n", session.Name, session.Host, text)
		return nil
	},
}

func findSession(name string) (tmux.Session, error) {
	sessions, err := tmux.ListAll()
	if err != nil {
		return tmux.Session{}, err
	}
	for _, s := range sessions {
		if s.Name == name {
			return s, nil
		}
	}
	return tmux.Session{}, fmt.Errorf("session %q not found (local or Franklin)", name)
}

func init() {
	rootCmd.AddCommand(sendCmd)
}
