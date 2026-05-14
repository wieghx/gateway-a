package server

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/wieghx/gateway-a/internal/handler"
	"github.com/wieghx/gateway-a/internal/metrics"
	"github.com/wieghx/gateway-a/middleware/connectionpool"
	"github.com/wieghx/gateway-a/middleware/ratelimit"
	"github.com/wieghx/gateway-a/middleware/requestid"
	"github.com/wieghx/gateway-a/middleware/security"
	"github.com/wieghx/gateway-a/pkg/queue"
	"github.com/prometheus/client_golang/prometheus"
)

type Server struct {
	hertz      *server.Hertz
	llmHandler *handler.LLMHandler
	queue      *queue.TaskQueue
	worker     *queue.Worker
	metrics    *metrics.Metrics
}

func NewServer() *Server {
	h := server.Default(
		server.WithHostPorts(":8080"),
		server.WithReadTimeout(60*time.Second),
		server.WithWriteTimeout(60*time.Second),
	)

	// Create connection pool
	pool := connectionpool.NewTransportPool(connectionpool.DefaultPoolConfig())
	poolMiddleware := connectionpool.NewConnectionPoolMiddleware(pool)

	// Register global middleware
	h.Use(
		requestid.Middleware(nil),
		security.Security(security.DefaultSecurityHeaders()),
		security.CORS(security.DefaultCORSOptions()),
		ratelimit.Middleware(ratelimit.DefaultMiddlewareOptions()),
		poolMiddleware.Handle,
	)

	// Health check endpoint
	h.GET("/health", func(c context.Context, ctx *app.RequestContext) {
		ctx.JSON(consts.StatusOK, map[string]string{
			"status": "ok",
		})
	})

	// Root endpoint
	h.GET("/", func(c context.Context, ctx *app.RequestContext) {
		ctx.JSON(consts.StatusOK, map[string]string{
			"service": "gateway-a",
			"version": "0.1.0",
		})
	})

	// LLM API routes
	llmHandler := handler.NewLLMHandler()
	v1 := h.Group("/v1")
	{
		v1.POST("/chat/completions", llmHandler.ChatCompletion)
		v1.GET("/health", llmHandler.HealthCheck)
	}

	// Setup metrics
	m := metrics.NewMetrics()

	// Add metrics endpoint
	h.GET("/metrics", func(c context.Context, ctx *app.RequestContext) {
		// Gather metrics from default registry and write to response
		metrics, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			ctx.JSON(consts.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// Build metrics response manually
		var buf strings.Builder
		for _, mf := range metrics {
			buf.WriteString("# HELP ")
			buf.WriteString(mf.GetName())
			buf.WriteString(" ")
			buf.WriteString(mf.GetHelp())
			buf.WriteString("\n")
			buf.WriteString("# TYPE ")
			buf.WriteString(mf.GetName())
			buf.WriteString(" ")
			buf.WriteString(string(mf.GetType()))
			buf.WriteString("\n")

			for _, m := range mf.Metric {
				buf.WriteString("{")
				var labels []string
				for _, lp := range m.Label {
					labels = append(labels, fmt.Sprintf("%s=%q", lp.GetName(), lp.GetValue()))
				}
				if len(labels) > 0 {
					buf.WriteString(strings.Join(labels, ","))
					buf.WriteString("}")
				}

				var value string
				if m.Counter != nil {
					value = fmt.Sprintf("%g", m.Counter.GetValue())
				} else if m.Gauge != nil {
					value = fmt.Sprintf("%g", m.Gauge.GetValue())
				} else if m.Histogram != nil {
					value = fmt.Sprintf("%g", m.Histogram.GetSampleSum())
				}
				buf.WriteString(" ")
				buf.WriteString(value)
				buf.WriteString("\n")
			}
			buf.WriteString("\n")
		}

		ctx.Response.SetStatusCode(consts.StatusOK)
		ctx.Response.Header.SetContentType("text/plain; version=0.0.4")
		ctx.Response.SetBodyString(buf.String())
	})

	// Setup queue
	queueConfig := queue.QueueConfig{
		Host:         "localhost",
		Port:         "6379",
		DB:           0,
		DefaultQueue: "default",
		DefaultRetries: 3,
		DefaultTimeout: 30 * time.Second,
		PoolSize:     4,
	}

	var q *queue.TaskQueue
	var w *queue.Worker
	var err error

	if q, err = queue.NewTaskQueue(&queueConfig); err != nil {
		log.Printf("Failed to create task queue: %v", err)
	} else {
		// Create queue handler
		queueHandler := handler.NewQueueHandler(q)

		// Register queue routes
		v1.POST("/queue/enqueue", queueHandler.Enqueue)
		v1.GET("/queue/jobs/:queue_id", queueHandler.GetJobStatus)
		v1.GET("/queue/jobs", queueHandler.ListJobs)
		v1.GET("/queue/stats", queueHandler.GetWorkerStats)

		// Create and start worker pool
		w = queue.NewWorker(q, queueConfig.PoolSize)
		w.Register(queue.JobTypeTokenCalc, queue.TokenCalculationHandler(q))
		w.Register(queue.JobTypeUsageTracking, queue.UsageTrackingHandler(q))
		w.Register(queue.JobTypeLLMCaching, queue.LLMCachingHandler())
		w.Register(queue.JobTypeTenantSync, queue.TenantSyncHandler())
		w.Start()
	}

	return &Server{
		hertz:      h,
		llmHandler: llmHandler,
		queue:      q,
		worker:     w,
		metrics:    m,
	}
}

func (s *Server) Start() error {
	log.Println("Starting AI Gateway...")
	if err := s.hertz.Run(); err != nil {
		log.Printf("Hertz server error: %v", err)
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Stop worker pool
	if s.worker != nil {
		s.worker.Stop()
	}

	// Close queue connection
	if s.queue != nil {
		s.queue.Close()
	}

	// Close metrics
	if s.metrics != nil {
		// Metrics are registered globally, no cleanup needed
	}

	// Give time for existing requests to complete
	time.Sleep(1 * time.Second)

	if err := s.hertz.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	return nil
}

