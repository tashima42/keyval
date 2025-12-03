// Package client implements the keyval client that communicates with the keyval server
package client

import (
	"context"
	"fmt"
	"log"
	"time"

	kv "github.com/tashima42/keyval/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func connect() (*grpc.ClientConn, kv.StorerClient, error) {
	addr := "localhost:50051"
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	c := kv.NewStorerClient(conn)
	return conn, c, err
}

func Add(key, value string) error {
	conn, c, err := connect()
	if err != nil {
		return fmt.Errorf("did not connect: %v", err)
	}
	defer conn.Close()

	// Contact the server and print out its response.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Add(ctx, &kv.Record{Key: key, Value: value})
	if err != nil {
		return err
	}
	log.Printf("stored | %s: %s\n", res.GetKey(), res.GetValue())
	return nil
}

func Get(key string) error {
	conn, c, err := connect()
	if err != nil {
		return fmt.Errorf("did not connect: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	res, err := c.Get(ctx, &kv.RecordRequest{Key: key})
	if err != nil {
		return err
	}
	log.Printf("%s: %s\n", res.GetKey(), res.GetValue())

	return nil
}
