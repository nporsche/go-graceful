package graceful

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/golang/glog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"tableflip.test.com/pkg/api"
	"tableflip.test.com/pkg/server"
	"tableflip.test.com/tableflip"
)

type Upgrade struct {
	upg      *tableflip.Upgrader
	transfer *TransferService
	servers  map[string]api.Server
}

func NewUpgrade(pidFile string) *Upgrade {
	upg, _ := tableflip.New(tableflip.Options{
		PIDFile: pidFile,
	})

	u := &Upgrade{
		transfer: NewTransferService(),
		upg:      upg,
		servers:  make(map[string]api.Server),
	}

	ctx := context.Background()

	udsServer := server.NewUDSServerImpl(ctx, upg, "unix", "unix.sock")
	u.servers[udsServer.Name()] = udsServer

	u.transfer.RegisterHandler("/transfer", u.parentHandleTransferRequest)

	return u
}

func (u *Upgrade) Do() {
	go u.watchSignal()

	for _, s := range u.servers {
		s.Run()
	}

	u.transfer.Run()
	if u.transfer.HasParent() {
		req := &TransferReq{}
		resp := &TransferResp{}
		glog.Info("request to transfer...")
		if err := u.transfer.Request("transfer", req, resp); err != nil {
			glog.Error("transfer request failed", err)
		}
		u.childRestoreTransferData(resp)
	}
	glog.Info("Transfer finished")

	u.upg.Ready()
	glog.Infoln("upg ready")

	<-u.upg.Exit()
	glog.Infoln("upg exit")
}

func (u *Upgrade) childRestoreTransferData(res *TransferResp) {
	glog.Infof("child restore transfer data %v", res)
	for k, v := range res.ServersTransferData {
		u.servers[k].Restore(v)
	}
}

func (u *Upgrade) parentHandleTransferRequest(c *gin.Context) {
	resp := &TransferResp{
		ServersTransferData: make(map[string]string),
	}
	for _, s := range u.servers {
		name := s.Name()
		ok, data := s.Transfer()
		if ok {
			resp.ServersTransferData[name] = data
		}
	}

	c.JSON(http.StatusOK, resp)
}

func (u *Upgrade) watchSignal() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGTERM)
	for s := range sig {
		if s == syscall.SIGHUP {
			err := u.upg.Upgrade()
			if err != nil {
				glog.Infoln("Upgrade failed:", err)
			}
		}
		if s == syscall.SIGTERM {
			u.upg.Stop()
			glog.Infoln("term signal, stopping")
		}
	}
}
