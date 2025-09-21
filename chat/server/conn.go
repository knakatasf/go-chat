package main

import "chat/messages"

type client struct {
	msgHandler *messages.MessageHandler
	username   string
	out        chan *messages.Wrapper
	closed     chan struct{} // Unbuffered channel
}

func newClient(msgHandler *messages.MessageHandler, username string) *client {
	c := &client{
		msgHandler: msgHandler,
		username:   username,
		// Can hold up to 128 messages in this channel
		// Bounded queue -> But, we need a default clause otherwise goroutine blocks!!!
		out:    make(chan *messages.Wrapper, 128),
		closed: make(chan struct{}),
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

func (c *client) enqueue(w *messages.Wrapper) {
	select {
	case c.out <- w:
	default:
		// If c.out reaches 128 messages, the select default kicks in -> new messages are dropped
		// We don't want to pile any more messages in the out channel
		// If no default, this will block if c.out is full!!!
	}
}
