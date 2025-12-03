// Package server implements the keyval server that can append and get values from the store
package server

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	log         []*kv.Entry
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
		log:         make([]*kv.Entry, 0),
		nextIndex:   make(map[string]uint32),
		matchIndex:  make(map[string]uint32),
	}, nil
}

func (s *server) AppendEntries(ctx context.Context, e *kv.AppendEntriesRequest) (*kv.AppendEntriesResponse, error) {
	// add mutex lock
	res := &kv.AppendEntriesResponse{Term: s.currentTerm, Success: false}
	log.Println("executing append entries procedure")

	if e.GetTerm() > s.currentTerm {
		log.Printf("request term bigger than current server term, reverting to follower state: %d > %d\n", e.GetTerm(), s.currentTerm)
		s.state = serverStateFollower
		return res, nil
	}

	if e.GetTerm() < s.currentTerm {
		log.Printf("current server term bigger than request term, returning false: %d > %d\n", s.currentTerm, e.GetTerm())
		return res, nil
	}
	// add boundaries checks
	if e.GetPrevLogIndex() > 0 && s.log[e.GetPrevLogIndex()].GetTerm() != e.GetPrevLogTerm() {
		log.Printf("index exists, but terms are not equal, removing local entries: %d != %d", e.GetPrevLogIndex() != s.log[e.GetPrevLogIndex()].GetTerm())
		s.log = s.log[:e.GetPrevLogIndex()]
		return res, nil
	}

	s.log = append(s.log, e.GetEntries()...)

	if e.GetLeaderCommit() > s.commitIndex {
		s.commitIndex = min(e.GetLeaderCommit(), s.commitIndex)
	}

	res.Success = true
	return res, nil
}

func (s *server) RequestVote(ctx context.Context, r *kv.RequestVoteRequest) (*kv.RequestVoteResponse, error) {
	if r.GetTerm() < s.currentTerm {
		return &kv.RequestVoteResponse{Term: s.currentTerm, VoteGranted: false}, nil
	}
	if (s.votedFor == "" || s.votedFor == r.GetCandidateId()) && r.GetLastLogIndex() >= s.commitIndex {
		return &kv.RequestVoteResponse{Term: s.currentTerm, VoteGranted: true}, nil
	}
	return nil, errors.New("could not grant or reject vote")
}

func (s *server) Get(ctx context.Context, r *kv.RecordRequest) (*kv.Record, error) {
	value, exists := s.store.Load(r.GetKey())
	if !exists {
		return nil, fmt.Errorf("key not found: %s", r.GetKey())
	}
	v, ok := value.(string)
	if !ok {
		return nil, errors.New("value is not a string")
	}
	return &kv.Record{Key: r.GetKey(), Value: v}, nil
}

func (s *server) append(e *kv.Entry) {
	r := *e.GetRecord()
	s.store.Store(r.GetKey(), r.GetValue())
}
