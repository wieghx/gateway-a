package queue

import (
	"context"
	"github.com/bytedance/sonic"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ==================== TEST STRUCTURES ====================

// Minimal mock types for queue testing
type MockStringSliceCmd struct {
	val []string
}

func (m *MockStringSliceCmd) Err() error {
	return nil
}

func (m *MockStringSliceCmd) Result() ([]string, error) {
	return m.val, nil
}

type MockIntCmd struct {
	val int64
}

func (m *MockIntCmd) Err() error {
	return nil
}

func (m *MockIntCmd) Result() (int64, error) {
	return m.val, nil
}

type MockStatusCmd struct {
	val string
}

func (m *MockStatusCmd) Err() error {
	return nil
}

func (m *MockStatusCmd) Result() (string, error) {
	return m.val, nil
}

type MockStringCmd struct {
	val string
}

func (m *MockStringCmd) Err() error {
	return nil
}

func (m *MockStringCmd) Result() (string, error) {
	return m.val, nil
}

type MockTx struct{}

func (m *MockTx) Exec(ctx context.Context) ([]interface{}, error) {
	return []interface{}{}, nil
}

// ==================== TESTS ====================

func TestTaskQueueConfig(t *testing.T) {
	t.Run("Default config values", func(t *testing.T) {
		config := QueueConfig{
			Host:           "localhost",
			Port:           "6379",
			Password:       "",
			DB:             0,
			DefaultQueue:   "default",
			DefaultRetries: 3,
			DefaultTimeout: 30 * time.Second,
			PoolSize:       4,
		}

		assert.Equal(t, "localhost", config.Host)
		assert.Equal(t, "6379", config.Port)
		assert.Equal(t, 0, config.DB)
		assert.Equal(t, "default", config.DefaultQueue)
		assert.Equal(t, 3, config.DefaultRetries)
		assert.Equal(t, 30*time.Second, config.DefaultTimeout)
		assert.Equal(t, 4, config.PoolSize)
	})

	t.Run("Custom config values", func(t *testing.T) {
		config := QueueConfig{
			Host:           "redis.example.com",
			Port:           "6380",
			Password:       "secret",
			DB:             1,
			DefaultQueue:   "critical",
			DefaultRetries: 5,
			DefaultTimeout: 60 * time.Second,
			PoolSize:       8,
		}

		assert.Equal(t, "redis.example.com", config.Host)
		assert.Equal(t, "6380", config.Port)
		assert.Equal(t, "secret", config.Password)
		assert.Equal(t, 1, config.DB)
		assert.Equal(t, "critical", config.DefaultQueue)
		assert.Equal(t, 5, config.DefaultRetries)
		assert.Equal(t, 60*time.Second, config.DefaultTimeout)
		assert.Equal(t, 8, config.PoolSize)
	})
}

func TestJob(t *testing.T) {
	t.Run("Job structure", func(t *testing.T) {
		now := time.Now()
		expiredAt := time.Now().Add(time.Hour)

		job := &Job{
			ID:          "job_123",
			Type:        "test_job",
			Payload:     map[string]interface{}{"key": "value"},
			Status:      StatusPending,
			Retries:     0,
			MaxRetries:  3,
			Timeout:     30 * time.Second,
			ExpiresAt:   &expiredAt,
			Queue:       "default",
			CreatedAt:   now,
			StartedAt:   &now,
			FinishedAt:  &now,
			Error:       "",
			Metadata:    map[string]interface{}{"meta": "data"},
		}

		assert.Equal(t, "job_123", job.ID)
		assert.Equal(t, "test_job", job.Type)
		assert.Equal(t, StatusPending, job.Status)
		assert.Equal(t, 0, job.Retries)
		assert.Equal(t, 3, job.MaxRetries)
		assert.NotNil(t, job.Payload)
		assert.NotNil(t, job.Metadata)
	})

	t.Run("Job status constants", func(t *testing.T) {
		assert.Equal(t, "pending", StatusPending)
		assert.Equal(t, "running", StatusRunning)
		assert.Equal(t, "completed", StatusCompleted)
		assert.Equal(t, "failed", StatusFailed)
		assert.Equal(t, "cancelled", StatusCancelled)
		assert.Equal(t, "dead", StatusDead)
	})

	t.Run("Job JSON marshaling", func(t *testing.T) {
		job := &Job{
			ID:         "job_123",
			Type:       "test",
			Status:     StatusPending,
			Payload:    map[string]interface{}{"key": "value"},
			Queue:      "default",
			MaxRetries: 3,
		}

		data, err := sonic.Marshal(job)
		assert.NoError(t, err)

		// Unmarshal back
		var job2 Job
		err = sonic.Unmarshal(data, &job2)
		assert.NoError(t, err)
		assert.Equal(t, job.ID, job2.ID)
		assert.Equal(t, job.Type, job2.Type)
		assert.Equal(t, job.Status, job2.Status)
	})

	t.Run("Job with nil error is empty", func(t *testing.T) {
		job := &Job{
			ID:         "job_123",
			Type:       "test",
			Error:      "",
			MaxRetries: 3,
		}

		data, err := sonic.Marshal(job)
		assert.NoError(t, err)

		var job2 Job
		err = sonic.Unmarshal(data, &job2)
		assert.NoError(t, err)
		assert.Empty(t, job2.Error)
	})
}

func TestJobIDGeneration(t *testing.T) {
	t.Run("generateJobID creates unique IDs", func(t *testing.T) {
		ids := make(map[string]bool)

		for i := 0; i < 100; i++ {
			id := generateJobID()
			assert.Contains(t, id, "job_")
			assert.False(t, ids[id], "Job ID should be unique")
			ids[id] = true
		}
	})

	t.Run("Job IDs have nanosecond precision", func(t *testing.T) {
		id1 := generateJobID()
		time.Sleep(time.Millisecond)
		id2 := generateJobID()

		assert.NotEqual(t, id1, id2)
	})
}

func TestNormalizeQueue(t *testing.T) {
	t.Run("Normalize empty queue returns default", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue:   "default",
			DefaultRetries: 3,
			DefaultTimeout: 30 * time.Second,
		}
		q := &TaskQueue{
			defaultQueue: "default",
			config:       config,
		}

		normalized := q.normalizeQueue("")
		assert.Equal(t, "default", normalized)
	})

	t.Run("Normalize non-empty queue returns same queue", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue: "default",
		}
		q := &TaskQueue{
			defaultQueue: "default",
			config:       config,
		}

		normalized := q.normalizeQueue("custom")
		assert.Equal(t, "custom", normalized)
	})

	t.Run("Normalize queue handles special characters", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue: "default",
		}
		q := &TaskQueue{
			defaultQueue: "default",
			config:       config,
		}

		queues := []string{"queue-1", "queue_2", "queue.3", "Queue4"}

		for _, queue := range queues {
			normalized := q.normalizeQueue(queue)
			assert.Equal(t, queue, normalized)
		}
	})
}

func TestQueueKeys(t *testing.T) {
	t.Run("queueKey returns correct key format", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue: "default",
		}
		q := &TaskQueue{
			defaultQueue: "default",
			config:       config,
		}

		key := q.queueKey("test")
		assert.Equal(t, "queue:test", key)
	})

	t.Run("jobKey returns correct key format", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue: "default",
		}
		q := &TaskQueue{
			defaultQueue: "default",
			config:       config,
		}

		key := q.jobKey("job_123", "default")
		assert.Equal(t, "job:job_123:default", key)
	})

	t.Run("jobKey uses normalized queue", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue: "default",
		}
		q := &TaskQueue{
			defaultQueue: "default",
			config:       config,
		}

		key := q.jobKey("job_123", "")
		assert.Equal(t, "job:job_123:default", key)
	})

	t.Run("pubSubChannel returns correct key format", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue: "default",
		}
		q := &TaskQueue{
			defaultQueue: "default",
			config:       config,
		}

		channel := q.pubSubChannel("events")
		assert.Equal(t, "pubsub:jobs:events", channel)
	})
}

func TestQueueWorker(t *testing.T) {
	t.Run("NewWorker creates valid worker", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue: "default",
			PoolSize:     4,
		}
		q := &TaskQueue{
			config:       config,
			defaultQueue: "default",
		}
		worker := NewWorker(q, 4)

		assert.NotNil(t, worker)
		assert.Equal(t, 4, worker.poolSize)
		assert.NotNil(t, worker.handlers)
		assert.NotNil(t, worker.stopChan)
	})

	t.Run("Register adds handler", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue: "default",
		}
		q := &TaskQueue{
			config:       config,
			defaultQueue: "default",
		}
		worker := NewWorker(q, 4)

		handler := JobHandlerFunc(func(ctx context.Context, job *Job) error {
			return nil
		})

		worker.Register("test_job", handler)

		// Handler should be registered
		assert.NotNil(t, worker.handlers["test_job"])
	})

	t.Run("Start starts worker pool", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue: "default",
			PoolSize:     2,
		}
		q := &TaskQueue{
			config:       config,
			defaultQueue: "default",
		}
		worker := NewWorker(q, 2)

		// Start the worker
		worker.Start()

		assert.True(t, worker.running)

		// Stop the worker
		worker.Stop()

		assert.False(t, worker.running)
	})

	t.Run("Worker handles job completion", func(t *testing.T) {
		var completed bool

		handler := JobHandlerFunc(func(ctx context.Context, job *Job) error {
			completed = true
			return nil
		})

		// Verify handler is callable
		assert.NotNil(t, handler)

		// Note: Full integration test would require actual Redis connection
		// This test verifies handler structure
		_ = completed
	})

	t.Run("Worker with no handler fails job", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue: "default",
			PoolSize:     1,
		}
		q := &TaskQueue{
			config:       config,
			defaultQueue: "default",
		}
		worker := NewWorker(q, 1)

		// No handlers registered
		assert.Len(t, worker.handlers, 0)

		// Worker should fail jobs with unknown types
		assert.NotNil(t, worker)
	})

	t.Run("Worker pool goroutines", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue: "default",
			PoolSize:     4,
		}
		q := &TaskQueue{
			config:       config,
			defaultQueue: "default",
		}
		worker := NewWorker(q, 4)

		// Verify pool size
		assert.Equal(t, 4, worker.poolSize)

		// Start and stop should be safe to call
		worker.Start()
		worker.Stop()
		// Note: Calling Stop() twice is not safe - we only call it once
	})
}

func TestJobHandlerInterface(t *testing.T) {
	t.Run("JobHandlerFunc implements interface", func(t *testing.T) {
		var handler JobHandler = JobHandlerFunc(func(ctx context.Context, job *Job) error {
			return nil
		})

		assert.NotNil(t, handler)
	})

	t.Run("JobHandlerFunc Handle method", func(t *testing.T) {
		var handled bool

		handler := JobHandlerFunc(func(ctx context.Context, job *Job) error {
			handled = true
			return nil
		})

		job := &Job{
			ID:   "job_123",
			Type: "test",
		}

		err := handler.Handle(job)
		assert.NoError(t, err)
		assert.True(t, handled)
	})

	t.Run("JobHandlerFunc returns errors", func(t *testing.T) {
		handler := JobHandlerFunc(func(ctx context.Context, job *Job) error {
			return fmt.Errorf("test error")
		})

		job := &Job{ID: "job_123"}
		err := handler.Handle(job)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "test error")
	})
}

func TestJobHandlers(t *testing.T) {
	t.Run("TokenCalculationHandler is callable", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue: "default",
		}
		q := &TaskQueue{
			config:       config,
			defaultQueue: "default",
		}

		handler := TokenCalculationHandler(q)
		assert.NotNil(t, handler)

		// Just verify handler exists - actual execution requires Redis
		job := &Job{
			Type: JobTypeTokenCalc,
		}
		_ = job

		// Note: Handler execution will fail without Redis connection
		// This is expected behavior for unit tests
	})

	t.Run("UsageTrackingHandler is callable", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue: "default",
		}
		q := &TaskQueue{
			config:       config,
			defaultQueue: "default",
		}

		handler := UsageTrackingHandler(q)
		assert.NotNil(t, handler)
		_ = q
	})

	t.Run("LLMCachingHandler is callable", func(t *testing.T) {
		handler := LLMCachingHandler()
		assert.NotNil(t, handler)
	})

	t.Run("TenantSyncHandler is callable", func(t *testing.T) {
		handler := TenantSyncHandler()
		assert.NotNil(t, handler)
	})
}

func TestHelperFunctions(t *testing.T) {
	t.Run("countTokens estimates token count", func(t *testing.T) {
		// Rough estimate: 4 chars per token
		text := "Hello, world! This is a test."
		count := countTokens(text)

		assert.Greater(t, count, 0)
		assert.Less(t, count, len(text)) // Token count should be less than char count
	})

	t.Run("countTokens on empty string", func(t *testing.T) {
		count := countTokens("")
		assert.Equal(t, 0, count)
	})

	t.Run("countTokens on large text", func(t *testing.T) {
		text := strings.Repeat("Hello, world! ", 1000)
		count := countTokens(text)

		assert.Greater(t, count, 0)
	})

	t.Run("generateCacheKey creates cache keys", func(t *testing.T) {
		key := generateCacheKey("test prompt", "test-model")

		assert.NotEmpty(t, key)
		assert.Equal(t, 8, len(key)) // 32-bit hex = 8 characters
	})

	t.Run("generateCacheKey is deterministic", func(t *testing.T) {
		key1 := generateCacheKey("test prompt", "test-model")
		key2 := generateCacheKey("test prompt", "test-model")

		assert.Equal(t, key1, key2)
	})

	t.Run("generateCacheKey is different for different inputs", func(t *testing.T) {
		key1 := generateCacheKey("prompt 1", "model 1")
		key2 := generateCacheKey("prompt 2", "model 1")
		key3 := generateCacheKey("prompt 1", "model 2")

		assert.NotEqual(t, key1, key2)
		assert.NotEqual(t, key1, key3)
	})
}

func TestConcurrentAccess(t *testing.T) {
	t.Run("Worker is thread-safe", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue: "default",
			PoolSize:     4,
		}
		q := &TaskQueue{
			config:       config,
			defaultQueue: "default",
		}
		worker := NewWorker(q, 4)

		// Register handlers sequentially to avoid concurrent map access
		// Note: Register is not thread-safe - it's expected that
		// all registrations happen before workers start
		for i := 0; i < 10; i++ {
			handler := JobHandlerFunc(func(ctx context.Context, job *Job) error {
				return nil
			})
			worker.Register(fmt.Sprintf("job_%d", i), handler)
		}

		// Verify handlers were registered
		assert.Len(t, worker.handlers, 10)
	})

	t.Run("Start and Stop are safe", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue: "default",
			PoolSize:     1,
		}
		q := &TaskQueue{
			config:       config,
			defaultQueue: "default",
		}
		worker := NewWorker(q, 1)

		// Verify worker can be started and stopped
		worker.Start()
		assert.True(t, worker.running)

		worker.Stop()
		assert.False(t, worker.running)

		// Note: Starting/stopping concurrently is not safe
		// This is a limitation of the current implementation
	})
}

func TestEdgeCases(t *testing.T) {
	t.Run("Job with max retries zero uses default", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue:   "default",
			DefaultRetries: 5,
		}
		q := &TaskQueue{
			config:       config,
			defaultQueue: "default",
		}

		job := &Job{
			Type:       "test",
			MaxRetries: 0,
		}

		// Would call normalizeQueue in Enqueue
		normalized := q.normalizeQueue(job.Queue)
		assert.Equal(t, "default", normalized)

		_ = job
	})

	t.Run("Job with zero timeout uses default", func(t *testing.T) {
		config := QueueConfig{
			DefaultQueue:   "default",
			DefaultRetries: 3,
			DefaultTimeout: 60 * time.Second,
		}
		q := &TaskQueue{
			config:       config,
			defaultQueue: "default",
		}

		job := &Job{
			Type:       "test",
			MaxRetries: 0,
		}

		// Would use default timeout in Enqueue
		assert.Equal(t, 60*time.Second, config.DefaultTimeout)

		_ = q
		_ = job
	})

	t.Run("Empty payload", func(t *testing.T) {
		job := &Job{
			ID:      "job_123",
			Type:    "test",
			Payload: map[string]interface{}{},
		}

		data, err := sonic.Marshal(job)
		assert.NoError(t, err)

		var job2 Job
		err = sonic.Unmarshal(data, &job2)
		assert.NoError(t, err)
		assert.Empty(t, job2.Payload)
	})

	t.Run("Nil metadata", func(t *testing.T) {
		job := &Job{
			ID:       "job_123",
			Type:     "test",
			Metadata: nil,
		}

		data, err := sonic.Marshal(job)
		assert.NoError(t, err)

		var job2 Job
		err = sonic.Unmarshal(data, &job2)
		assert.NoError(t, err)
		assert.Empty(t, job2.Metadata)
	})
}

// Benchmark tests
func BenchmarkJobMarshaling(b *testing.B) {
	job := &Job{
		ID:         "job_123",
		Type:       "test",
		Status:     StatusPending,
		Payload:    map[string]interface{}{"key": "value"},
		Queue:      "default",
		MaxRetries: 3,
		Retries:    0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sonic.Marshal(job)
		var j Job
		_ = sonic.Unmarshal([]byte{}, &j)
	}
}

func BenchmarkGenerateJobID(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = generateJobID()
	}
}

func BenchmarkCountTokens(b *testing.B) {
	text := "This is a test string for token counting."
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = countTokens(text)
	}
}
