package main

import (
	"chat/messages"
	"fmt"
)

type registry struct {
	byConn map[*messages.MessageHandler]*client
	byName map[string]*client
	rooms  map[string]map[*client]struct{} // room -> set of members

	addChan           chan addRequest
	removeChan        chan removeRequest
	roomJoinChan      chan roomJoinRequest
	roomLeaveChan     chan roomLeaveRequest
	roomBroadcastChan chan roomBroadcastRequest
	directChan        chan directRequest
}

func newRegistry() *registry {
	r := &registry{
		byConn:            make(map[*messages.MessageHandler]*client),
		byName:            make(map[string]*client),
		rooms:             make(map[string]map[*client]struct{}),
		addChan:           make(chan addRequest),
		removeChan:        make(chan removeRequest),
		roomJoinChan:      make(chan roomJoinRequest),
		roomLeaveChan:     make(chan roomLeaveRequest),
		roomBroadcastChan: make(chan roomBroadcastRequest, 1024),
		directChan:        make(chan directRequest, 1024),
	}
	go r.loop()
	return r
}

func (r *registry) loop() {
	for {
		select {
		case addReq := <-r.addChan:
			if addReq.c.username == "" || r.byName[addReq.c.username] != nil {
				addReq.response <- fmt.Errorf("username is empty or already exists")
				continue
			}
			r.byName[addReq.c.username] = addReq.c
			r.byConn[addReq.c.msgHandler] = addReq.c
			addReq.response <- nil

		case removeReq := <-r.removeChan:
			var removed *client
			if c, ok := r.byConn[removeReq.msgHandler]; ok {
				delete(r.byConn, removeReq.msgHandler)
				delete(r.byName, c.username)
				// purge from all rooms
				for _, set := range r.rooms {
					delete(set, c)
				}
				close(c.out) // This is necessary!! writePump() goroutine is waiting a new message infinitely. We need to signal that there is no more new messages.
				<-c.closed   // We need this!! Because there might be leftover buffered messages in writePump()
				// <-c.closed(): receive operation. Normally, if nothing has been sent, it would block
				// But, if the channel is closed, <-channel immediately return zero value
				// In this program, it blocks until the closed channel is closed
				removed = c
			}
			removeReq.response <- removed

		case j := <-r.roomJoinChan:
			if j.room == "" {
				j.result <- fmt.Errorf("room name cannot be empty")
				continue
			}
			set := r.rooms[j.room]
			if set == nil {
				set = make(map[*client]struct{})
				r.rooms[j.room] = set
			}
			set[j.c] = struct{}{}
			j.result <- nil

		case l := <-r.roomLeaveChan:
			if set, ok := r.rooms[l.room]; ok {
				delete(set, l.c)
				if len(set) == 0 {
					delete(r.rooms, l.room)
				}
			}
			l.result <- nil

		case rb := <-r.roomBroadcastChan:
			if set, ok := r.rooms[rb.room]; ok {
				for c := range set {
					c.enqueue(rb.w)
				}
			}

		case dm := <-r.directChan:
			if c := r.byName[dm.to]; c != nil {
				c.enqueue(dm.w)
			}
		}
	}
}

func (r *registry) add(c *client) error {
	res := make(chan error, 1)
	r.addChan <- addRequest{c: c, response: res}
	return <-res
}

func (r *registry) remove(mh *messages.MessageHandler) *client {
	res := make(chan *client, 1)
	r.removeChan <- removeRequest{msgHandler: mh, response: res}
	return <-res
}

func (r *registry) joinRoom(c *client, room string) error {
	res := make(chan error, 1)
	r.roomJoinChan <- roomJoinRequest{c: c, room: room, result: res}
	return <-res
}

func (r *registry) leaveRoom(c *client, room string) error {
	res := make(chan error, 1)
	r.roomLeaveChan <- roomLeaveRequest{c: c, room: room, result: res}
	return <-res
}

func (r *registry) broadcastRoom(room string, w *messages.Wrapper) {
	r.roomBroadcastChan <- roomBroadcastRequest{room: room, w: w}
}

func (r *registry) direct(to string, w *messages.Wrapper) {
	r.directChan <- directRequest{to: to, w: w}
}
