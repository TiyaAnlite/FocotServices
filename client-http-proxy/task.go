package main

import (
	"k8s.io/klog/v2"
	"time"
)

type TaskScheduler struct {
	rateLimitMs time.Duration
	taskQueue   chan func()
	ticker      *time.Ticker
}

func (t *TaskScheduler) Start(limit ...time.Duration) {
	cfg.worker.Add(1)
	if len(limit) == 0 {
		klog.Warning("Rate limit unset")
	} else {
		t.rateLimitMs = limit[0]
		t.ticker = time.NewTicker(t.rateLimitMs)
		klog.Infof("Set rate limit at %v per request", t.rateLimitMs)
	}
	t.taskQueue = make(chan func(), 8192)
	t.scheduler()
}

func (t *TaskScheduler) scheduler() {
	klog.Info("TaskScheduler start")
	if t.ticker != nil {
		// Limit scheduler
		for {
			select {
			case <-t.ticker.C:
				if task := t.getTaskNow(); task != nil {
					task()
				}
			case <-cfg.worker.Ctx.Done():
				goto LastRunTick
			}
		}
	} else {
		// Unlimited scheduler
		for {
			select {
			case task := <-t.taskQueue:
				go task()
			case <-cfg.worker.Ctx.Done():
				goto LastRun
			}
		}
	}
LastRunTick:
	klog.Info("Waiting last task for tick")
	for {
		task := t.getTaskNow()
		if task == nil {
			goto Stop
		}
		<-t.ticker.C
		task()
	}
LastRun:
	klog.Info("Waiting last task")
	for task := range t.taskQueue {
		task()
	}
Stop:
	t.Stop()
	return
}

func (t *TaskScheduler) getTaskNow() func() {
	select {
	case task := <-t.taskQueue:
		return task
	default:
		return nil
	}
}

func (t *TaskScheduler) AddTask(task func(), qos ...int) {
	t.taskQueue <- task
}

// Stop Will call by ctx, not defer needed
func (t *TaskScheduler) Stop() {
	if t.ticker != nil {
		t.ticker.Stop()
	}
	klog.Info("TaskScheduler end")
	cfg.worker.Done()
}
