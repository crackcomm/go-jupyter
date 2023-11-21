package jupyter

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/go-zeromq/zmq4"
	"github.com/google/uuid"
)

// ConnectionInfo - Jupyter kernel connection info.
type ConnectionInfo struct {
	SignatureScheme string `json:"signature_scheme"`
	Transport       string `json:"transport"`
	IP              string `json:"ip"`
	Key             string `json:"key"`
	StdinPort       int    `json:"stdin_port"`
	ControlPort     int    `json:"control_port"`
	IoPubPort       int    `json:"iopub_port"`
	HeartBeatPort   int    `json:"hb_port"`
	ShellPort       int    `json:"shell_port"`
}

func (info *ConnectionInfo) ShellAddr() string {
	return fmt.Sprintf("%s://%s:%d", info.Transport, info.IP, info.ShellPort)
}

func (info *ConnectionInfo) IoPubAddr() string {
	return fmt.Sprintf("%s://%s:%d", info.Transport, info.IP, info.IoPubPort)
}

func ReadConfigFile(path string) (info ConnectionInfo, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if err = json.Unmarshal(data, &info); err != nil {
		return
	}
	return
}

// Client - Jupyter kernel client.
type Client struct {
	shell   zmq4.Socket
	iopub   zmq4.Socket
	signKey []byte
	session uuid.UUID

	// Lock used to add and delete channels.
	ioChanLock *sync.RWMutex
	ioChannels map[string]chan<- interface{}
}

func NewClient(ctx context.Context, info *ConnectionInfo) (_ *Client, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if err != nil {
			cancel()
		}
	}()
	shell := zmq4.NewReq(ctx)
	if err = shell.Dial(info.ShellAddr()); err != nil {
		err = fmt.Errorf("Shell connection error: %v", err)
		return
	}
	iopub := zmq4.NewSub(ctx)
	if err = iopub.Dial(info.IoPubAddr()); err != nil {
		err = fmt.Errorf("IoPub connection error: %v", err)
		return
	}
	if err = iopub.SetOption(zmq4.OptionSubscribe, ""); err != nil {
		return
	}
	client := Client{
		shell:      shell,
		iopub:      iopub,
		signKey:    []byte(info.Key),
		session:    uuid.New(),
		ioChanLock: new(sync.RWMutex),
		ioChannels: make(map[string]chan<- interface{}),
	}
	go func() {
		if err := client.pollIO(); err != nil {
			cancel()
		}
	}()
	return &client, nil
}

func (client *Client) createHeader(msgType string) Header {
	return Header{
		Version:  Version,
		Date:     time.Now().UTC().Format(time.RFC3339),
		MsgID:    uuid.New().String(),
		MsgType:  msgType,
		Username: "go-jupyter",
		Session:  client.session.String(),
	}
}

func (client *Client) createMessage(msgType string, req interface{}) Message {
	return Message{
		Header:   client.createHeader(msgType),
		Metadata: make(map[string]interface{}),
		Content:  req,
	}
}

func (client *Client) Execute(req *ExecutionRequest) (rep ExecutionResult, ch <-chan interface{}, err error) {
	msg := client.createMessage(RequestExecute, req)
	ch = client.addIOChannel(msg.Header.MsgID)
	err = client.request(msg, &rep)
	return
}

func (client *Client) addIOChannel(id string) <-chan interface{} {
	client.ioChanLock.Lock()
	defer client.ioChanLock.Unlock()
	ch := make(chan interface{})
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

func (client *Client) request(req Message, rep interface{}) (err error) {
	if err = client.sendRequest(req); err != nil {
		return
	}
	err = client.recvReply(&rep)
	return
}

func (client *Client) sendRequest(msg Message) error {
	frames := [][]byte{[]byte("<IDS|MSG>")}
	encoded, err := msg.Encode(client.signKey)
	if err != nil {
		return fmt.Errorf("Error encoding message: %v", err)
	}
	frames = append(frames, encoded...)

	if err := client.shell.SendMulti(zmq4.NewMsgFrom(frames...)); err != nil {
		return fmt.Errorf("Error sending shell message: %v", err)
	}
	return nil
}

func (client *Client) recvReply(content interface{}) (err error) {
	reply := Message{Content: content}
	body, err := client.shell.Recv()
	if err != nil {
		return
	}
	return reply.Decode(body.Frames, client.signKey)
}

func (client *Client) pollIO() (err error) {
	for {
		body, err := client.iopub.Recv()
		if err != nil {
			break
		}
		var msg RawMessage
		if err = msg.Decode(body.Frames, client.signKey); err != nil {
			return fmt.Errorf("Error decoding a message: %#v", err)
		}
		content, err := parseContent(msg.Header.MsgType, msg.Content)
		if err != nil {
			return fmt.Errorf("Error decoding a content: %#v (MsgType: %s)", err, msg.Header.MsgType)
		}
		if ch, ok := client.getIOChannel(msg.ParentHeader.MsgID); ok {
			ch <- content
		} else if msgType := msg.ParentHeader.MsgType; maybeShouldListen(msgType) {
			return fmt.Errorf("Message dropped on empty channel: %s", msgType)
		}

		// close the channel if status is idle
		if status, ok := content.(*StatusMessage); ok && status.ExecutionState == StateIdle {
			client.deleteIOChannel(msg.ParentHeader.MsgID)
		}
	}
	return
}

func maybeShouldListen(msgType string) bool {
	switch msgType {
	case RequestExecute:
		return true
	default:
		return false
	}
}

func (client *Client) getIOChannel(id string) (ch chan<- interface{}, ok bool) {
	client.ioChanLock.RLock()
	defer client.ioChanLock.RUnlock()
	ch, ok = client.ioChannels[id]
	return
}

func (client *Client) deleteIOChannel(id string) {
	client.ioChanLock.Lock()
	defer client.ioChanLock.Unlock()
	if ch, ok := client.ioChannels[id]; ok {
		close(ch)
	}
	delete(client.ioChannels, id)
}

func (client *Client) Close() error {
	defer func() {
		client.ioChanLock.Lock()
		defer client.ioChanLock.Unlock()

		if n := len(client.ioChannels); n != 0 {
			log.Printf("Closing %d IO channels", n)
		}
		for _, ch := range client.ioChannels {
			close(ch)
		}
	}()

	err1 := client.shell.Close()
	err2 := client.iopub.Close()
	if err1 != nil {
		return err1
	}
	return err2
}
