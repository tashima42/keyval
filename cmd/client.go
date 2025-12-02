package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tashima42/keyval/client"
)

var clientCmd = &cobra.Command{
	Use:   "client [key] [value]",
	Short: "Run the KeyVal client",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("not enought args, expected 2, got %d", len(args))
		}
		return client.Append(args[0], args[1])
	},
}
