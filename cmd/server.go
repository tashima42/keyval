package cmd

import (
	"flag"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/spf13/cobra"
	keyval "github.com/tashima42/keyval/protos"
	"github.com/tashima42/keyval/server"
	"google.golang.org/grpc"
)

var (
	port  string
	nodes []string
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
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	s := grpc.NewServer()

	server, err := server.NewServer(&sync.Map{})
	if err != nil {
		return fmt.Errorf("failed to create new server: %s", err.Error())
	}

	keyval.RegisterStorerServer(s, server)
	log.Printf("server listening at %v", lis.Addr())
	return s.Serve(lis)
}
