package cmd

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tashima42/keyval/server"
)

var (
	serverStorePath string
	id              string
	ids             []string
	addresses       []string
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the KeyVal server",
	RunE: func(cmd *cobra.Command, args []string) error {
		return RunServer()
	},
}

func RunServer() error {
	flag.Parse()

	if len(ids) != len(addresses) {
		return fmt.Errorf("ids and addresses have different lengths: %d != %d", len(ids), len(addresses))
	}

	peers := make([]*server.Peer, len(ids))

	for i, id := range ids {
		address := addresses[i]

		peers[i] = server.NewPeer(id, address)
	}

	server, err := server.NewServer(id, serverStorePath, peers)
	if err != nil {
		return fmt.Errorf("failed to create new server: %s", err.Error())
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel() // 'stop' is a cleanup function to unregister signal notifications

	fmt.Println("Server running. Awaiting signal (SIGINT or SIGTERM).")

	go func() {
		if err := server.Start(port); err != nil {
			log.Fatalf("failed to start server: %s\n", err.Error())
		}
	}()

	for range ctx.Done() {
		// The context is done (a signal was received).
		fmt.Println("\nContext cancelled:", ctx.Err())
		// Perform graceful shutdown procedures here, e.g., shut down an HTTP server.
		fmt.Println("Starting graceful shutdown...")
		time.Sleep(2 * time.Second) // Simulate cleanup work
	}

	return nil
}

func initServerSubCmd() {
	serverCmd.Flags().StringVarP(&id, "id", "i", "", "server id")
	if err := serverCmd.MarkFlagRequired("id"); err != nil {
		log.Fatalf("failed to mark flag id as required: %s", err.Error())
	}

	serverCmd.Flags().StringSliceVarP(&ids, "ids", "d", []string{}, "peers ids, in the same order as the addresses")
	if err := serverCmd.MarkFlagRequired("ids"); err != nil {
		log.Fatalf("failed to mark flag ids as required: %s", err.Error())
	}

	serverCmd.Flags().StringSliceVarP(&addresses, "addresses", "a", []string{}, "peers addresses, in the same order as the ids")
	if err := serverCmd.MarkFlagRequired("ids"); err != nil {
		log.Fatalf("failed to mark flag addresses as required: %s", err.Error())
	}

	serverCmd.Flags().StringVarP(&serverStorePath, "store-path", "s", "store.gob", "location of the persistent storage files for the server")
}
