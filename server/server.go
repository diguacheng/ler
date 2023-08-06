package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/diguacheng/ler/sub/msg"
	"github.com/diguacheng/ler/sub/pipe"
	"io"
	"log"
	"net"
	"time"
)

type ServerConfig struct {
	ClientPort string `json:"client_port,omitempty"` //用于监听客户端链接的端口
	ServerPort string `json:"server_port,omitempty"` //接收外部请求的接口
}

type Server struct {
	*ServerConfig
	//ClientPort string `json:"client_port,omitempty"` //用于监听客户端链接的端口
	//ServerPort string `json:"server_port,omitempty"` //接收外部请求的接口

	Ln       net.Listener // 监听内部的请求
	ServerLn net.Listener //监听外部的请求
	cli      *Cli         //客户端管理

	Cancel context.CancelFunc
}

func NewServer(cfg *ServerConfig) (*Server, error) {
	server := &Server{
		ServerConfig: cfg,
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", cfg.ClientPort))
	if err != nil {
		return nil, err
	}
	server.Ln = ln

	sln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", cfg.ServerPort))
	if err != nil {
		return nil, err
	}
	server.ServerLn = sln
	return server, nil
}

func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.Cancel = cancel
	// 监听内部请求
	go s.HandleClientListener()
	// 监听外部请求
	go s.HandleServerListener()

	<-ctx.Done()
	if s.Ln != nil {
		s.Close()
	}
	return nil

}

func (s *Server) Close() {
	if s.Ln != nil {
		s.Ln.Close()
		s.Ln = nil
	}
	if s.ServerLn != nil {
		s.ServerLn.Close()
		s.ServerLn = nil
	}
	if s.cli != nil {
		//TODO closeCli
	}

}

func (s *Server) HandleServerListener() {
	for {
		conn, err := s.ServerLn.Accept()
		if err != nil {
			return
		}
		log.Printf("Get user connection addr：L[%s] R[%s]\n", conn.LocalAddr(), conn.RemoteAddr())
		go s.HandleUserConnection(conn)
	}

}

func (s *Server) HandleUserConnection(conn net.Conn) {
	defer conn.Close()

	workConn, err := s.cli.GetWorkConn()
	if err != nil {
		log.Printf("HandleUserConnection getworkConn failed err\n", err)
		return
	}
	defer workConn.Close()
	log.Printf("HandleUserConnection#start UserConn L[%s] <->R[%s],workConn L[%s] <->R[%s]\n", conn.LocalAddr(), conn.RemoteAddr(), workConn.LocalAddr(), workConn.RemoteAddr())
	pipe.Trans(conn, workConn)
	//log.Printf("HandleUserConnection#end UserConn L[%s] <->R[%s],workConn L[%s] <->R[%s]\n", conn.LocalAddr(), conn.RemoteAddr(), workConn.LocalAddr(), workConn.RemoteAddr())

}

func (s *Server) HandleClientListener() {
	for {
		conn, err := s.Ln.Accept()
		if err != nil {
			log.Printf("HandleClientListener#Err %s \n", err)
			return
		} //处理内部数据
		go s.HandleConnection(conn)

	}
}

func (s *Server) HandleConnection(coon net.Conn) {
	//log.Printf("Server#HandleConnection...l[%s] t[%s] \n", coon.LocalAddr(), coon.RemoteAddr())
	_ = coon.SetReadDeadline(time.Now().Add(time.Second * 10))
	// 读消息
	// 判断消息是啥
	m, err := msg.RedMsg(coon)
	if err != nil {
		coon.Close()
		return
	}
	_ = coon.SetReadDeadline(time.Time{})
	switch m.MsgType {
	case msg.MsgTypeRegister:
		log.Printf("Server#HandleConnection get client register l[%s] t[%s] \n", coon.LocalAddr(), coon.RemoteAddr())
		s.cli = NewCli(coon)
		s.cli.Start()

	case msg.MsgTypeNewConn:
		//对面来的新链接 将其放到池子里去
		log.Printf("Server#HandleConnection get client newConn l[%s] t[%s] \n", coon.LocalAddr(), coon.RemoteAddr())
		if s.cli == nil {
			return
		}
		s.cli.workConn <- coon
	default:
		return
	}
}

type Cli struct {
	Conn         net.Conn
	readCh       chan *msg.Msg
	writeCh      chan *msg.Msg
	lastPingTime time.Time
	workConn     chan net.Conn
}

func NewCli(conn net.Conn) *Cli {
	return &Cli{
		Conn:     conn,
		readCh:   make(chan *msg.Msg, 10),
		writeCh:  make(chan *msg.Msg, 10),
		workConn: make(chan net.Conn, 10),
	}
}

func (c *Cli) Start() {
	//返回数据
	_ = msg.WriteMsg(c.Conn, &msg.Msg{
		MsgType: msg.MsgTypeRegisterResp,
	})

	go c.Read()
	go c.Write()
	go c.Run()
	log.Println("cli Start ...")

}

func (c *Cli) Run() {
	heartbeat := time.NewTicker(time.Second * 5)
	c.lastPingTime = time.Now()
	for {
		select {
		case <-heartbeat.C:
			if time.Since(c.lastPingTime) > time.Second*10 {
				log.Println("heartbeat time out")
				//return
			}
		case m := <-c.readCh:
			switch m.MsgType {
			case msg.MsgTypePing:
				c.lastPingTime = time.Now()
				c.writeCh <- &msg.Msg{
					MsgType: msg.MsgTypePong,
				}
				log.Println("Get ping msg ")
			}
		}
	}
}

func (c *Cli) Read() {
	for {
		m, err := msg.RedMsg(c.Conn)
		if err != nil {
			if err == io.EOF {
				log.Printf("connection is closed\n")
				return
			}
			c.Conn.Close()
			return
		}
		c.readCh <- m
	}
}

func (c *Cli) Write() {
	for {
		m, ok := <-c.writeCh
		if !ok {
			log.Println("writeCh close")
		}
		if err := msg.WriteMsg(c.Conn, m); err != nil {
			log.Printf("write err \n", err)
			return
		}
		//log.Println("write Msg,%s", m.MsgType)
	}
}

func (c *Cli) GetWorkConn() (net.Conn, error) {
	var conn net.Conn
	var ok bool
	select {
	case conn, ok = <-c.workConn:
		if !ok {
			return nil, errors.New("workConn closed")
		}
	default:
		c.writeCh <- &msg.Msg{
			MsgType: msg.MsgTypeNewConnReq,
		}
		select {
		case conn, ok = <-c.workConn:
			if !ok {
				return nil, errors.New("workConn closed")
			}
		case <-time.After(time.Second * 10):
			return nil, errors.New("timeout....")
		}
	}
	c.writeCh <- &msg.Msg{
		MsgType: msg.MsgTypeNewConnReq,
	}
	//log.Println("get workconn success")
	return conn, nil
}

func main() {
	ctx := context.Background()

	server, err := NewServer(&ServerConfig{
		ClientPort: "8888",
		ServerPort: "9999",
	})
	if err != nil {
		log.Fatalf(err.Error())
		return
	}
	fmt.Print("hello world server\n")
	server.Run(ctx)
}
