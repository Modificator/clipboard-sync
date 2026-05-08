package protocol

import "time"

const (
	TypeHello    = "hello"
	TypeHelloAck = "hello_ack"
	TypeUpdate   = "clipboard_update"
	TypeError    = "error"
)

const (
	KindText  = "text"
	KindImage = "image"
)

type Envelope struct {
	Type   string           `json:"type"`
	Hello  *Hello           `json:"hello,omitempty"`
	Ack    *HelloAck        `json:"ack,omitempty"`
	Update *ClipboardUpdate `json:"update,omitempty"`
	Error  *ErrorMessage    `json:"error,omitempty"`
}

type Hello struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name,omitempty"`
	Platform   string `json:"platform,omitempty"`
	Room       string `json:"room"`
	Token      string `json:"token"`
}

type HelloAck struct {
	ServerTime time.Time `json:"server_time"`
	Message    string    `json:"message,omitempty"`
}

type ClipboardUpdate struct {
	DeviceID   string    `json:"device_id"`
	DeviceName string    `json:"device_name,omitempty"`
	Room       string    `json:"room"`
	Kind       string    `json:"kind"`
	MIMEType   string    `json:"mime_type,omitempty"`
	Hash       string    `json:"hash"`
	Text       string    `json:"text,omitempty"`
	DataBase64 string    `json:"data_base64,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	ServerSeq  uint64    `json:"server_seq,omitempty"`
}

type ErrorMessage struct {
	Message string `json:"message"`
}
