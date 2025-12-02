package server

import (
	"context"
	"log"

	kv "github.com/tashima42/keyval/proto"
)

type Server struct {
	kv.UnimplementedStorerServer
}

func (s *Server) Append(_ context.Context, r *kv.Record) (*kv.Record, error) {
	log.Printf("Storing: %s: %s\n", r.GetKey(), r.GetValue())
	return r, nil
}
