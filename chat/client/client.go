package main

import (
	"bufio"
	"chat/messages"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatalln("usage: client <username> <host:port>")
	}
	user := os.Args[1]
	host := os.Args[2]
	fmt.Println("Hello,", user)

	conn, err := net.Dial("tcp", host)
	if err != nil {
		log.Fatalln(err)
	}
	defer conn.Close()

	msgHandler := messages.NewMessageHandler(conn)

	// Register
	reg := messages.Registration{Username: user}
	if err := msgHandler.Send(&messages.Wrapper{
		Msg: &messages.Wrapper_RegistrationMessage{RegistrationMessage: &reg},
	}); err != nil {
		log.Fatalln("registration failed:", err)
	}

	// Reader goroutine
	go func() {
		for {
			w, err := msgHandler.Receive()
			if err != nil {
				log.Println("recv:", err)
				return
			}
			switch m := w.Msg.(type) {
			case *messages.Wrapper_ServerNotice:
				if room := m.ServerNotice.GetRoom(); room != "" {
					fmt.Fprintf(os.Stderr, "\r\033[K[room:%s] * %s\n", room, m.ServerNotice.GetText())
				} else {
					fmt.Fprintf(os.Stderr, "\r\033[K* %s\n", m.ServerNotice.GetText())
				}
			case *messages.Wrapper_RoomChat:
				rc := m.RoomChat
				fmt.Fprintf(os.Stderr, "\r\033[K[room:%s] <%s> %s\n",
					rc.GetRoom(), rc.GetUsername(), rc.GetMessageBody())
			case *messages.Wrapper_DirectChat:
				dc := m.DirectChat
				fmt.Fprintf(os.Stderr, "\r\033[K[dm %s→%s] %s\n",
					dc.GetFrom(), dc.GetTo(), dc.GetMessageBody())
			}
			fmt.Fprint(os.Stderr, "message> ")
		}
	}()

	currentRoom := "" // user must /join before sending
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Fprint(os.Stderr, "message> ")
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			fmt.Fprint(os.Stderr, "message> ")
			continue
		}

		if strings.HasPrefix(line, "/") {
			fields := strings.Fields(line)
			cmd := strings.ToLower(fields[0])
			switch cmd {
			case "/join":
				if len(fields) < 2 {
					fmt.Fprintln(os.Stderr, "usage: /join <room>")
					fmt.Fprint(os.Stderr, "message> ")
					continue
				}
				room := fields[1]
				currentRoom = room
				_ = msgHandler.Send(&messages.Wrapper{
					Msg: &messages.Wrapper_RoomJoin{
						RoomJoin: &messages.RoomJoin{Username: user, Room: room},
					},
				})

			case "/leave":
				if len(fields) < 2 {
					fmt.Fprintln(os.Stderr, "usage: /leave <room>")
					fmt.Fprint(os.Stderr, "message> ")
					continue
				}
				room := fields[1]
				if currentRoom == room {
					currentRoom = ""
				}
				_ = msgHandler.Send(&messages.Wrapper{
					Msg: &messages.Wrapper_RoomLeave{
						RoomLeave: &messages.RoomLeave{Username: user, Room: room},
					},
				})

			case "/dm":
				if len(fields) < 3 {
					fmt.Fprintln(os.Stderr, "usage: /dm <user> <message>")
					fmt.Fprint(os.Stderr, "message> ")
					continue
				}
				to := fields[1]
				body := strings.TrimSpace(line[len(cmd)+1+len(to)+1:])
				_ = msgHandler.Send(&messages.Wrapper{
					Msg: &messages.Wrapper_DirectChat{
						DirectChat: &messages.DirectChat{
							From: user, To: to, MessageBody: body,
						},
					},
				})

			default:
				fmt.Fprintln(os.Stderr, "commands: /join /leave /dm /room")
			}
		} else {
			// plain message -> current room
			if currentRoom == "" {
				fmt.Fprintln(os.Stderr, "Join a room first: /join <room>")
				fmt.Fprint(os.Stderr, "message> ")
				continue
			}
			_ = msgHandler.Send(&messages.Wrapper{
				Msg: &messages.Wrapper_RoomChat{
					RoomChat: &messages.RoomChat{
						Username: user, Room: currentRoom, MessageBody: line,
					},
				},
			})
		}

		fmt.Fprint(os.Stderr, "\r\033[K")
		fmt.Fprint(os.Stderr, "message> ")
	}
	if err := scanner.Err(); err != nil {
		log.Println("stdin:", err)
	}
}
