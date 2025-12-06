// Package server implements the keyval server that can append and get values from the store
package server

import (
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"net/rpc"
	"sync"
	"time"
)

type serverState uint8

const (
	serverStateLeader    serverState = iota
	serverStateFollower  serverState = iota
	serverStateCandidate serverState = iota
)

type actions uint8

const (
	actionsAdd    actions = iota
	actionsDelete actions = iota
	actionsEmpty  actions = iota
)

// copied from https://github.com/peterbourgon/raft/blob/3e45f3d150111fb39d0b962ae51d3816fb170ee5/server.go#L29C1-L37C2
var (
	minimumElectionTimeoutMS int32 = 300
	maximumElectionTimeoutMS int32 = 2 * minimumElectionTimeoutMS
)

type Entry struct {
	Term   uint32
	Action actions
	Record Record
}

type Record struct {
	Key   string
	Value string
}

type Server struct {
	CurrentTerm  *pUint32
	VotedFor     *pString
	Log          []*Entry
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
	rpcClient  *RaftClient
	mu         *sync.Mutex
}

func NewPeer(id, address string) *Peer {
	return &Peer{
		ID:         &pString{v: id},
		Address:    &pString{v: address},
		nextIndex:  &pUint32{v: 0},
		matchIndex: &pUint32{v: 0},
		votedFor:   &pString{v: ""},
		rpcClient:  NewClient(address),
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
		Log:          make([]*Entry, 0),
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

func (s *Server) Start(port uint32) error {
	log.Println("starting server")

	// Register the server instance with net/rpc
	// Methods will be available as "Server.AppendEntries" etc.
	if err := rpc.Register(s); err != nil {
		return fmt.Errorf("failed to register rpc server: %v", err)
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	log.Printf("Raft Server listening on port %d", port)

	// Accept connections in a loop
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				log.Printf("listener accept error: %v", err)
				continue
			}
			go rpc.ServeConn(conn)
		}
	}()

	s.resetElectionTimeout()
	go s.loop()
	return nil
}

func (s *Server) applyLog() {
	for _, entry := range s.Log {
		switch entry.Action {
		case actionsAdd:
			s.keyval[entry.Record.Key] = entry.Record.Value
		case actionsDelete:
			delete(s.keyval, entry.Record.Key)
		case actionsEmpty:
			// do nothing
		}
	}
}

func (s *Server) AppendEntries(e *AppendEntriesRequest, res *AppendEntriesResponse) error {
	s.resetElectionTimeout()
	// add mutex lock
	res.Term = s.CurrentTerm.Get()
	res.Success = false

	log.Println("executing append entries procedure")

	if e.Term > s.CurrentTerm.Get() {
		log.Printf("request term bigger than current server term, reverting to follower state: %d > %d\n", e.Term, s.CurrentTerm)
		s.state.Set(serverStateFollower)
		return nil
	}

	if e.Term < s.CurrentTerm.Get() {
		log.Printf("current server term bigger than request term, returning false: %d > %d\n", s.CurrentTerm, e.Term)
		return nil
	}
	// add boundaries checks
	if e.PrevLogIndex > 0 && s.Log[e.PrevLogIndex].Term != e.PrevLogTerm {
		log.Printf("index exists, but terms are not equal, removing local entries: %d != %d", e.PrevLogIndex, s.Log[e.PrevLogIndex].Term)
		s.Log = s.Log[:e.PrevLogIndex]
		return nil
	}

	s.Log = append(s.Log, e.Entries...)

	if e.LeaderCommit > s.CommitIndex.Get() {
		s.CommitIndex.Set(min(e.LeaderCommit, s.CommitIndex.Get()))
	}

	res.Success = true
	return nil
}

func (s *Server) RequestVote(r *RequestVoteRequest, res *RequestVoteResponse) error {
	log.Printf("received vote request:\n  ID: %s\n  Term: %d\n  LastLogIndex: %d\n  LastLogTerm: %d\n", r.CandidateID, r.Term, r.LastLogIndex, r.LastLogTerm)
	if r.Term > s.CurrentTerm.Get() {
		log.Printf("request term bigger than current term, reverting to follower: %d > %d\n", r.Term, s.CurrentTerm.Get())
		s.CurrentTerm.Set(r.Term)
		s.state.Set(serverStateFollower)
		s.VotedFor.Set("")
	}
	if r.Term < s.CurrentTerm.Get() {
		log.Printf("request term smaller than current term, denying vote: %d < %d\n", r.Term, s.CurrentTerm.Get())

		res.Term = s.CurrentTerm.Get()
		res.VoteGranted = false
		return nil
	}
	var lastTerm uint32 = 0
	if len(s.Log) > 0 {
		lastTerm = s.Log[len(s.Log)-1].Term
	}
	if s.VotedFor.Get() != "" && s.VotedFor.Get() != r.CandidateID {

		res.Term = s.CurrentTerm.Get()
		res.VoteGranted = false
		return nil
	}

	if s.CommitIndex.Get() > r.LastLogIndex || lastTerm > r.LastLogTerm {

		res.Term = s.CurrentTerm.Get()
		res.VoteGranted = false
		return nil
	}

	log.Println("responding vote with true")

	res.Term = s.CurrentTerm.Get()
	res.VoteGranted = true
	return nil
}

func (s *Server) resetElectionTimeout() {
	n := rand.IntN(int(maximumElectionTimeoutMS - minimumElectionTimeoutMS))
	d := int(minimumElectionTimeoutMS) + n
	log.Printf("election timeout: %d\n", d)
	s.electionTick = time.NewTimer(time.Duration(d) * time.Millisecond).C
}

func (s *Server) requestVotes() chan bool {
	allVoted := make(chan bool)

	go func(allVoted chan bool) {
		var wg sync.WaitGroup

		for _, peer := range s.peers {
			peer.votedFor.Set("")
			log.Printf("requesting vote from peer: %s\n", peer.ID.Get())
			wg.Go(func() { s.requestVote(peer) })
		}
		wg.Wait()

		allVoted <- true
	}(allVoted)

	return allVoted
}

func (s *Server) requestVote(p *Peer) {
	c := *p.rpcClient
	log.Printf("sending vote request rpc to %s\n", p.ID.Get())
	res, err := c.SendRequestVote(RequestVoteRequest{
		Term:         s.CurrentTerm.Get(),
		CandidateID:  s.id.Get(),
		LastLogIndex: s.lastApplied.Get(),
		LastLogTerm:  s.lastLogTerm(),
	})
	if err != nil {
		// tratar erro
		log.Printf("failed to request vote: %s\n", err.Error())
		return
	}

	// if RPC request or response contains term T > currentTerm: set current term = T, convert to follower 5.1
	log.Printf("checking terms %d > %d\n", res.Term, s.CurrentTerm.Get())
	if res.Term > s.CurrentTerm.Get() {
		s.CurrentTerm.Set(res.Term)
		s.state.Set(serverStateFollower)
		s.resetElectionTimeout()
		// if err := s.storer.Store(s); err != nil {
		// 	log.Printf("falha ao guardar estado: %s\n", err.Error())
		// 	return
		// }
		return
	}

	log.Printf("checking if vote was granted: %t\n", res.VoteGranted)
	if res.VoteGranted {
		log.Printf("vote granted from %s\n", p.ID.Get())
		p.votedFor.Set(s.id.Get())
	}
}

// receivedMajorityOfVotes: A candidate wins an election if it receives votes
// from a majority of the servers in the full cluster for the same term.
// Only call this function after requesting votes
func (s *Server) receivedMajorityOfVotes() bool {
	log.Println("counting votes")
	votes := 0

	for _, p := range s.peers {
		log.Printf("checking peer vote: %s => %s\n", p.ID.Get(), p.votedFor.Get())
		if p.votedFor.Get() == s.id.Get() {
			log.Printf("peer voted for server: %s\n", p.ID.Get())
			votes++
		}
	}

	majority := len(s.peers)/2 + 1
	log.Printf("checking if votes are majority: %d >= %d\n", votes, majority)

	return votes >= majority
}

func (s *Server) lastLogTerm() uint32 {
	if len(s.Log) <= 0 {
		return 0
	}
	return s.Log[len(s.Log)-1].Term
}

func (s *Server) sendHeartBeats() {
	for _, peer := range s.peers {
		log.Printf("sending heartbeat to peer: %s\n", peer.ID.Get())
		go s.sendHeartBeat(peer)
	}
}

func (s *Server) sendHeartBeat(p *Peer) {
	c := *p.rpcClient
	log.Printf("sending append entries rpc to %s\n", p.ID.Get())
	res, err := c.SendAppendEntries(AppendEntriesRequest{
		Term:         s.CurrentTerm.Get(),
		LeaderID:     s.id.Get(),
		PrevLogIndex: s.CommitIndex.Get() - 1,
		PrevLogTerm:  s.lastLogTerm(),
		Entries:      []*Entry{},
		LeaderCommit: s.CommitIndex.Get(),
	})
	if err != nil {
		// tratar erro
		log.Printf("failed to request vote: %s\n", err.Error())
		return
	}
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
	allVoted := s.requestVotes()

	for range allVoted {
		log.Println("all peers voted")
		if s.receivedMajorityOfVotes() {
			// allVoted <- false
			log.Println("becoming leader")
			s.state.Set(serverStateLeader)
			s.resetElectionTimeout()
			return
		}
	}
}

func (s *Server) leaderState() {
	log.Println("Leader state")
	time.Sleep(time.Second * 10)
	log.Fatal("Exiting, not implemented yet")
}
