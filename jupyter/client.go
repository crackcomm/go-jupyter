package jupyter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/crackcomm/go-jupyter/jupyter/zmtp"
)

// Client - Jupyter kernel client implemented using pure Go ZMTP.
type Client struct {
	shellConn net.Conn
	iopubConn net.Conn
	signKey   SignKey
	session   string

	// Lock used to add and delete channels.
	ioChanLock *sync.RWMutex
	ioChannels map[string]chan<- any
}

func NewClient(ctx context.Context, info ConnectionInfo) (*Client, error) {
	var (
		shellConn net.Conn
		iopubConn net.Conn
	)
	closeAll := func() {
		if shellConn != nil {
			_ = shellConn.Close()
		}
		if iopubConn != nil {
			_ = iopubConn.Close()
		}
	}

	// 1. Dial & Handshake Shell socket (DEALER pattern)
	var err error
	shellConn, err = dialAddr(ctx, info.ShellAddr())
	if err != nil {
		return nil, fmt.Errorf("shell dial: %w", err)
	}
	if err := zmtp.PerformHandshake(shellConn, "DEALER"); err != nil {
		closeAll()
		return nil, fmt.Errorf("shell zmtp handshake: %w", err)
	}

	// 2. Dial & Handshake IOPub socket (SUB pattern)
	iopubConn, err = dialAddr(ctx, info.IOPubAddr())
	if err != nil {
		closeAll()
		return nil, fmt.Errorf("iopub dial: %w", err)
	}
	if err := zmtp.PerformHandshake(iopubConn, "SUB"); err != nil {
		closeAll()
		return nil, fmt.Errorf("iopub zmtp handshake: %w", err)
	}

	// 3. Send ZMTP subscription frame (subscribe to all topics: byte 0x01)
	if err := zmtp.SendMultipart(iopubConn, [][]byte{[]byte("\x01")}); err != nil {
		closeAll()
		return nil, fmt.Errorf("iopub subscribe: %w", err)
	}

	client := Client{
		shellConn:  shellConn,
		iopubConn:  iopubConn,
		signKey:    info.SignKey(),
		session:    strconv.FormatInt(time.Now().UnixNano(), 10),
		ioChanLock: new(sync.RWMutex),
		ioChannels: make(map[string]chan<- any),
	}

	go func() {
		if err := client.pollIO(); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("iopub error: %v", err)
			_ = client.Close()
		}
	}()

	return &client, nil
}

func (client *Client) createHeader(msgType string) Header {
	now := time.Now().UTC()
	return Header{
		Version:  Version,
		Date:     now.Format(time.RFC3339),
		MsgID:    strconv.FormatInt(now.UnixNano(), 10),
		MsgType:  msgType,
		Username: "go-jupyter",
		Session:  client.session,
	}
}

func (client *Client) createMessage(msgType string, req any) Message {
	return Message{
		Header:   client.createHeader(msgType),
		Metadata: make(map[string]any),
		Content:  req,
	}
}

func (client *Client) Execute(req *ExecutionRequest) (rep ExecutionResult, ch <-chan any, err error) {
	msg := client.createMessage(RequestExecute, req)
	ch = client.addIOChannel(msg.Header.MsgID)
	err = client.request(msg, &rep)
	return
}

func (client *Client) addIOChannel(id string) <-chan any {
	client.ioChanLock.Lock()
	defer client.ioChanLock.Unlock()
	ch := make(chan any)
	client.ioChannels[id] = ch
	return ch
}

func (client *Client) Inspect(req *IntrospectionRequest) (rep InspectReply, err error) {
	msg := client.createMessage(RequestInspect, req)
	err = client.request(msg, &rep)
	return
}

func (client *Client) History(req *HistoryRequest) (rep HistoryReply, err error) {
	msg := client.createMessage(RequestHistory, req)
	err = client.request(msg, &rep)
	return
}

func (client *Client) Shutdown() (rep map[string]any, err error) {
	rep = make(map[string]any)
	msg := client.createMessage(RequestShutdown, ShutdownRequest{Restart: false})
	err = client.request(msg, &rep)
	return
}

func (client *Client) request(req Message, rep any) error {
	if err := client.sendRequest(req); err != nil {
		return err
	}
	return client.recvReply(&rep)
}

func (client *Client) sendRequest(msg Message) error {
	parts := make([][]byte, 7)
	parts[0] = []byte("<IDS|MSG>")

	if err := msg.EncodeTo(client.signKey, parts[1:]); err != nil {
		return fmt.Errorf("encode: %v", err)
	}

	// Clean up trailing nil frames before sending over ZMTP
	for len(parts) > 0 && parts[len(parts)-1] == nil {
		parts = parts[:len(parts)-1]
	}

	if err := zmtp.SendMultipart(client.shellConn, parts); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	return nil
}

func (client *Client) recvReply(content any) (err error) {
	frames, err := zmtp.ReadMultipart(client.shellConn)
	if err != nil {
		return err
	}
	var raw RawMessage
	if err := raw.Decode(frames, client.signKey); err != nil {
		return err
	}
	return json.Unmarshal(raw.Content, content)
}

func (client *Client) pollIO() error {
	for {
		frames, err := zmtp.ReadMultipart(client.iopubConn)
		if err != nil {
			if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		var raw RawMessage
		if err := raw.Decode(frames, client.signKey); err != nil {
			return fmt.Errorf("iopub decode: %w", err)
		}

		if raw.ParentHeader.MsgType != RequestExecute {
			continue
		}

		msg, err := unmarshalIOPubMessage(raw.Header.MsgType, raw.Content)
		if err != nil {
			return fmt.Errorf("iopub unmarshal: %w (%s)", err, raw.Header.MsgType)
		}

		if ch, ok := client.getIOChannel(raw.ParentHeader.MsgID); ok {
			ch <- msg
		} else {
			return fmt.Errorf("already closed %s", raw.ParentHeader.MsgID)
		}

		// Close channel if execution state is idle
		if status, ok := msg.(*StatusMessage); ok && status.ExecutionState == StateIdle {
			client.deleteIOChannel(raw.ParentHeader.MsgID)
		}
	}
}

func (client *Client) getIOChannel(id string) (ch chan<- any, ok bool) {
	client.ioChanLock.RLock()
	defer client.ioChanLock.RUnlock()
	ch, ok = client.ioChannels[id]
	return
}

func (client *Client) deleteIOChannel(id string) {
	client.ioChanLock.Lock()
	defer client.ioChanLock.Unlock()
	ch, ok := client.ioChannels[id]
	if !ok {
		return
	}
	close(ch)
	delete(client.ioChannels, id)
}

func (client *Client) Close() error {
	var errs []error
	if client.shellConn != nil {
		errs = append(errs, client.shellConn.Close())
	}
	if client.iopubConn != nil {
		errs = append(errs, client.iopubConn.Close())
	}

	client.ioChanLock.Lock()
	defer client.ioChanLock.Unlock()

	for _, ch := range client.ioChannels {
		close(ch)
	}

	return errors.Join(errs...)
}

// dialAddr is a helper function that dials tcp or unix sockets depending on scheme.
func dialAddr(ctx context.Context, rawAddr string) (net.Conn, error) {
	network := "tcp"
	address := rawAddr

	if strings.HasPrefix(rawAddr, "tcp://") {
		network = "tcp"
		address = strings.TrimPrefix(rawAddr, "tcp://")
	} else if strings.HasPrefix(rawAddr, "ipc://") {
		network = "unix"
		address = strings.TrimPrefix(rawAddr, "ipc://")
	}

	var d net.Dialer
	return d.DialContext(ctx, network, address)
}
