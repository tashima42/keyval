package cmd

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/spf13/cobra"
	keyval "github.com/tashima42/keyval/protos"
	"github.com/tashima42/keyval/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	s := grpc.NewServer()

	if len(ids) != len(addresses) {
		return fmt.Errorf("ids and addresses have different lengths: %d != %d", len(ids), len(addresses))
	}

	peers := make([]*server.Peer, len(ids))

	for i, id := range ids {
		address := addresses[i]
		conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("did not connect: %v", err)
		}
		c := keyval.NewRaftClient(conn)

		peers[i] = server.NewPeer(id, address, &c)
	}

	server, err := server.NewServer(id, serverStorePath, peers)
	if err != nil {
		return fmt.Errorf("failed to create new server: %s", err.Error())
	}

	keyval.RegisterRaftServer(s, server)
	log.Printf("server listening at %v", lis.Addr())
	server.Start()
	return s.Serve(lis)
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
