package main

import (
	"chat/messages"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func joinRoom(user string, currentRoom string, room string, msgHandler *messages.MessageHandler) string {
	if currentRoom != "" { // If you're in a room, you need to leave the room first
		leaveRoom(user, currentRoom, msgHandler)
	}
	_ = msgHandler.Send(&messages.Wrapper{
		Msg: &messages.Wrapper_RoomJoin{
			RoomJoin: &messages.RoomJoin{Username: user, Room: room},
		},
	})
	return room
}

func leaveRoom(user string, currentRoom string, msgHandler *messages.MessageHandler) string {
	if currentRoom == "" {
		guiPrint("[warn] You haven't joined a room")
		return currentRoom
	}
	_ = msgHandler.Send(&messages.Wrapper{
		Msg: &messages.Wrapper_RoomLeave{
			RoomLeave: &messages.RoomLeave{Username: user, Room: currentRoom},
		},
	})
	return ""
}

func directMessage(from string, to string, body string, msgHandler *messages.MessageHandler) {
	_ = msgHandler.Send(&messages.Wrapper{
		Msg: &messages.Wrapper_DirectChat{
			DirectChat: &messages.DirectChat{
				From: from, To: to, MessageBody: body,
			},
		},
	})
}

var (
	guiApp    fyne.App
	guiWindow fyne.Window
	readPane  *fyne.Container
	scroll    *container.Scroll

	guiPrint = func(line string) {
		if guiApp == nil || readPane == nil || scroll == nil {
			fmt.Fprintln(os.Stderr, line)
			return
		}
		const maxMsgs = 2000
		if len(readPane.Objects) > maxMsgs {
			// drop the oldest
			readPane.Objects = readPane.Objects[len(readPane.Objects)-maxMsgs:]
			readPane.Refresh()
		}
		fyne.Do(func() {
			lbl := widget.NewLabel(line)
			lbl.Wrapping = fyne.TextWrapWord
			readPane.Add(lbl)
			scroll.Refresh()
			scroll.ScrollToBottom()
		})
	}
)

func receiveMessage(msgHandler *messages.MessageHandler) {
	for {
		w, err := msgHandler.Receive()
		if err != nil {
			guiPrint(fmt.Sprintf("recv: %v", err))
			return
		}
		switch m := w.Msg.(type) {
		case *messages.Wrapper_ServerNotice:
			if room := m.ServerNotice.GetRoom(); room != "" {
				guiPrint(fmt.Sprintf("[room:%s] * %s", room, m.ServerNotice.GetText()))
			} else {
				guiPrint("* " + m.ServerNotice.GetText())
			}
		case *messages.Wrapper_RoomChat:
			rc := m.RoomChat
			guiPrint(fmt.Sprintf("[room:%s] <%s> %s",
				rc.GetRoom(), rc.GetUsername(), rc.GetMessageBody()))
		case *messages.Wrapper_DirectChat:
			dc := m.DirectChat
			guiPrint(fmt.Sprintf("[dm %s→%s] %s",
				dc.GetFrom(), dc.GetTo(), dc.GetMessageBody()))
		}
	}
}

func main() {
	if len(os.Args) < 3 {
		log.Fatalln("usage: client <username> <host:port>")
	}
	user := os.Args[1]
	host := os.Args[2]
	fmt.Println("Hello,", user)

	// Connect
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

	// start gui window
	guiApp = app.New()
	guiWindow = guiApp.NewWindow(fmt.Sprintf("%s — not joined", user))
	guiWindow.Resize(fyne.NewSize(820, 600))

	// Reading section
	readPane = container.NewVBox()
	scroll = container.NewVScroll(readPane)
	scroll.SetMinSize(fyne.NewSize(800, 460))

	// Writing section
	input := widget.NewEntry()
	input.SetPlaceHolder(`type message or command (e.g., "/join bluesky", "/leave", "/dm bob hello")`)
	sendBtn := widget.NewButton("Send", func() {})

	var currentRoom atomic.Value
	currentRoom.Store("")

	// command handler
	handleLine := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}

		if strings.HasPrefix(line, "/") {
			fields := strings.Fields(line)
			cmd := strings.ToLower(fields[0])
			switch cmd {
			case "/join":
				if len(fields) < 2 {
					guiPrint("usage: /join <room>")
					return
				}
				room := fields[1]
				newRoom := joinRoom(user, currentRoom.Load().(string), room, msgHandler)
				currentRoom.Store(newRoom)
				// Window mutations must be on UI thread
				fyne.Do(func() {
					guiWindow.SetTitle(fmt.Sprintf("%s — %s", user, newRoom))
				})

			case "/leave":
				newRoom := leaveRoom(user, currentRoom.Load().(string), msgHandler)
				currentRoom.Store(newRoom)
				fyne.Do(func() {
					if newRoom == "" {
						guiWindow.SetTitle(fmt.Sprintf("%s — not joined", user))
					} else {
						guiWindow.SetTitle(fmt.Sprintf("%s — %s", user, newRoom))
					}
				})

			case "/dm":
				if len(fields) < 3 {
					guiPrint("usage: /dm <user> <message>")
					return
				}
				to := fields[1]
				body := strings.TrimSpace(line[len(cmd)+1+len(to)+1:])
				directMessage(user, to, body, msgHandler)

			default:
				guiPrint("commands: /join /leave /dm")
			}
		} else {
			// plain message -> current room
			room := currentRoom.Load().(string)
			if room == "" {
				guiPrint("Join a room first: /join <room>")
				return
			}
			_ = msgHandler.Send(&messages.Wrapper{
				Msg: &messages.Wrapper_RoomChat{
					RoomChat: &messages.RoomChat{
						Username: user, Room: room, MessageBody: line,
					},
				},
			})
		}
	}

	// wire input submit + button
	input.OnSubmitted = func(text string) {
		handleLine(text)
		input.SetText("")
	}
	sendBtn.OnTapped = func() {
		txt := input.Text
		handleLine(txt)
		input.SetText("")
	}

	// Layout
	bottomBar := container.NewBorder(nil, nil, nil, sendBtn, input)
	guiWindow.SetContent(container.NewBorder(nil, bottomBar, nil, nil, scroll))

	// Receiver goroutine (prints via guiPrint; guiPrint uses fyne.Do)
	go receiveMessage(msgHandler)

	guiWindow.ShowAndRun()
}
