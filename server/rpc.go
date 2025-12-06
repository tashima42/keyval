package server

import "net/rpc"

type AppendEntriesRequest struct {
	Term         uint32
	LeaderID     string
	PrevLogIndex uint32
	PrevLogTerm  uint32
	Entries      []*Entry
	LeaderCommit uint32
}

type AppendEntriesResponse struct {
	Term    uint32
	Success bool
}

type RequestVoteRequest struct {
	Term         uint32
	CandidateID  string
	LastLogIndex uint32
	LastLogTerm  uint32
}

type RequestVoteResponse struct {
	Term        uint32
	VoteGranted bool
}
type RaftClient struct {
	client *rpc.Client
	addr   string
}

func NewClient(addr string) *RaftClient {
	return &RaftClient{
		client: nil,
		addr:   addr,
	}
}

func (c *RaftClient) Close() error {
	return c.client.Close()
}

func (c *RaftClient) call(method string, req, reply any) error {
	if c.client == nil {
		client, err := rpc.Dial("tcp", c.addr)
		if err != nil {
			return err
		}
		c.client = client
	}
	return c.client.Call(method, req, reply)
}

func (c *RaftClient) SendAppendEntries(req AppendEntriesRequest) (*AppendEntriesResponse, error) {
	var reply AppendEntriesResponse
	// "Server" is the name of the struct registered via rpc.Register
	err := c.call("Server.AppendEntries", req, &reply)
	if err != nil {
		return nil, err
	}
	return &reply, nil
}

func (c *RaftClient) SendRequestVote(req RequestVoteRequest) (*RequestVoteResponse, error) {
	var reply RequestVoteResponse
	err := c.call("Server.RequestVote", req, &reply)
	if err != nil {
		return nil, err
	}
	return &reply, nil
}
