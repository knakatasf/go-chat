// go get github.com/gorilla/websocket
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"

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

func main() {
	// Build Jetstream URL with a filter: only posts
	qs := url.Values{} // map[string][]string
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

	// Connect
	c, _, err := websocket.DefaultDialer.Dial(u.String(), http.Header{})
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	fmt.Println("Connected to Jetstream! Waiting for posts...")

	// Loop and read messages
	for {
		_, data, err := c.ReadMessage() // c = connection
		if err != nil {
			log.Fatal("read:", err)
		}
		// data is raw message the connection caught
		var ev JetEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			continue
		}
		if ev.Kind != "commit" {
			continue // For now, I only want "commit"
		}

		// Try to decode as a post
		var post PostRecord
		if err := json.Unmarshal(ev.Commit.Record, &post); err == nil {
			fmt.Printf("Post from %s: %s\n", ev.Commit.Repo, post.Text)
		}
	}
}
