package main

import (
	"chat/messages"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
)

type client struct {
	msgHandler *messages.MessageHandler
	username   string
}

type registry struct {
	mutex  sync.RWMutex
	byConn map[*messages.MessageHandler]*client
	byName map[string]*client
}

func newRegistry() *registry {
	return &registry{
		byConn: make(map[*messages.MessageHandler]*client),
		byName: make(map[string]*client),
	}
}

func (r *registry) add(c *client) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if c.username == "" {
		return fmt.Errorf("username is empty")
	}
	if _, exists := r.byName[c.username]; exists {
		return fmt.Errorf("user %s already exists", c.username)
	}
	r.byConn[c.msgHandler] = c
	r.byName[c.username] = c
	return nil
}

func (r *registry) remove(msgHandler *messages.MessageHandler) (removed *client) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if c, ok := r.byConn[msgHandler]; ok {
		delete(r.byConn, msgHandler)
		delete(r.byName, c.username)
		return c
	}
	return nil
}

func (r *registry) broadcast(w *messages.Wrapper) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	for _, c := range r.byConn {
		go func(cc *client) {
			_ = cc.msgHandler.Send(w)
		}(c)
	}
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
