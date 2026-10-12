package main

import (
	"github.com/segfaultscribe/waford/internal"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Waford server",

	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := internal.LoadConfig(".")
		if err != nil {
			return err
		}

		return internal.Run(cfg)
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
