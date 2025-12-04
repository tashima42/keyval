// Package server implements the keyval server that can append and get values from the store
package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"sync"
	"time"

	kv "github.com/tashima42/keyval/protos"
)

type serverState uint8

const (
	serverStateLeader    serverState = iota
	serverStateFollower  serverState = iota
	serverStateCandidate serverState = iota
)

// copied from https://github.com/peterbourgon/raft/blob/3e45f3d150111fb39d0b962ae51d3816fb170ee5/server.go#L29C1-L37C2
var (
	minimumElectionTimeoutMS int32 = 250
	maximumElectionTimeoutMS int32 = 2 * minimumElectionTimeoutMS
	heartbeatMS              int32 = 300
)

type Server struct {
	kv.UnimplementedRaftServer
	CurrentTerm      uint32
	VotedFor         string
	Log              []*kv.Entry
	CommitIndex      uint32
	peers            []*Peer
	id               string
	lastApplied      uint32
	keyval           map[string]string
	state            serverState
	electionTimeout  time.Time
	heartbeatMS      int32
	heartbeatTimeout time.Time
	electionTick     <-chan time.Time
	storer           *storer
	mu               *sync.RWMutex
}

type Peer struct {
	ID         string
	Address    string
	nextIndex  uint32
	matchIndex uint32
	votedFor   string
	grpcClient *kv.RaftClient
}

func NewPeer(id, address string, grpcClient *kv.RaftClient) *Peer {
	return &Peer{ID: id, Address: address, nextIndex: 0, matchIndex: 0, votedFor: "", grpcClient: grpcClient}
}

func NewServer(id, serverStorePath string, peers []*Peer) (*Server, error) {
	storer := NewStorer(serverStorePath)

	s := &Server{
		CurrentTerm:  0,
		VotedFor:     "",
		CommitIndex:  0,
		Log:          make([]*kv.Entry, 0),
		id:           id,
		lastApplied:  0,
		state:        serverStateFollower,
		keyval:       make(map[string]string),
		peers:        peers,
		electionTick: nil,
		heartbeatMS:  heartbeatMS,
		storer:       storer,
		mu:           &sync.RWMutex{},
	}

	restoredServer, err := storer.Restore()
	if err != nil && !errors.Is(err, ErrServerFileNotExist) {
		return nil, err
	}

	if restoredServer != nil {
		// restore saved data
		s.CurrentTerm = restoredServer.CurrentTerm
		s.VotedFor = restoredServer.VotedFor
		s.CommitIndex = restoredServer.CommitIndex
		s.Log = restoredServer.Log
	}

	s.applyLog()

	return s, nil
}

func (s *Server) Start() {
	go s.loop()
}

func (s *Server) applyLog() {
	for _, entry := range s.Log {
		switch entry.GetAction() {
		case kv.Actions_ADD:
			s.keyval[entry.GetRecord().GetKey()] = entry.GetRecord().GetValue()
		case kv.Actions_DELETE:
			delete(s.keyval, entry.GetRecord().GetKey())
		}
	}
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
	if r.GetTerm() > s.CurrentTerm {
		s.CurrentTerm = r.GetTerm()
		s.state = serverStateFollower
		s.VotedFor = ""
	}
	if r.GetTerm() < s.CurrentTerm {
		return &kv.RequestVoteResponse{Term: s.CurrentTerm, VoteGranted: false}, nil
	}
	var lastTerm uint32 = 0
	if len(s.Log) > 0 {
		lastTerm = s.Log[len(s.Log)-1].GetTerm()
	}
	validLog := (r.GetLastLogTerm() > lastTerm) || (r.GetLastLogTerm() == lastTerm && r.GetLastLogIndex() >= s.CommitIndex)
	if (s.VotedFor == "" || s.VotedFor == r.GetCandidateId()) && validLog && r.GetTerm() == s.CurrentTerm {
		s.VotedFor = r.GetCandidateId()
		return &kv.RequestVoteResponse{Term: s.CurrentTerm, VoteGranted: true}, nil
	}
	return &kv.RequestVoteResponse{Term: s.CurrentTerm, VoteGranted: false}, nil
}

func (s *Server) resetElectionTimeout() {
	n := rand.IntN(int(maximumElectionTimeoutMS - minimumElectionTimeoutMS))
	d := int(minimumElectionTimeoutMS) + n
	s.electionTimeout = time.Now().Add(time.Duration(d) * time.Millisecond)
}

func (s *Server) timeout() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if time.Now().After(s.electionTimeout) {
		s.state = serverStateCandidate
		s.CurrentTerm++
		s.VotedFor = s.id
		for _, peer := range s.peers {
			peer.votedFor = ""
		}
		s.resetElectionTimeout()
		if err := s.storer.Store(s); err != nil {
			// add proper error handling
			log.Fatalf("failed storing server: %s", err.Error())
		}
	}
}

func (s *Server) requestVotes() error {
	for _, peer := range s.peers {
		c := *peer.grpcClient
		ctx, _ := context.WithTimeout(context.Background(), time.Millisecond*100)
		res, err := c.RequestVote(ctx, &kv.RequestVoteRequest{
			Term:         s.CurrentTerm,
			CandidateId:  s.id,
			LastLogIndex: s.lastApplied,
			LastLogTerm:  s.lastLogTerm(),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) lastLogTerm() uint32 {
	return s.Log[len(s.Log)-1].Term
}

func (s *Server) loop() {
	for {
		fmt.Println("loop")
		// switch s.state {
		// case serverStateLeader:
		// }
	}
}
