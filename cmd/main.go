// Package cmd implements a cli that can control the keyval client and server
package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "keyval",
	Short: "KeyVal is a simple key-value store",
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&port, "port", "p", "5895", "server rpc bind port")

	serverCmd.Flags().StringSliceVarP(&nodes, "nodes", "n", []string{}, "cluster nodes bind addressess in 'ip:port' format")
	if err := serverCmd.MarkFlagRequired("nodes"); err != nil {
		log.Fatalf("failed to mark flag nodes as required: %s", err.Error())
	}

	clientCmd.AddCommand(clientAppendCmd)
	clientCmd.AddCommand(clientGetCmd)
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(clientCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
