package client

import (
	"context"
	"fmt"
	"log"
	"time"

	kv "github.com/tashima42/keyval/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func Append(key, value string) error {
	addr := "localhost:50051"
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("did not connect: %v", err)
	}
	defer conn.Close()
	c := kv.NewStorerClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	res, err := c.Append(ctx, &kv.Record{Key: key, Value: value})
	if err != nil {
		return err
	}
	log.Printf("stored | %s: %s\n", res.GetKey(), res.GetValue())
	return nil
}
