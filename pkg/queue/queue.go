package queue

import (
	"context"
	"github.com/bytedance/sonic"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Job represents a task queue job
type Job struct {
	ID        string                   `json:"id"`
	Type      string                   `json:"type"`
	Payload   map[string]interface{}   `json:"payload"`
	Status    string                   `json:"status"`
	Retries   int                      `json:"retries"`
	MaxRetries int                     `json:"max_retries"`
	Timeout   time.Duration            `json:"timeout"`
	ExpiresAt *time.Time               `json:"expires_at"`
	Queue     string                   `json:"queue"`
	CreatedAt time.Time                `json:"created_at"`
	StartedAt *time.Time               `json:"started_at"`
	FinishedAt *time.Time              `json:"finished_at"`
	Error     string                   `json:"error,omitempty"`
	Metadata  map[string]interface{}   `json:"metadata,omitempty"`
}

// JobStatus represents job states
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
	StatusDead      = "dead" // Dead letter queue
)

// QueueConfig holds queue configuration
type QueueConfig struct {
	Host            string        `mapstructure:"host"`
	Port            string        `mapstructure:"port"`
	Password        string        `mapstructure:"password"`
	DB              int           `mapstructure:"db"`
	DefaultQueue    string        `mapstructure:"default_queue"`
	DefaultRetries  int           `mapstructure:"default_retries"`
	DefaultTimeout  time.Duration `mapstructure:"default_timeout"`
	PoolSize        int           `mapstructure:"pool_size"`
}

// TaskQueue manages the job queue
type TaskQueue struct {
	client     *redis.Client
	defaultQueue string
	config     QueueConfig
}

// NewTaskQueue creates a new task queue
func NewTaskQueue(config *QueueConfig) (*TaskQueue, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", config.Host, config.Port),
		Password: config.Password,
		DB:       config.DB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &TaskQueue{
		client:         client,
		defaultQueue:   config.DefaultQueue,
		config:         *config,
	}, nil
}

// Enqueue adds a job to the queue
func (q *TaskQueue) Enqueue(job *Job) error {
	job.ID = generateJobID()
	job.Status = StatusPending
	job.CreatedAt = time.Now()
	job.Queue = q.normalizeQueue(job.Queue)
	if job.MaxRetries == 0 {
		job.MaxRetries = q.config.DefaultRetries
	}
	if job.Timeout == 0 {
		job.Timeout = q.config.DefaultTimeout
	}

	data, err := sonic.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Push to queue list
	return q.client.RPush(ctx, q.queueKey(job.Queue), string(data)).Err()
}

// Dequeue removes and returns the next job from the queue
func (q *TaskQueue) Dequeue(queueName string) (*Job, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Blocking pop with timeout
	result, err := q.client.BRPop(ctx, 30*time.Second, q.normalizeQueue(queueName)).Result()
	if err != nil {
		return nil, err
	}

	// result[0] is the key, result[1] is the job JSON
	var job Job
	if err := sonic.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	return &job, nil
}

// Complete marks a job as completed
func (q *TaskQueue) Complete(jobID, queueName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := q.jobKey(jobID, queueName)
	return q.client.Del(ctx, key).Err()
}

// Fail marks a job as failed and may retry
func (q *TaskQueue) Fail(jobID, queueName string, err error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := q.jobKey(jobID, queueName)
	job, err := q.Get(jobID, queueName)
	if err != nil {
		return err
	}

	job.Retries++
	job.Error = err.Error()
	job.Status = StatusFailed
	job.FinishedAt = now()

	if job.Retries >= job.MaxRetries {
		// Move to dead letter queue
		job.Status = StatusDead
		return q.moveToDLQ(ctx, job)
	}

	// Update job state
	data, err := sonic.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return q.client.Set(ctx, key, string(data), 0).Err()
}

// Get retrieves a job by ID
func (q *TaskQueue) Get(jobID, queueName string) (*Job, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := q.jobKey(jobID, queueName)
	data, err := q.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}

	var job Job
	if err := sonic.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	return &job, nil
}

// Start marks a job as running
func (q *TaskQueue) Start(jobID, queueName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := q.jobKey(jobID, queueName)
	job, err := q.Get(jobID, queueName)
	if err != nil {
		return err
	}

	job.Status = StatusRunning
	job.StartedAt = now()

	data, err := sonic.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return q.client.Set(ctx, key, string(data), job.Timeout).Err()
}

// PubSub creates a pub/sub channel for job events
func (q *TaskQueue) PubSub(channel string) *redis.PubSub {
	return q.client.Subscribe(context.Background(), q.pubSubChannel(channel))
}

// Publish publishes a message to a channel
func (q *TaskQueue) Publish(channel, message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return q.client.Publish(ctx, q.pubSubChannel(channel), message).Err()
}

// worker implements the QueueWorker interface
type Worker struct {
	queue        *TaskQueue
	handlers     map[string]JobHandler
	poolSize     int
	running      bool
	stopChan     chan struct{}
}

// NewWorker creates a new worker
func NewWorker(queue *TaskQueue, poolSize int) *Worker {
	return &Worker{
		queue:    queue,
		handlers: make(map[string]JobHandler),
		poolSize: poolSize,
		stopChan: make(chan struct{}),
	}
}

// Register registers a job handler
func (w *Worker) Register(jobType string, handler JobHandler) {
	w.handlers[jobType] = handler
}

// Start starts the worker pool
func (w *Worker) Start() {
	w.running = true
	for i := 0; i < w.poolSize; i++ {
		go w.worker(i)
	}
}

// Stop stops the worker pool
func (w *Worker) Stop() {
	w.running = false
	close(w.stopChan)
}

func (w *Worker) worker(id int) {
	for {
		if !w.running {
			return
		}

		job, err := w.queue.Dequeue("")
		if err != nil {
			continue
		}

		handler, ok := w.handlers[job.Type]
		if !ok {
			w.queue.Fail(job.ID, job.Queue, fmt.Errorf("no handler for job type: %s", job.Type))
			continue
		}

		// Start job
		w.queue.Start(job.ID, job.Queue)

		// Execute handler
		err = handler.Handle(job)

		// Complete or fail job
		if err != nil {
			w.queue.Fail(job.ID, job.Queue, err)
		} else {
			w.queue.Complete(job.ID, job.Queue)
		}
	}
}

// JobHandler is an interface for job handlers
type JobHandler interface {
	Handle(job *Job) error
}

// Helper functions
func (q *TaskQueue) normalizeQueue(queueName string) string {
	if queueName == "" {
		return q.defaultQueue
	}
	return queueName
}

func (q *TaskQueue) queueKey(queueName string) string {
	return "queue:" + queueName
}

func (q *TaskQueue) jobKey(jobID, queueName string) string {
	return "job:" + jobID + ":" + q.normalizeQueue(queueName)
}

func (q *TaskQueue) pubSubChannel(channel string) string {
	return "pubsub:jobs:" + channel
}

func (q *TaskQueue) moveToDLQ(ctx context.Context, job *Job) error {
	dlqKey := "dlq:" + job.Queue
	data, err := sonic.Marshal(job)
	if err != nil {
		return err
	}
	return q.client.RPush(ctx, dlqKey, string(data)).Err()
}

func generateJobID() string {
	return fmt.Sprintf("job_%d", time.Now().UnixNano())
}

func now() *time.Time {
	t := time.Now()
	return &t
}

// Close closes the Redis connection
func (q *TaskQueue) Close() error {
	if q.client != nil {
		return q.client.Close()
	}
	return nil
}
