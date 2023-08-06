package graceful

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/golang/glog"
	"github.com/libp2p/go-reuseport"
	"io"
	"net/http"
	"sync"
)

var defaultTransferAddr = "127.0.0.1:10000"

type TransferService struct {
	transferServer *http.Server
	transferClient *http.Client
	hasParent      bool
	wg             sync.WaitGroup

	handlers map[string]gin.HandlerFunc
}

type HelloReq struct {
}
type HelloResp struct {
	Version int
}
type TransferReq struct {
}
type TransferResp struct {
	ServersTransferData map[string]string
}

func NewTransferService() *TransferService {
	t := &TransferService{}
	t.handlers = make(map[string]gin.HandlerFunc)

	return t
}

func (t *TransferService) RegisterHandler(path string, h gin.HandlerFunc) {
	t.handlers[path] = h
}

func (t *TransferService) connectParent() {
	t.transferClient = &http.Client{}
	t.transferClient.Transport = &http.Transport{}

	req := &HelloReq{}
	resp := &HelloResp{}
	if err := t.Request("hello", req, resp); err != nil {
		return
	}

	t.hasParent = true
}

func (t *TransferService) Run() {
	t.connectParent()
	t.wg.Add(1)
	go func() {
		t.serve()
		t.wg.Done()
	}()
}

func (t *TransferService) serve() (err error) {
	gin.SetMode(gin.ReleaseMode)
	g := gin.New()

	g.Handle("POST", "/hello", t.helloHandler)
	for k, v := range t.handlers {
		g.Handle("POST", k, v)
	}

	t.transferServer = &http.Server{
		Handler: g,
	}

	ln, err := reuseport.Listen("tcp", defaultTransferAddr)
	if err != nil {
		return err
	}

	return t.transferServer.Serve(ln)
}

func (t *TransferService) Request(service string, req any, resp any) (err error) {
	var resBody []byte

	defer func() {
		glog.Infof("Request %s, req %v, resp %v, body=%s, err=%v", service, req, resp, string(resBody), err)
	}()
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	rBody := bytes.NewReader(b)
	hResp, err := t.transferClient.Post(fmt.Sprintf("http://%s/%s", defaultTransferAddr, service), "application/json", rBody)
	if err != nil {
		return err
	}
	defer hResp.Body.Close()

	resBody, err = io.ReadAll(hResp.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(resBody, resp)

	return err
}

func (t *TransferService) helloHandler(c *gin.Context) {
	resp := &HelloResp{
		Version: 1,
	}
	c.JSON(http.StatusOK, resp)
}

func (t *TransferService) HasParent() bool {
	return t.hasParent
}
