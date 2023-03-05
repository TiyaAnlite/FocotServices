package main

import (
	"k8s.io/klog/v2"
	"time"
)

func doSync() {
	cfg.worker.Add(1)
	defer cfg.worker.Done()
	klog.Infof("Starting sync")
	ticker := time.NewTicker(time.Duration(int64(time.Second) * int64(cfg.RefreshInterval)))
	for {
		select {
		case <-ticker.C:
		case <-cfg.worker.Ctx.Done():

		}
	}
}

func doRequest(recorderUrl string) {

}
