package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tashima42/keyval/client"
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Run the KeyVal client",
}

var clientAppendCmd = &cobra.Command{
	Use:   "append [key] [value]",
	Short: "Append a key to the store",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("not enought args, expected 2, got %d", len(args))
		}
		return client.Add(args[0], args[1])
	},
}

var clientGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a value from the store",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("not enought args, expected 1, got %d", len(args))
		}
		return client.Get(args[0])
	},
}
