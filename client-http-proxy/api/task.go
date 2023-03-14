package api

import "go.opentelemetry.io/otel/trace"

const (
	TASK_ON_PROCESS = iota
	TASK_SCHEDULED
	TASK_FINISHED
	TASK_FAILED
)

type TaskStatus int

type RequestProcessMsg struct {
	Status    TaskStatus    `json:"status"`
	RequestId string        `json:"request_id"`
	TraceId   trace.TraceID `json:"trace_id"`
}
