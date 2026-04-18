package cmd

import (
	"fmt"

	"mux/internal/grid"

	"github.com/spf13/cobra"
)

var gridURL string

var gridCmd = &cobra.Command{
	Use:   "grid",
	Short: "Show Grid Server layout (all monitors + cells)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := grid.NewClient(gridURL)
		layouts, err := c.GetLayout()
		if err != nil {
			return fmt.Errorf("fetching layout: %w", err)
		}
		for _, l := range layouts {
			label := l.Label
			if label == "" {
				label = "Monitor " + l.Monitor.String()
			}
			fmt.Printf("\n%s [%s] (mode: %s)\n", label, l.Monitor, l.Mode)
			for _, p := range l.Panes {
				url := p.URL
				if len(url) > 60 {
					url = url[:57] + "..."
				}
				fmt.Printf("  %-4s %s\n", p.ID, url)
			}
		}
		fmt.Println()
		return nil
	},
}

var gridSetRaw bool

var gridSetCmd = &cobra.Command{
	Use:   "set <monitor> <cell> <session-or-url>",
	Short: "Assign a tmux session or URL to a grid cell",
	Long: `Assign content to a grid cell.

By default, the third argument is treated as a tmux session name and
wrapped in a ttyd URL. Use --raw to pass a URL directly.

Examples:
  mux grid set 0 A1 fizz                          # tmux session via ttyd
  mux grid set 0 A1 --raw http://localhost:9090/   # direct URL`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		monitor, cell, value := args[0], args[1], args[2]
		c := grid.NewClient(gridURL)

		url := value
		if !gridSetRaw {
			url = grid.TtydURL(value)
		}

		if err := c.SetCell(monitor, cell, url); err != nil {
			return err
		}
		fmt.Printf("Assigned %s/%s → %s\n", monitor, cell, url)
		return nil
	},
}

var gridOpenCmd = &cobra.Command{
	Use:   "open <monitor> <cell> <url>",
	Short: "Open a URL in a grid cell",
	Long:  `Shorthand for 'mux grid set --raw'. Opens any URL in a grid cell.`,
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		monitor, cell, url := args[0], args[1], args[2]
		c := grid.NewClient(gridURL)
		if err := c.SetCell(monitor, cell, url); err != nil {
			return err
		}
		fmt.Printf("Opened %s/%s → %s\n", monitor, cell, url)
		return nil
	},
}

var gridClearCmd = &cobra.Command{
	Use:   "clear <monitor> <cell>",
	Short: "Clear a grid cell (set to about:blank)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		monitor, cell := args[0], args[1]
		c := grid.NewClient(gridURL)
		if err := c.ClearCell(monitor, cell); err != nil {
			return err
		}
		fmt.Printf("Cleared %s/%s\n", monitor, cell)
		return nil
	},
}

func init() {
	gridCmd.PersistentFlags().StringVar(&gridURL, "url", grid.DefaultURL, "Grid Server URL")
	gridSetCmd.Flags().BoolVar(&gridSetRaw, "raw", false, "Treat value as a raw URL instead of a tmux session name")
	gridCmd.AddCommand(gridSetCmd)
	gridCmd.AddCommand(gridOpenCmd)
	gridCmd.AddCommand(gridClearCmd)
	rootCmd.AddCommand(gridCmd)
}
