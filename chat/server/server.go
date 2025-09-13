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
		broadcastChan: make(chan broadcastRequest),
	}
	go r.loop() // Single goroutine
	return r
}

func (r *registry) loop() {
	for {
		select {
		case addReq := <-r.addChan:
			if addReq.c.username == "" {
				addReq.response <- fmt.Errorf("username is empty")
				continue
			}
			if _, exists := r.byName[addReq.c.username]; exists {
				addReq.response <- fmt.Errorf("username %s already exists", addReq.c.username)
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
				removed = c
			}
			removeReq.response <- removed

		case broadcastReq := <-r.broadcastChan:
			for _, c := range r.byConn {
				cc := c
				go func() { _ = cc.msgHandler.Send(broadcastReq.w) }()
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
