package main

import (
	"chat/messages"
	"fmt"
	"log"
	"net"
	"os"
)

var users = newRegistry()

func notice(text string) *messages.Wrapper {
	return &messages.Wrapper{
		Msg: &messages.Wrapper_ServerNotice{
			ServerNotice: &messages.ServerNotice{Text: text},
		},
	}
}

func roomNotice(room, text string) *messages.Wrapper {
	return &messages.Wrapper{
		Msg: &messages.Wrapper_ServerNotice{
			ServerNotice: &messages.ServerNotice{Text: text, Room: room},
		},
	}
}

func handleClient(msgHandler *messages.MessageHandler) {
	defer func() {
		if c := users.remove(msgHandler); c != nil && c.username != "" {
			// announce to all rooms they were in is optional; here we skip, since membership purged
		}
		msgHandler.Close()
	}()

	first, err := msgHandler.Receive()
	if err != nil {
		log.Println("registration read error:", err)
		return
	}
	reg := first.GetRegistrationMessage()
	if reg == nil || reg.GetUsername() == "" {
		_ = msgHandler.Send(notice("You must register with a non-empty username"))
		return
	}
	username := reg.GetUsername()

	// Create running client up-front and add
	c := newClient(msgHandler, username)
	if err := users.add(c); err != nil {
		_ = msgHandler.Send(notice("Registration failed: " + err.Error()))
		return
	}

	// Main loop (room-only + DM)
	for {
		wrapper, err := msgHandler.Receive()
		if err != nil {
			log.Println("receive error:", err)
			return
		}

		switch msg := wrapper.Msg.(type) {
		case *messages.Wrapper_ServerNotice:
			// ignore client-crafted notices

		case *messages.Wrapper_RoomJoin:
			room := msg.RoomJoin.GetRoom()
			if err := users.joinRoom(c, room); err != nil {
				_ = msgHandler.Send(notice("Join failed: " + err.Error()))
				continue
			}
			users.broadcastRoom(room, roomNotice(room, fmt.Sprintf("%s joined", username)))

		case *messages.Wrapper_RoomLeave:
			room := msg.RoomLeave.GetRoom()
			if err := users.leaveRoom(c, room); err != nil {
				_ = msgHandler.Send(notice("Leave failed: " + err.Error()))
				continue
			}
			users.broadcastRoom(room, roomNotice(room, fmt.Sprintf("%s left", username)))

		case *messages.Wrapper_RoomChat:
			rc := msg.RoomChat
			room := rc.GetRoom()
			// overwrite sender
			rc.Username = username

			// optional membership guard: if not in room, bounce
			if set, ok := users.rooms[room]; !ok || set[c] == (struct{}{}) == false {
				_ = msgHandler.Send(&messages.Wrapper{
					Msg: &messages.Wrapper_ServerNotice{ServerNotice: &messages.ServerNotice{
						Text: fmt.Sprintf("Join the room first: /join %s", room),
						Room: room,
					}},
				})
				continue
			}

			users.broadcastRoom(room, wrapper)

		case *messages.Wrapper_DirectChat:
			dc := msg.DirectChat
			// overwrite sender
			dc.From = username
			users.direct(dc.GetTo(), wrapper)

		case *messages.Wrapper_RegistrationMessage:
			_ = msgHandler.Send(notice("Already registered as " + username))

		default:
			log.Printf("unexpected message type: %T", msg)
		}
	}
}

func main() {
	if len(os.Args) < 2 {
		log.Fatalln("usage: server <port>")
	}
	addr := ":" + os.Args[1]
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("listening on", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("accept error:", err)
			continue
		}
		msgHandler := messages.NewMessageHandler(conn)
		go handleClient(msgHandler)
	}
}
