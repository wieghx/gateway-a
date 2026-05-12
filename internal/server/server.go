package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/gu/gateway-a/internal/handler"
	"github.com/gu/gateway-a/middleware/connectionpool"
	"github.com/gu/gateway-a/middleware/ratelimit"
	"github.com/gu/gateway-a/middleware/requestid"
	"github.com/gu/gateway-a/middleware/security"
)

type Server struct {
	hertz      *server.Hertz
	llmHandler *handler.LLMHandler
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

	return &Server{
		hertz: h,
		llmHandler: llmHandler,
	}
}

func (s *Server) Start() error {
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

	// Give time for existing requests to complete
	time.Sleep(1 * time.Second)

	if err := s.hertz.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	return nil
}
