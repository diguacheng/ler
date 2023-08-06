package msg

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
)

const (
	MsgTypeRegister int8 = iota //client注册
	MsgTypeRegisterResp
	MsgTypePing //探活
	MsgTypePong
	MsgTypeNewConn    // 要求客户端发起请链接
	MsgTypeNewConnReq //客户端发起的新链接

)

type Msg struct {
	MsgType    int8   `json:"msg_type,omitempty"`
	MsgContent string `json:"msg_content,omitempty"`
}

// 2+n两个字节是长度

func RedMsg(conn net.Conn) (*Msg, error) {
	var length int64
	err := binary.Read(conn, binary.BigEndian, &length)
	if err != nil {
		return nil, err
	}
	if length < 0 {
		return nil, errors.New("length less zero")
	}
	data := make([]byte, length)
	n, err := io.ReadFull(conn, data)
	if err != nil {
		return nil, err
	}

	if int64(n) != length {
		err = errors.New("length error not equal")
	}
	var msg Msg
	err = json.Unmarshal(data, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func WriteMsg(conn net.Conn, msg *Msg) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	length := int64(len(data))
	buffer := bytes.NewBuffer(nil)
	err = binary.Write(buffer, binary.BigEndian, length)
	if err != nil {
		return err
	}
	buffer.Write(data)
	_, err = conn.Write(buffer.Bytes())
	return err
}
