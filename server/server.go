// Package server implements the keyval server that can append and get values from the store
package server

import (
	"context"
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
	minimumElectionTimeoutMS int32 = 300
	maximumElectionTimeoutMS int32 = 2 * minimumElectionTimeoutMS
)

type Server struct {
	kv.UnimplementedRaftServer
	CurrentTerm  *pUint32
	VotedFor     *pString
	Log          []*kv.Entry
	CommitIndex  *pUint32
	peers        []*Peer
	id           *pString
	leader       *pString
	lastApplied  *pUint32
	keyval       map[string]string
	state        *pServerState
	storer       *storer
	electionTick <-chan time.Time
}

type Peer struct {
	ID         *pString
	Address    *pString
	nextIndex  *pUint32
	matchIndex *pUint32
	votedFor   *pString
	grpcClient *kv.RaftClient
	mu         *sync.Mutex
}

func NewPeer(id, address string, grpcClient *kv.RaftClient) *Peer {
	return &Peer{
		ID:         &pString{v: id},
		Address:    &pString{v: address},
		nextIndex:  &pUint32{v: 0},
		matchIndex: &pUint32{v: 0},
		votedFor:   &pString{v: ""},
		grpcClient: grpcClient,
		mu:         &sync.Mutex{},
	}
}

func NewServer(id, serverStorePath string, peers []*Peer) (*Server, error) {
	log.Println("creating new server")

	storer := NewStorer(serverStorePath)

	s := &Server{
		CurrentTerm:  &pUint32{v: 0},
		VotedFor:     &pString{v: ""},
		CommitIndex:  &pUint32{v: 0},
		Log:          make([]*kv.Entry, 0),
		id:           &pString{v: id},
		leader:       &pString{v: ""},
		lastApplied:  &pUint32{v: 0},
		state:        &pServerState{v: serverStateFollower},
		keyval:       make(map[string]string),
		peers:        peers,
		storer:       storer,
		electionTick: nil,
	}

	// restoredServer, err := storer.Restore()
	// if err != nil && !errors.Is(err, ErrServerFileNotExist) {
	// 	return nil, err
	// }
	//
	// if restoredServer != nil {
	// 	log.Printf("restoring server from disk: %+v", *restoredServer)
	// 	// restore saved data
	// 	s.CurrentTerm = restoredServer.CurrentTerm
	// 	s.VotedFor = restoredServer.VotedFor
	// 	s.CommitIndex = restoredServer.CommitIndex
	// 	s.Log = restoredServer.Log
	// }

	log.Println("applying log")
	s.applyLog()

	return s, nil
}

func (s *Server) Start() {
	log.Println("starting server")
	s.resetElectionTimeout()
	go s.loop()
}

func (s *Server) applyLog() {
	for _, entry := range s.Log {
		switch entry.GetAction() {
		case kv.Actions_ADD:
			s.keyval[entry.GetRecord().GetKey()] = entry.GetRecord().GetValue()
		case kv.Actions_DELETE:
			delete(s.keyval, entry.GetRecord().GetKey())
		case kv.Actions_EMPTY:
			// do nothing
		}
	}
}

func (s *Server) AppendEntries(ctx context.Context, e *kv.AppendEntriesRequest) (*kv.AppendEntriesResponse, error) {
	s.resetElectionTimeout()
	// add mutex lock
	res := &kv.AppendEntriesResponse{Term: s.CurrentTerm.Get(), Success: false}
	log.Println("executing append entries procedure")

	if e.GetTerm() > s.CurrentTerm.Get() {
		log.Printf("request term bigger than current server term, reverting to follower state: %d > %d\n", e.GetTerm(), s.CurrentTerm)
		s.state.Set(serverStateFollower)
		return res, nil
	}

	if e.GetTerm() < s.CurrentTerm.Get() {
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

	if e.GetLeaderCommit() > s.CommitIndex.Get() {
		s.CommitIndex.Set(min(e.GetLeaderCommit(), s.CommitIndex.Get()))
	}

	res.Success = true
	return res, nil
}

func (s *Server) RequestVote(ctx context.Context, r *kv.RequestVoteRequest) (*kv.RequestVoteResponse, error) {
	log.Printf("received vote request: %+v\n", r)
	if r.GetTerm() > s.CurrentTerm.Get() {
		log.Printf("request term bigger than current term, reverting to follower: %d > %d\n", r.GetTerm(), s.CurrentTerm)
		s.CurrentTerm.Set(r.GetTerm())
		s.state.Set(serverStateFollower)
		s.VotedFor.Set("")
	}
	if r.GetTerm() < s.CurrentTerm.Get() {
		log.Printf("request term smaller than current term, denying vote: %d < %d\n", r.GetTerm(), s.CurrentTerm)
		return &kv.RequestVoteResponse{Term: s.CurrentTerm.Get(), VoteGranted: false}, nil
	}
	var lastTerm uint32 = 0
	if len(s.Log) > 0 {
		lastTerm = s.Log[len(s.Log)-1].GetTerm()
	}
	validLog := (r.GetLastLogTerm() > lastTerm) || (r.GetLastLogTerm() == lastTerm && r.GetLastLogIndex() >= s.CommitIndex.Get())
	if (s.VotedFor.Get() == "" || s.VotedFor.Get() == r.GetCandidateId()) && validLog && r.GetTerm() == s.CurrentTerm.Get() {
		s.VotedFor.Set(r.GetCandidateId())
		s.resetElectionTimeout()
		// if err := s.storer.Store(s); err != nil {
		// 	return nil, err
		// }
		return &kv.RequestVoteResponse{Term: s.CurrentTerm.Get(), VoteGranted: true}, nil
	}
	return &kv.RequestVoteResponse{Term: s.CurrentTerm.Get(), VoteGranted: false}, nil
}

func (s *Server) resetElectionTimeout() {
	n := rand.IntN(int(maximumElectionTimeoutMS - minimumElectionTimeoutMS))
	d := int(minimumElectionTimeoutMS) + n
	log.Printf("election timeout: %d\n", d)
	s.electionTick = time.NewTimer(time.Duration(d) * time.Millisecond).C
}

func (s *Server) requestVotes() error {
	var wg sync.WaitGroup

	for _, peer := range s.peers {
		peer.votedFor.Set("")
		log.Printf("requesting vote from peer: %s\n", peer.ID.Get())
		wg.Go(func() { s.requestVote(peer) })
	}

	wg.Wait()
	log.Println("exiting request vote")
	return nil
}

func (s *Server) requestVote(p *Peer) {
	c := *p.grpcClient
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*200)
	defer cancel()
	log.Printf("sending vote request rpc to %s\n", p.ID.Get())
	res, err := c.RequestVote(ctx, &kv.RequestVoteRequest{
		Term:         s.CurrentTerm.Get(),
		CandidateId:  s.id.Get(),
		LastLogIndex: s.lastApplied.Get(),
		LastLogTerm:  s.lastLogTerm(),
	})
	if err != nil {
		// tratar erro
		log.Printf("failed to request vote: %s\n", err.Error())
		return
	}

	// if RPC request or response contains term T > currentTerm: set current term = T, convert to follower 5.1
	log.Printf("checking terms %d > %d\n", res.GetTerm(), s.CurrentTerm.Get())
	if res.GetTerm() > s.CurrentTerm.Get() {
		s.CurrentTerm.Set(res.GetTerm())
		s.state.Set(serverStateFollower)
		s.resetElectionTimeout()
		// if err := s.storer.Store(s); err != nil {
		// 	log.Printf("falha ao guardar estado: %s\n", err.Error())
		// 	return
		// }
		return
	}

	log.Printf("checking if vote was granted: %t\n", res.GetVoteGranted())
	if res.GetVoteGranted() {
		log.Printf("vote granted from %s\n", p.ID.Get())
		p.votedFor.Set(p.ID.Get())
	}
}

// receivedMajorityOfVotes: A candidate wins an election if it receives votes
// from a majority of the servers in the full cluster for the same term.
// Only call this function after requesting votes
func (s *Server) receivedMajorityOfVotes() bool {
	votes := 0

	for _, p := range s.peers {
		if p.votedFor.Get() == s.id.Get() {
			votes++
		}
	}

	majority := len(s.peers)/2 + 1

	return votes >= majority
}

// func (s *Server) becomeLeader() {
// 	s.mu.Lock()
// 	defer s.mu.Unlock()
//
// 	votes := 0
// 	for _, p := range s.peers {
// 		if p.votedFor == s.id {
// 			log.Printf("adding vote: %s => %s\n", p.ID, p.votedFor)
// 			votes++
// 		}
// 	}
//
// 	log.Printf("checking peer votes: %d > %d\n", votes, len(s.peers)/2+1)
// 	if votes >= len(s.peers)/2+1 {
// 		for _, p := range s.peers {
// 			p.nextIndex = uint32(len(s.Log) + 1)
// 			p.matchIndex = 0
// 		}
//
// 		s.state = serverStateLeader
//
// 		s.Log = append(s.Log, &kv.Entry{Term: s.CurrentTerm, Action: kv.Actions_EMPTY, Record: nil})
// 		if err := s.storer.Store(s); err != nil {
// 			log.Printf("error storing server state:  %s\n", err.Error())
// 		}
//
// 		// s.heartbeatTimeout = time.Now()
// 	}
// }

func (s *Server) lastLogTerm() uint32 {
	if len(s.Log) <= 0 {
		return 0
	}
	return s.Log[len(s.Log)-1].Term
}

func (s *Server) loop() {
	for {
		log.Printf("server state: %d\n", s.state.Get())
		switch state := s.state.Get(); state {
		case serverStateFollower:
			s.followerState()
		case serverStateCandidate:
			s.candidateState()
		case serverStateLeader:
			s.leaderState()
		default:
			log.Fatalf("unkown server state: %d\n", s.state.Get())
		}
	}
}

func (s *Server) followerState() {
	for range s.electionTick {
		// 5.2  If a follower receives no communication over a period of time
		// called the election timeout, then it assumes there is no viable leader
		// and begins an election to choose a new leader.
		log.Println("election timeout, moving to candidate and starting election")
		// a follower increments its current term
		s.CurrentTerm.Set(s.CurrentTerm.Get() + 1)
		// and transitions to candidate state
		s.state.Set(serverStateCandidate)
		// it then votes for itself
		s.VotedFor.Set(s.id.Get())
		s.leader.Set("")
		s.resetElectionTimeout()
		return
	}
}

func (s *Server) candidateState() {
	if err := s.requestVotes(); err != nil {
		log.Fatalf("failed to request votes: %s\n", err.Error())
	}
	if s.receivedMajorityOfVotes() {
		s.state.Set(serverStateLeader)
		s.resetElectionTimeout()
	}
}

func (s *Server) leaderState() {
	log.Println("Leader state")
	time.Sleep(time.Second * 100)
	log.Fatal("Exiting, not implemented yet")
}
