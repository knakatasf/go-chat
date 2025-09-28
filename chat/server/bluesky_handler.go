package main

import (
	"chat/messages"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Simple Jetstream event structure
type JetEvent struct {
	Kind   string `json:"kind"` // usually "commit"
	Commit struct {
		Repo   string          `json:"repo"`   // DID (user id)
		Record json.RawMessage `json:"record"` // JSON of post, like, etc.
	} `json:"commit"`
}

//{
//	"kind": "commit",
//	"commit": {
//		"repo": "did:plc:abcd1234",
//		"record": {
//			"text": "Hello world from Bluesky!",
//			"createdAt": "2025-09-27T10:00:00Z"
//		}
//	}
//}

// Bluesky post record (app.bsky.feed.post)
type PostRecord struct {
	Text string `json:"text"`
}

func bskyRoomChat(room, from, text string) *messages.Wrapper {
	return &messages.Wrapper{
		Msg: &messages.Wrapper_RoomChat{
			RoomChat: &messages.RoomChat{
				Username: from, Room: room, MessageBody: text,
			},
		},
	}
}

func startJetstreamConsumer(ctx context.Context, room string) {
	// Build Jetstream URL with a filter: only posts
	qs := url.Values{}
	qs.Set("wantedCollections", "app.bsky.feed.post")
	// wantedCollections=app.bsky.feed.post
	// Jetstream supports filtering by collection (AT Protocol "namespace" of the record)
	// app.bsky.feed.post = normal bluesky posts
	// I only want "post", without this filtering, I would get everything (likes, follows)

	u := url.URL{
		Scheme:   "wss",                             // WebSocket Secure (like https)
		Host:     "jetstream1.us-east.bsky.network", // one of the public jetstream servers
		Path:     "/subscribe",                      // path jetstream listens to
		RawQuery: qs.Encode(),
	}
	// So final url looks: wss://jetstream1.us-east.bsky.network/subscribe?wantedCollections=app.bsky.feed.post

	backoff := time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		log.Println("[bsky] dialing", u.String())
		// Connect
		c, _, err := websocket.DefaultDialer.Dial(u.String(), http.Header{})
		if err != nil {
			log.Println("[bsky] dial error:", err)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		log.Println("[bsky] connected")
		backoff = time.Second

		// read loop
		for {
			_, data, err := c.ReadMessage()
			if err != nil {
				log.Println("[bsky] read error:", err)
				_ = c.Close()
				break // reconnect
			}

			var ev JetEvent
			if json.Unmarshal(data, &ev) != nil || ev.Kind != "commit" {
				continue
			}

			// Try to parse as a post
			var pr PostRecord
			if json.Unmarshal(ev.Commit.Record, &pr) == nil {
				txt := strings.TrimSpace(pr.Text)
				if txt == "" {
					continue
				}

				from := "bsky:" + ev.Commit.Repo // later you can resolve DID->handle
				users.broadcastRoom(room, bskyRoomChat(room, from, txt))
			}
		}

		// loop to reconnect
		time.Sleep(backoff)
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}
