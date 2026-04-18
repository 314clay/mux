package cmd

import (
	"fmt"
	"strings"

	"mux/internal/tmux"

	"github.com/spf13/cobra"
)

var broadcastCmd = &cobra.Command{
	Use:   "broadcast <group> <message...>",
	Short: "Send text to all sessions in a tmux group",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		group := args[0]
		text := strings.Join(args[1:], " ")

		sessions, err := tmux.ListAll()
		if err != nil {
			return err
		}

		var targets []tmux.Session
		for _, s := range sessions {
			if s.Group == group {
				targets = append(targets, s)
			}
		}
		if len(targets) == 0 {
			return fmt.Errorf("no sessions in group %q", group)
		}

		var errs []string
		for _, s := range targets {
			if err := tmux.Send(s, text); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", s.Name, err))
			} else {
				fmt.Printf("Sent to %s (%s)\n", s.Name, s.Host)
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("some sends failed:\n  %s", strings.Join(errs, "\n  "))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(broadcastCmd)
}
