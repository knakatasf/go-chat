package main

import (
	"chat/messages"
	"fmt"
	"log"
	"net"
	"os"
)

type client struct {
	msgHandler *messages.MessageHandler
	username   string
	out        chan *messages.Wrapper
	closed     chan struct{} // Unbuffered channel
}

func newClient(msgHandler *messages.MessageHandler, username string) *client {
	c := &client{
		username:   username,
		msgHandler: msgHandler,
		// Can hold up to 128 messages in this channel
		// Bounded queue -> But, we need a default clause otherwise goroutine blocks!!!
		out:    make(chan *messages.Wrapper, 128),
		closed: make(chan struct{}), // Signal channel
	}
	go c.writePump()
	return c
}

func (c *client) writePump() {
	defer close(c.closed)
	defer c.msgHandler.Close()

	for w := range c.out {
		if err := c.msgHandler.Send(w); err != nil {
			return
		}
	}
}

type addRequest struct {
	c        *client
	response chan error
}

type removeRequest struct {
	msgHandler *messages.MessageHandler
	response   chan *client
}

type broadcastRequest struct {
	w *messages.Wrapper
}

type registry struct {
	byConn map[*messages.MessageHandler]*client
	byName map[string]*client

	addChan       chan addRequest
	removeChan    chan removeRequest
	broadcastChan chan broadcastRequest
}

func newRegistry() *registry {
	r := &registry{
		byConn:        make(map[*messages.MessageHandler]*client),
		byName:        make(map[string]*client),
		addChan:       make(chan addRequest),
		removeChan:    make(chan removeRequest),
		broadcastChan: make(chan broadcastRequest, 1024),
	}
	go r.loop() // Single goroutine
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

			c := newClient(addReq.c.msgHandler, addReq.c.username)
			r.byName[addReq.c.username] = c
			r.byConn[addReq.c.msgHandler] = c
			addReq.response <- nil

		case removeReq := <-r.removeChan:
			var removed *client
			if c, ok := r.byConn[removeReq.msgHandler]; ok {
				delete(r.byConn, removeReq.msgHandler)
				delete(r.byName, c.username)
				close(c.out) // This is necessary!! writePump() goroutine is waiting a new message infinitely. We need to signal that there is no more new messages.
				<-c.closed   // We need this!! Because there might be leftover buffered messages in writePump()
				// <-c.closed(): receive operation. Normally, if nothing has been sent, it would block
				// But, if the channel is closed, <-channel immediately return zero value
				// In this program, it blocks until the closed channel is closed
				removed = c
			}
			removeReq.response <- removed

		case broadcastReq := <-r.broadcastChan:
			for _, c := range r.byConn {
				select {
				case c.out <- broadcastReq.w:
				default:
					// If c.out reaches 128 messages, the select default kicks in -> new messages are dropped
					// We don't want to pile any more messages in the out channel
					// If no default, this will block if c.out is full!!!
				}
			}
		}
	}
}

func (r *registry) add(c *client) error {
	res := make(chan error, 1)
	r.addChan <- addRequest{c: c, response: res}
	return <-res
}

func (r *registry) remove(msgHandler *messages.MessageHandler) *client {
	res := make(chan *client, 1)
	r.removeChan <- removeRequest{msgHandler: msgHandler, response: res}
	return <-res
}

func (r *registry) broadcast(w *messages.Wrapper) {
	r.broadcastChan <- broadcastRequest{w: w}
}

var users = newRegistry()

func notice(text string) *messages.Wrapper {
	msg := messages.Chat{Username: "server", MessageBody: text}
	return &messages.Wrapper{Msg: &messages.Wrapper_ChatMessage{ChatMessage: &msg}}
}

func handleClient(msgHandler *messages.MessageHandler) {
	defer func() {
		if c := users.remove(msgHandler); c != nil && c.username != "" {
			users.broadcast(notice(fmt.Sprintf("user %s has left the room", c.username)))
		}
		msgHandler.Close()
	}()

	first, err := msgHandler.Receive()
	if err != nil {
		log.Println("Registration read error: ", err)
		return
	}
	reg := first.GetRegistrationMessage()
	if reg == nil {
		_ = msgHandler.Send(notice("You must register first"))
		return
	}
	username := reg.GetUsername()
	if username == "" {
		_ = msgHandler.Send(notice("Username cannot be empty"))
		return
	}

	c := &client{msgHandler: msgHandler, username: username}
	if err := users.add(c); err != nil {
		_ = msgHandler.Send(notice("Registration failed: " + err.Error()))
		return
	}

	users.broadcast(notice(fmt.Sprintf("User %s has joined the room", username)))

	for {
		wrapper, _ := msgHandler.Receive()

		switch msg := wrapper.Msg.(type) {
		case *messages.Wrapper_RegistrationMessage:
			_ = msgHandler.Send(notice("Already registered as " + c.username))

		case *messages.Wrapper_ChatMessage:
			fmt.Println("<"+msg.ChatMessage.GetUsername()+"> ",
				msg.ChatMessage.MessageBody)
			users.broadcast(wrapper)

		case nil:
			log.Println("Received an empty message, terminating client")
			return
		default:
			log.Printf("Unexpected message type: %T", msg)
		}
	}
}

func main() {
	listener, err := net.Listen("tcp", ":"+os.Args[1])
	if err != nil {
		log.Fatalln(err.Error())
		return
	}

	for {
		if conn, err := listener.Accept(); err == nil {
			msgHandler := messages.NewMessageHandler(conn)
			go handleClient(msgHandler)
		}
	}
}
