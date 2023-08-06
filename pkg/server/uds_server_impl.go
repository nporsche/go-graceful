package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"github.com/google/uuid"
	"go-graceful/tableflip"
	"go.uber.org/atomic"
	"net"
	"os"
	"sync"
	"time"
)

type UDSServerImpl struct {
	network string
	addr    string
	upg     *tableflip.Upgrader
	ctx     context.Context
	cancel  context.CancelFunc

	isTransfer   *atomic.Bool
	transferData *UDSServerImplTransferData
	lock         sync.Mutex

	ln net.Listener

	wg sync.WaitGroup
}

type UDSConnection struct {
	id string
	c  net.Conn
}

type UDSServerImplTransferData struct {
	Conns map[string]net.Conn
}

func NewUDSServerImpl(c context.Context, upg *tableflip.Upgrader, network, addr string) *UDSServerImpl {
	s := &UDSServerImpl{}
	s.ctx, s.cancel = context.WithCancel(c)
	s.isTransfer = atomic.NewBool(false)
	s.transferData = &UDSServerImplTransferData{
		Conns: make(map[string]net.Conn),
	}
	s.upg = upg
	s.addr = addr
	s.network = network
	return s
}

func (s *UDSServerImpl) Name() string {
	return "uds_server"
}

func (s *UDSServerImpl) Transfer() (bool, string) {
	s.isTransfer.Store(true)
	s.Stop()

	b, _ := json.Marshal(s.transferData)
	glog.Infof("transfer data %s", string(b))
	return true, string(b)
}

func (s *UDSServerImpl) Restore(data string) {
	transData := &UDSServerImplTransferData{}
	json.Unmarshal([]byte(data), transData)

	glog.Infof("restore data %s", data)
	for k, _ := range transData.Conns {
		connId := k
		c, e := s.upg.Conn(s.network, connId)
		if e != nil {
			glog.Errorf("restore %s error %v", connId, e)
			continue
		}
		glog.Infof("restore conn %s", connId)
		go s.connServe(connId, c)
	}
}

func (s *UDSServerImpl) Run() error {
	return s.listenAndServe()
}

func (s *UDSServerImpl) Stop() {
	s.ln.Close()
	s.cancel()
	s.waitExit()
}

func (s *UDSServerImpl) waitExit() {
	s.wg.Wait()
}

func (s *UDSServerImpl) AddConn(connID string, c net.Conn) {
	s.lock.Lock()
	s.transferData.Conns[connID] = c
	s.lock.Unlock()

	s.upg.Fds.AddConn(s.network, connID, c.(tableflip.Conn))
}

func (s *UDSServerImpl) RemoveConn(connID string) {
	s.lock.Lock()
	delete(s.transferData.Conns, connID)
	s.lock.Unlock()

	s.upg.Fds.RemoveConn(s.network, connID)
}

func (s *UDSServerImpl) listenAndServe() (err error) {
	s.ln, err = s.upg.Listen(s.network, s.addr)
	if err != nil {
		glog.Error("listen", err)
		return err
	}

	go func() {
		s.wg.Add(1)
		defer s.wg.Done()

		glog.Infof("listening on %s", s.ln.Addr())

		for {
			c, err := s.ln.Accept()
			if err != nil {
				glog.Errorf("accept error %s", err)
				return
			}

			id, _ := uuid.NewUUID()
			go s.connServe(id.String(), c)
		}
	}()

	return nil
}

func (s *UDSServerImpl) connServe(connID string, c net.Conn) {
	glog.Infof("accept and serving conn %s", c.RemoteAddr().String())
	s.AddConn(connID, c)
	s.wg.Add(1)
	defer func() {
		c.Close()
		s.wg.Done()
		glog.Infof("conn %s exit, isTransfer=%v", connID, s.isTransfer.Load())
	}()
	exit := false
	for !exit {
		select {
		case <-s.ctx.Done():
			exit = true
		default:
			c.SetDeadline(time.Now().Add(10 * time.Millisecond))
			line, _, err := bufio.NewReader(c).ReadLine()
			if os.IsTimeout(err) {
				continue
			}
			if err != nil {
				glog.Infof("Failure to read:%s", err.Error())
				return
			}
			msg := fmt.Sprintf("[%d]echo %s\n", os.Getpid(), line)
			_, err = c.Write([]byte(msg))
			if err != nil {
				glog.Infof("Failure to write: %s", err.Error())
				return
			}
		}
	}
	if s.isTransfer.Load() {
		return
	}
	s.RemoveConn(connID)
}
