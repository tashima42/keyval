// Package cmd implements a cli that can control the keyval client and server
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var port uint32

var rootCmd = &cobra.Command{
	Use:   "keyval",
	Short: "KeyVal is a simple key-value store",
}

func init() {
	rootCmd.PersistentFlags().Uint32VarP(&port, "port", "p", 5891, "server rpc bind port")

	initServerSubCmd()

	clientCmd.AddCommand(clientAppendCmd)
	clientCmd.AddCommand(clientGetCmd)
	clientCmd.AddCommand(clientAnalyzeServerCmd)
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(clientCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
