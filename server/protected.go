package server

import "sync"

type pString struct {
	sync.RWMutex
	v string
}

type pServerState struct {
	sync.RWMutex
	v serverState
}

type pUint32 struct {
	sync.RWMutex
	v uint32
}

func (p *pString) Set(v string) {
	p.Lock()
	defer p.Unlock()
	p.v = v
}

func (p *pString) Get() string {
	p.RLock()
	defer p.RUnlock()
	return p.v
}

func (p *pServerState) Set(v serverState) {
	p.Lock()
	defer p.Unlock()
	p.v = v
}

func (p *pServerState) Get() serverState {
	p.RLock()
	defer p.RUnlock()
	return p.v
}

func (p *pUint32) Set(u uint32) {
	p.Lock()
	defer p.Unlock()
	p.v = u
}

func (p *pUint32) Get() uint32 {
	p.RLock()
	defer p.RUnlock()
	return p.v
}
