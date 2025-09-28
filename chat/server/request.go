package main

import (
	"chat/messages"
)

type addRequest struct {
	c        *client
	response chan error
}

type removeRequest struct {
	msgHandler *messages.MessageHandler
	response   chan *client
}

type roomJoinRequest struct {
	c      *client
	room   string
	result chan error
}

type roomLeaveRequest struct {
	c      *client
	room   string
	result chan error
}

type roomBroadcastRequest struct {
	room string
	w    *messages.Wrapper
}

type directRequest struct {
	to string
	w  *messages.Wrapper
}
