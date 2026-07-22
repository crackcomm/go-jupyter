package jupyter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-zeromq/zmq4"
	"github.com/sony/sonyflake/v2"
)

// Client - Jupyter kernel client.
type Client struct {
	shell   zmq4.Socket
	iopub   zmq4.Socket
	signKey SignKey
	session string

	// Lock used to add and delete channels.
	ioChanLock *sync.RWMutex
	ioChannels map[string]chan<- any
}

var idgen, _ = sonyflake.New(sonyflake.Settings{})

func NewClient(ctx context.Context, info ConnectionInfo) (*Client, error) {
	var (
		shell zmq4.Socket
		iopub zmq4.Socket
	)
	closeAll := func() {
		if shell != nil {
			_ = shell.Close()
		}
		if iopub != nil {
			_ = iopub.Close()
		}
	}
	shell = zmq4.NewReq(ctx)
	if err := shell.Dial(info.ShellAddr()); err != nil {
		return nil, fmt.Errorf("shell dial: %v", err)
	}
	iopub = zmq4.NewSub(ctx)
	if err := iopub.Dial(info.IOPubAddr()); err != nil {
		closeAll()
		return nil, fmt.Errorf("iopub dial: %v", err)
	}
	if err := iopub.SetOption(zmq4.OptionSubscribe, ""); err != nil {
		closeAll()
		return nil, err
	}
	session, _ := idgen.NextID()
	client := Client{
		shell:      shell,
		iopub:      iopub,
		signKey:    info.SignKey(),
		session:    fmt.Sprintf("%d", session),
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
	msgID, _ := idgen.NextID()
	return Header{
		Version:  Version,
		Date:     time.Now().UTC().Format(time.RFC3339),
		MsgID:    fmt.Sprintf("%d", msgID),
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

	if err := client.shell.SendMulti(zmq4.Msg{Frames: parts}); err != nil {
		return fmt.Errorf("send: %v", err)
	}
	return nil
}

func (client *Client) recvReply(content any) (err error) {
	body, err := client.shell.Recv()
	if err != nil {
		return
	}
	var raw RawMessage
	if err := raw.Decode(body.Frames, client.signKey); err != nil {
		return err
	}
	return json.Unmarshal(raw.Content, content)
}

func (client *Client) pollIO() error {
	for {
		body, err := client.iopub.Recv()
		if err != nil {
			return err
		}

		var raw RawMessage
		if err := raw.Decode(body.Frames, client.signKey); err != nil {
			return fmt.Errorf("iopub decode: %v", err)
		}
		// log.Printf("parent: %s type: %s content: %s", raw.ParentHeader.MsgID, raw.Header.MsgType, raw.Content)

		if raw.ParentHeader.MsgType != RequestExecute {
			continue
		}

		msg, err := unmarshalIOPubMessage(raw.Header.MsgType, raw.Content)
		if err != nil {
			return fmt.Errorf("iopub unmarshal: %v (%s)", err, raw.Header.MsgType)
		}

		if ch, ok := client.getIOChannel(raw.ParentHeader.MsgID); ok {
			ch <- msg
		} else {
			return fmt.Errorf("already closed %s", raw.ParentHeader.MsgID)
		}

		// close the channel if status is idle
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
	err := errors.Join(
		client.shell.Close(),
		client.iopub.Close(),
	)

	client.ioChanLock.Lock()
	defer client.ioChanLock.Unlock()

	for _, ch := range client.ioChannels {
		close(ch)
	}

	return err
}
