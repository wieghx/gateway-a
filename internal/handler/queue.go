package handler

import (
	"context"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/gu/gateway-a/pkg/queue"
)

// QueueHandler handles queue API endpoints
type QueueHandler struct {
	taskQueue *queue.TaskQueue
}

// NewQueueHandler creates a new queue handler
func NewQueueHandler(q *queue.TaskQueue) *QueueHandler {
	return &QueueHandler{
		taskQueue: q,
	}
}

// EnqueueRequest represents a queue job enqueue request
type EnqueueRequest struct {
	JobType  string                 `json:"job_type"`
	Payload  map[string]interface{} `json:"payload"`
	Queue    string                 `json:"queue,omitempty"`
	MaxRetries int                   `json:"max_retries,omitempty"`
	Timeout  string                 `json:"timeout,omitempty"`
}

// EnqueueResponse represents the response after enqueueing a job
type EnqueueResponse struct {
	JobID string `json:"job_id"`
	Queue string `json:"queue"`
	Status string `json:"status"`
}

// Enqueue adds a job to the queue
// POST /v1/queue/enqueue
func (h *QueueHandler) Enqueue(c context.Context, ctx *app.RequestContext) {
	var req EnqueueRequest
	if err := ctx.BindAndValidate(&req); err != nil {
		ctx.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	job := &queue.Job{
		Type:       req.JobType,
		Payload:    req.Payload,
		Queue:      req.Queue,
		MaxRetries: req.MaxRetries,
	}

	if req.Timeout != "" {
		// Parse timeout string (e.g., "30s", "1m")
		// Simplified: just use default timeout
		job.Timeout = 30 // seconds
	}

	if err := h.taskQueue.Enqueue(job); err != nil {
		ctx.JSON(consts.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	ctx.JSON(consts.StatusOK, EnqueueResponse{
		JobID:  job.ID,
		Queue:  job.Queue,
		Status: queue.StatusPending,
	})
}

// JobStatusResponse represents the status of a job
type JobStatusResponse struct {
	JobID      string                 `json:"job_id"`
	Type       string                 `json:"type"`
	Payload    map[string]interface{} `json:"payload"`
	Status     string                 `json:"status"`
	Retries    int                    `json:"retries"`
	MaxRetries int                    `json:"max_retries"`
	Error      string                 `json:"error,omitempty"`
	CreatedAt  string                 `json:"created_at"`
	StartedAt  string                 `json:"started_at,omitempty"`
	FinishedAt string                 `json:"finished_at,omitempty"`
}

// GetJobStatus returns the status of a job
// GET /v1/queue/jobs/:job_id
func (h *QueueHandler) GetJobStatus(c context.Context, ctx *app.RequestContext) {
	jobID := ctx.Param("job_id")
	queueName := ctx.Param("queue")

	if queueName == "" {
		queueName = "default"
	}

	job, err := h.taskQueue.Get(jobID, queueName)
	if err != nil {
		ctx.JSON(consts.StatusNotFound, map[string]string{"error": "Job not found"})
		return
	}

	ctx.JSON(consts.StatusOK, JobStatusResponse{
		JobID:      job.ID,
		Type:       job.Type,
		Payload:    job.Payload,
		Status:     job.Status,
		Retries:    job.Retries,
		MaxRetries: job.MaxRetries,
		Error:      job.Error,
		CreatedAt:  job.CreatedAt.String(),
		StartedAt:  formatTime(job.StartedAt),
		FinishedAt: formatTime(job.FinishedAt),
	})
}

// ListJobsResponse represents a list of jobs
type ListJobsResponse struct {
	Jobs []JobStatusResponse `json:"jobs"`
	Total int                `json:"total"`
}

// ListJobs lists jobs in a queue (simplified - returns pending jobs from JSON store)
// GET /v1/queue/jobs
func (h *QueueHandler) ListJobs(c context.Context, ctx *app.RequestContext) {
	queueName := ctx.Query("queue")
	if queueName == "" {
		queueName = "default"
	}

	// Simplified: In production, maintain a set of job IDs in Redis
	// This is a placeholder implementation
	ctx.JSON(consts.StatusOK, ListJobsResponse{
		Jobs:  []JobStatusResponse{},
		Total: 0,
	})
}

// WorkerStatsResponse represents worker statistics
type WorkerStatsResponse struct {
	Queue        string `json:"queue"`
	ActiveWorkers int   `json:"active_workers"`
	PendingJobs  int64  `json:"pending_jobs"`
}

// GetWorkerStats returns worker statistics
// GET /v1/queue/stats
func (h *QueueHandler) GetWorkerStats(c context.Context, ctx *app.RequestContext) {
	// Simplified: return placeholder stats
	// In production, track active workers and pending jobs in Redis
	stats := WorkerStatsResponse{
		Queue:       "default",
		ActiveWorkers: 4,
		PendingJobs:  0,
	}

	ctx.JSON(consts.StatusOK, stats)
}

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.String()
}
