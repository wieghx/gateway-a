package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

type Server struct {
	hertz      *server.Hertz
	startTime  time.Time
}

func NewServer() *Server {
	h := server.Default(
		server.WithHostPorts(":8080"),
		server.WithReadTimeout(60*time.Second),
		server.WithWriteTimeout(60*time.Second),
	)

	h.GET("/health", func(c context.Context, ctx *app.RequestContext) {
		ctx.JSON(consts.StatusOK, map[string]string{
			"status": "ok",
		})
	})

	h.GET("/", func(c context.Context, ctx *app.RequestContext) {
		ctx.JSON(consts.StatusOK, map[string]string{
			"service": "gateway-a",
			"version": "0.1.0",
		})
	})

	return &Server{
		hertz:     h,
		startTime: time.Now(),
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
