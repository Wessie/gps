package gps

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
)

const debug = false

type Plugin struct {
	Path string

	In  io.Reader
	Out io.Writer
	Cmd *exec.Cmd

	Controller *Controller

	in           chan *Message
	out          chan *Message
	messageID    uint64
	returnerLock *sync.Mutex
	returner     map[uint64]chan *Message
	waiter       chan struct{}
	active       int32
}

// NewPlugin returns a non-ready plugin for advanced use.
//
// The `Controller`, `In` and `Out` fields need to be set
// before `Run` can be called.
func newPlugin(c *Controller) *Plugin {
	return &Plugin{
		in:           make(chan *Message, 6),
		out:          make(chan *Message, 6),
		waiter:       make(chan struct{}, 1),
		returnerLock: new(sync.Mutex),
		returner:     make(map[uint64]chan *Message),
		Controller:   c,
	}
}

func (p *Plugin) Run() error {
	go p.serveIn()
	go p.serveOut()

	if p.Cmd == nil {
		return nil
	}

	err := p.Cmd.Start()

	return err
}

func (p *Plugin) Stop() error {
	err := p.Cmd.Process.Signal(os.Interrupt)

	if err != nil {
		return err
	}

	return p.Wait()
}

func (p *Plugin) Wait() error {
	<-p.waiter
	return p.Cmd.Wait()
}

func (p *Plugin) sendMessage(m *Message) chan []interface{} {
	m.ID = atomic.AddUint64(&p.messageID, 1)
	m.Result = false

	// Create a channel that we can get the result from later
	rc := make(chan *Message, 1)
	// Put it somewhere the serve loops can find it
	p.returnerLock.Lock()
	p.returner[m.ID] = rc
	p.returnerLock.Unlock()

	// A channel to return while we wait on a reply
	out := make(chan []interface{}, 1)
	go func() {
		// Send our message
		p.out <- m
		// Receive a response message, discard our message
		m := <-rc

		var values []interface{}
		for _, v := range m.Values {
			value := p.Controller.DecodeValue(v)

			values = append(values, value)
		}

		out <- values
	}()

	return out
}

func (p *Plugin) serveIn() {
	atomic.AddInt32(&p.active, 1)
	defer func() {
		if atomic.AddInt32(&p.active, -1) <= 0 {
			close(p.waiter)
		}
	}()

	d := gob.NewDecoder(p.In)

	var in = make([]interface{}, 0, 256)
	var m Message
	for {
		err := d.Decode(&m)
		if err != nil {
			if err == io.EOF {
				break
			} else {
				panic("decoding error")
			}
		}

		if m.Result {
			p.returnerLock.Lock()
			rc := p.returner[m.ID]
			p.returnerLock.Unlock()

			r := m
			rc <- &r
			continue
		}

		if debug {
			fmt.Printf("%p Received: %v\n", p, m)
		}

		in = in[:0]
		for _, v := range m.Values {
			i := p.Controller.DecodeValue(v)
			in = append(in, i)
		}

		var result = &Message{
			Result: true,
			ID:     m.ID,
			FuncID: m.FuncID,
		}

		// Call the requested function in a new goroutine
		go func() {
			r := p.Controller.callFunction(result.FuncID, in...)
			for _, i := range r {
				v, err := p.Controller.EncodeValue(i)
				if err != nil {
					panic(err)
				}

				result.Values = append(result.Values, v)
			}

			p.in <- result
		}()
	}
}

func (p *Plugin) serveOut() {
	e := gob.NewEncoder(p.Out)
	for {
		select {
		case m := <-p.in:
			if debug {
				fmt.Printf("%p Sending Result: %v\n", p, m)
			}

			if err := e.Encode(m); err != nil {
				panic(err)
			}
		case m := <-p.out:
			if debug {
				fmt.Printf("%p Sending: %v\n", p, m)
			}

			err := e.Encode(m)
			if err != nil {
				panic(err)
			}
		}
	}
}

type Message struct {
	Result bool
	ID     uint64
	FuncID string
	InstID uint64
	Values []*Value
}
