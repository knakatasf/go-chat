package main

import (
	"bufio"
	"chat/messages"
	"fmt"
	"log"
	"net"
	"os"
)

func main() {
	user := os.Args[1]
	fmt.Println("Hello, " + user)

	host := os.Args[2]
	conn, err := net.Dial("tcp", host)
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	defer conn.Close()

	msgHandler := messages.NewMessageHandler(conn)

	reg := messages.Registration{Username: user}
	if err := msgHandler.Send(&messages.Wrapper{
		Msg: &messages.Wrapper_RegistrationMessage{RegistrationMessage: &reg},
	}); err != nil {
		log.Fatalln("Registration failed: ", err.Error())
	}

	go func() {
		for {
			w, err := msgHandler.Receive()
			if err != nil {
				log.Println("Error receiving message: ", err)
				return
			}
			if chat := w.GetChatMessage(); chat != nil {
				fmt.Fprint(os.Stderr, "\r\033[K")
				fmt.Printf("<%s> %s\n", chat.GetUsername(), chat.GetMessageBody())
				fmt.Fprint(os.Stderr, "message> ")
			}
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Fprint(os.Stderr, "message> ")
	for {
		result := scanner.Scan() // Reads up to a \n newline character
		if result == false {
			break
		}

		message := scanner.Text()
		if len(message) != 0 {
			msg := messages.Chat{Username: user, MessageBody: message}
			wrapper := &messages.Wrapper{
				Msg: &messages.Wrapper_ChatMessage{ChatMessage: &msg},
			}
			if err := msgHandler.Send(wrapper); err != nil {
				log.Println("Error sending message: ", err)
				break
			}
			fmt.Fprint(os.Stderr, "\r\033[K")
			fmt.Fprint(os.Stderr, "message> ")
		}
	}
}
