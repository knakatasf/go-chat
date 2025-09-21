package messages

import (
	"encoding/binary"
	"io"
	"net"
	"sync"

	"google.golang.org/protobuf/proto"
)

type MessageHandler struct {
	conn      net.Conn
	sendMutex sync.Mutex
}

func (m *MessageHandler) Handle(conn net.Conn) {}

func NewMessageHandler(conn net.Conn) *MessageHandler {
	m := &MessageHandler{
		conn: conn,
	}

	return m
}

func (m *MessageHandler) readN(buf []byte) error {
	bytesRead := uint64(0)
	for bytesRead < uint64(len(buf)) {
		n, err := m.conn.Read(buf[bytesRead:])
		if err != nil {
			return err
		}
		bytesRead += uint64(n)
	}
	return nil
}

func (m *MessageHandler) writeN(buf []byte) error {
	bytesWritten := uint64(0)
	for bytesWritten < uint64(len(buf)) {
		n, err := m.conn.Write(buf[bytesWritten:])
		if err != nil {
			return err
		}
		bytesWritten += uint64(n)
	}
	return nil
}

func (m *MessageHandler) Send(wrapper *Wrapper) error {
	serialized, err := proto.Marshal(wrapper)
	if err != nil {
		return err
	}

	m.sendMutex.Lock()
	defer m.sendMutex.Unlock()

	prefix := make([]byte, 8)
	binary.LittleEndian.PutUint64(prefix, uint64(len(serialized)))
	if err := m.writeN(prefix); err != nil { // <- check error
		return err
	}
	if err := m.writeN(serialized); err != nil { // <- check error
		return err
	}

	return nil
}

func (m *MessageHandler) Receive() (*Wrapper, error) {
	prefix := make([]byte, 8)
	if err := m.readN(prefix); err != nil { // Propagate read errors (incl. EOF)
		return nil, err
	}

	payloadSize := binary.LittleEndian.Uint64(prefix)
	if payloadSize == 0 { // Defensive: treat 0-size as closed/invalid
		return nil, io.EOF
	}

	payload := make([]byte, payloadSize)
	if err := m.readN(payload); err != nil { // Propagate read errors
		return nil, err
	}

	wrapper := &Wrapper{}
	if err := proto.Unmarshal(payload, wrapper); err != nil {
		return nil, err
	}
	if wrapper.GetMsg() == nil {
		return nil, io.EOF
	}
	return wrapper, nil
}

func (m *MessageHandler) SendBytes(frame []byte) error {
	m.sendMutex.Lock()
	defer m.sendMutex.Unlock()
	return m.writeN(frame)
}

func MarshalFrame(w *Wrapper) ([]byte, error) {
	payload, err := proto.Marshal(w)
	if err != nil {
		return nil, err
	}
	frame := make([]byte, 8+len(payload))
	binary.LittleEndian.PutUint64(frame, uint64(len(payload)))
	copy(frame[8:], payload)
	return frame, nil
}

func (m *MessageHandler) Close() {
	m.conn.Close()
}
