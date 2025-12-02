package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tashima42/keyval/client"
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Run the KeyVal client",
	RunE: func(cmd *cobra.Command, args []string) error {
		return client.Append()
	},
}
