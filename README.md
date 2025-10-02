# Go Chat

## Building Proto File

Install Protocol Buffers first.

`cd` to the `proto` directory and run `./build.sh`.

If you don't have `protoc-gen-go`:  
    `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`


## Running the Client + Server
First, `cd` into the `chat` directory.

```
go mod init chat
go mod tidy
```

To run server:
`go run server/server.go listen-port`

To run client:
`go run client/client.go username servername:port`




`go run chat/server 9999`
