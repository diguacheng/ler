package main

import (
	"context"
	"errors"
	"io"
	"ler/sub/msg"
	"ler/sub/pipe"
	"log"
	"net"
	"time"
)

type ClientConfig struct {
	ClientPort  string `json:"client_port,omitempty"` //用于客户端链接的端口
	ServicePort string `json:"server_port,omitempty"` //被代理服务的端口
}

type Client struct {
	*ClientConfig

	Conn net.Conn //控制信号的连接

	ReadCh  chan *msg.Msg
	WriteCh chan *msg.Msg
	CloseCh chan struct{}
}

func NewClient(config *ClientConfig) *Client {
	client := &Client{
		ClientConfig: config,
	}
	return client
}

func (c *Client) Start() {
	ctx := context.Background()

	for {
		err := c.Register(ctx)
		if err != nil {
			log.Printf("client registe failed %v\n", err)
			time.Sleep(time.Second * 3)

		} else {
			c.Run(ctx)
		}
	}
	return

}

func (c *Client) Run(ctx context.Context) {
	go c.MsgHandler(ctx)
	go c.Write(ctx)
	go c.Read(ctx)
	<-ctx.Done()
	if c.Conn != nil {
		log.Println("conn close")
		c.Conn.Close()
	}

}

func (c *Client) MsgHandler(ctx context.Context) {

	//
	hbSendCh := time.NewTicker(time.Second * 3)

	for {
		select {
		case <-hbSendCh.C:
			c.WriteCh <- &msg.Msg{
				MsgType:    msg.MsgTypePing,
				MsgContent: "",
			}
			log.Printf("client#ping ..\n")

		case rMsg, ok := <-c.ReadCh:
			if !ok {
				return
			}
			switch rMsg.MsgType {
			case msg.MsgTypePong:
				log.Printf("client#pong... \n")
			case msg.MsgTypeNewConnReq:
				go c.HandleNewConnReq(ctx)

			}
		}
	}

}

func (c *Client) HandleNewConnReq(ctx context.Context) {

	serviceConn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", c.ServicePort))
	if err != nil {
		return
	}
	workConn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", c.ClientPort))
	if err != nil {
		return

	}

	err = msg.WriteMsg(workConn, &msg.Msg{
		MsgType: msg.MsgTypeNewConn,
		//MsgContent: fmt.Sprintf("serviceConn L[%s] <->R[%s],workConn L[%s] <->R[%s]", serviceConn.LocalAddr(), serviceConn.RemoteAddr(), workConn.LocalAddr(), workConn.RemoteAddr()),
	})
	if err != nil {
		return
	}

	defer workConn.Close()
	defer serviceConn.Close()
	log.Printf("HandleNewConnReq#start serviceConn L[%s] <->R[%s],workConn L[%s] <->R[%s]\n", serviceConn.LocalAddr(), serviceConn.RemoteAddr(), workConn.LocalAddr(), workConn.RemoteAddr())
	pipe.Trans(serviceConn, workConn)
	//log.Printf("HandleNewConnReq#end serviceConn L[%s] <->R[%s],workConn L[%s] <->R[%s]\n", serviceConn.LocalAddr(), serviceConn.RemoteAddr(), workConn.LocalAddr(), workConn.RemoteAddr())

}

func (c *Client) Read(ctx context.Context) {

	defer close(c.CloseCh)

	for {
		rMsg, err := msg.RedMsg(c.Conn)
		if err != nil {
			if err == io.EOF {
				log.Println("conn EOF...")
				return
			}
			c.Conn.Close()
			return

		}
		c.ReadCh <- rMsg
	}
}

func (c *Client) Write(ctx context.Context) {

	for {
		m, ok := <-c.WriteCh
		if !ok {
			log.Println("writech closed")
			return
		}
		err := msg.WriteMsg(c.Conn, m)
		if err != nil {
			log.Printf("write msg err %s\n", err)
			return
		}
	}

}

func (c *Client) Register(ctx context.Context) error {
	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", c.ClientPort))
	if err != nil {
		return err
	}
	err = msg.WriteMsg(conn, &msg.Msg{
		MsgType:    msg.MsgTypeRegister,
		MsgContent: "",
	})
	if err != nil {
		log.Println("Register#write failed")
		return err
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second * 10))
	redMsg, err := msg.RedMsg(conn)
	if err != nil {
		return err
	}
	if redMsg.MsgType != msg.MsgTypeRegisterResp {
		log.Printf("client#Register err redMsg:%v", redMsg)
		return errors.New("client#Register resp err")
	}
	_ = conn.SetReadDeadline(time.Time{})
	c.Conn = conn
	c.ReadCh = make(chan *msg.Msg, 5)
	c.WriteCh = make(chan *msg.Msg, 5)
	c.CloseCh = make(chan struct{})
	log.Printf("Client#register success conn r[%s] l[%s] \n", conn.RemoteAddr(), conn.LocalAddr())
	return nil
}

func main() {
	C := NewClient(&ClientConfig{
		ClientPort:  "8888",
		ServicePort: "7860",
	})
	C.Start()
}
