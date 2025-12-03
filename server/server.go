// Package server implements the keyval server that can append and get values from the store
package server

import (
	"context"
	"errors"
	"log"

	kv "github.com/tashima42/keyval/protos"
)

type serverState uint8

const (
	serverStateLeader    serverState = iota
	serverStateFollower  serverState = iota
	serverStateCandidate serverState = iota
)

type Server struct {
	kv.UnimplementedRaftServer
	CurrentTerm uint32
	VotedFor    string
	Log         []*kv.Entry
	CommitIndex uint32
	lastApplied uint32
	keyval      map[string]string
	nextIndex   map[string]uint32
	matchIndex  map[string]uint32
	state       serverState
}

func NewServer(serverStorePath string) (*Server, error) {
	storer := NewStorer(serverStorePath)

	server, err := storer.Restore()
	if err != nil && errors.Is(err, ErrServerFileNotExist) {
		return &Server{
			CurrentTerm: 0,
			VotedFor:    "",
			CommitIndex: 0,
			lastApplied: 0,
			state:       serverStateFollower,
			Log:         make([]*kv.Entry, 0),
			nextIndex:   make(map[string]uint32),
			matchIndex:  make(map[string]uint32),
			keyval:      make(map[string]string),
		}, nil
	}

	return server, err
}

func (s *Server) AppendEntries(ctx context.Context, e *kv.AppendEntriesRequest) (*kv.AppendEntriesResponse, error) {
	// add mutex lock
	res := &kv.AppendEntriesResponse{Term: s.CurrentTerm, Success: false}
	log.Println("executing append entries procedure")

	if e.GetTerm() > s.CurrentTerm {
		log.Printf("request term bigger than current server term, reverting to follower state: %d > %d\n", e.GetTerm(), s.CurrentTerm)
		s.state = serverStateFollower
		return res, nil
	}

	if e.GetTerm() < s.CurrentTerm {
		log.Printf("current server term bigger than request term, returning false: %d > %d\n", s.CurrentTerm, e.GetTerm())
		return res, nil
	}
	// add boundaries checks
	if e.GetPrevLogIndex() > 0 && s.Log[e.GetPrevLogIndex()].GetTerm() != e.GetPrevLogTerm() {
		log.Printf("index exists, but terms are not equal, removing local entries: %d != %d", e.GetPrevLogIndex(), s.Log[e.GetPrevLogIndex()].GetTerm())
		s.Log = s.Log[:e.GetPrevLogIndex()]
		return res, nil
	}

	s.Log = append(s.Log, e.GetEntries()...)

	if e.GetLeaderCommit() > s.CommitIndex {
		s.CommitIndex = min(e.GetLeaderCommit(), s.CommitIndex)
	}

	res.Success = true
	return res, nil
}

func (s *Server) RequestVote(ctx context.Context, r *kv.RequestVoteRequest) (*kv.RequestVoteResponse, error) {
	if r.GetTerm() < s.CurrentTerm {
		return &kv.RequestVoteResponse{Term: s.CurrentTerm, VoteGranted: false}, nil
	}
	if (s.VotedFor == "" || s.VotedFor == r.GetCandidateId()) && r.GetLastLogIndex() >= s.CommitIndex {
		return &kv.RequestVoteResponse{Term: s.CurrentTerm, VoteGranted: true}, nil
	}
	return nil, errors.New("could not grant or reject vote")
}
