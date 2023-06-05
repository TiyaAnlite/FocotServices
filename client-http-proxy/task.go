package main

import (
	"k8s.io/klog/v2"
	"time"
)

type TaskScheduler struct {
	rateLimitMs time.Duration
	taskQueue   chan *Task
	ticker      *time.Ticker
}

type Task struct {
	JobFunc func()
	Qos     int
}

func (t *TaskScheduler) Start(limit ...time.Duration) {
	cfg.worker.Add(1)
	if len(limit) == 0 {
		klog.Warning("Rate limit unset, request will be unlimited")
	} else {
		t.rateLimitMs = limit[0]
		t.ticker = time.NewTicker(t.rateLimitMs)
		klog.Infof("Set rate limit at %v per request", t.rateLimitMs)
	}
	t.taskQueue = make(chan *Task, 8192)
	t.scheduler()
}

func (t *TaskScheduler) scheduler() {
	klog.Info("TaskScheduler start")
	if t.ticker != nil {
		// Limited scheduler
		for {
			select {
			case task := <-t.taskQueue:
				<-t.ticker.C // Wait for next tick
				go task.JobFunc()
			case <-cfg.worker.Ctx.Done():
				goto LastRunTick
			}
		}
	} else {
		// Unlimited scheduler
		for {
			select {
			case task := <-t.taskQueue:
				go task.JobFunc()
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
		task.JobFunc()
	}
LastRun:
	klog.Info("Waiting last task")
	for task := range t.taskQueue {
		task.JobFunc()
	}
Stop:
	t.Stop()
	return
}

func (t *TaskScheduler) getTaskNow() *Task {
	select {
	case task := <-t.taskQueue:
		return task
	default:
		return nil
	}
}

// AddTask Will add a ready task and wait for schedule, qos grow from 0 and value is 0 by default
func (t *TaskScheduler) AddTask(task func(), qos ...int) {
	var q int
	if len(qos) != 0 {
		if qos[0] < 0 {
			klog.Warningf("QoS[%d] < 0", qos[0])
		}
		q = qos[0]
	}
	t.taskQueue <- &Task{
		JobFunc: task,
		Qos:     q,
	}
}

// Stop Will call by ctx, not defer needed
func (t *TaskScheduler) Stop() {
	if t.ticker != nil {
		t.ticker.Stop()
	}
	klog.Info("TaskScheduler end")
	cfg.worker.Done()
}
