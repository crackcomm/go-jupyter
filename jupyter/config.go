package jupyter

import (
	"encoding/json"
	"fmt"
	"os"
)

type SignKey []byte

// ConnectionInfo - Jupyter kernel connection info.
type ConnectionInfo struct {
	SignatureScheme string `json:"signature_scheme"`
	Transport       string `json:"transport"`
	IP              string `json:"ip"`
	Key             string `json:"key"`
	StdinPort       int    `json:"stdin_port"`
	ControlPort     int    `json:"control_port"`
	IOPubPort       int    `json:"iopub_port"`
	HeartBeatPort   int    `json:"hb_port"`
	ShellPort       int    `json:"shell_port"`
}

func (info ConnectionInfo) ShellAddr() string {
	if info.Transport == "ipc" {
		return fmt.Sprintf("ipc://%s-%d", info.IP, info.ShellPort)
	}
	return fmt.Sprintf("%s://%s:%d", info.Transport, info.IP, info.ShellPort)
}

func (info ConnectionInfo) IOPubAddr() string {
	if info.Transport == "ipc" {
		return fmt.Sprintf("ipc://%s-%d", info.IP, info.IOPubPort)
	}
	return fmt.Sprintf("%s://%s:%d", info.Transport, info.IP, info.IOPubPort)
}

func (info ConnectionInfo) SignKey() SignKey {
	return SignKey(info.Key)
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
