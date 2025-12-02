package cmd

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/spf13/cobra"
	keyval "github.com/tashima42/keyval/proto"
	"github.com/tashima42/keyval/server"
	"google.golang.org/grpc"
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
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", 50051))
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	keyval.RegisterStorerServer(s, &server.Server{})
	log.Printf("server listening at %v", lis.Addr())
	return s.Serve(lis)
}
