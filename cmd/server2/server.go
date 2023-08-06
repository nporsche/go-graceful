package main

import (
	"flag"
	"github.com/golang/glog"
	"tableflip.test.com/pkg/graceful"
	"time"
)

func main() {
	var (
		pidFile = flag.String("pid-file", "main.pid", "`Path` to pid file")
	)
	flag.Parse()
	defer glog.Flush()

	go func() {
		t := time.Tick(10 * time.Millisecond)
		for {
			<-t
			glog.Flush()
		}
	}()

	up := graceful.NewUpgrade(*pidFile)
	up.Do()
}
