// Package server implements the keyval server that can append and get values from the store
package server

import (
	"context"
	"errors"
	"fmt"
	"sync"

	kv "github.com/tashima42/keyval/protos"
)

type serverState uint8

const (
	serverStateLeader    serverState = iota
	serverStateFollower  serverState = iota
	serverStateCandidate serverState = iota
)

type server struct {
	kv.UnimplementedStorerServer
	kv.UnimplementedRaftServer
	store       *sync.Map
	currentTerm uint32
	votedFor    string
	commitIndex uint32
	lastApplied uint32
	nextIndex   map[string]uint32
	matchIndex  map[string]uint32
	state       serverState
}

func NewServer(store *sync.Map) (*server, error) {
	if store == nil {
		return nil, errors.New("store is nil")
	}
	return &server{
		store:       store,
		currentTerm: 0,
		votedFor:    "",
		commitIndex: 0,
		lastApplied: 0,
		state:       serverStateFollower,
		nextIndex:   make(map[string]uint32),
		matchIndex:  make(map[string]uint32),
	}, nil
}

func (s *server) AppendEntries(ctx context.Context, r *kv.AppendEntriesRequest) (*kv.AppendEntriesResponse, error) {
	if r.GetTerm() < s.currentTerm {
		return &kv.AppendEntriesResponse{
			Term:    s.currentTerm,
			Success: false,
		}, nil
	}
	return &kv.AppendEntriesResponse{
		Term:    s.currentTerm,
		Success: true,
	}, nil
}

func (s *server) Get(ctx context.Context, r *kv.EntryRequest) (*kv.Entry, error) {
	value, exists := s.store.Load(r.GetKey())
	if !exists {
		return nil, fmt.Errorf("key not found: %s", r.GetKey())
	}
	v, ok := value.(string)
	if !ok {
		return nil, errors.New("value is not a string")
	}
	return &kv.Entry{Key: r.GetKey(), Value: v}, nil
}

func (s *server) append(e *kv.Entry) {
	s.store.Store(e.GetKey(), e.GetValue())
}
