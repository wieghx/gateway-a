package server

import (
	"context"
	"log"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

type Server struct {
	hertz *server.Hertz
}

func NewServer() *Server {
	h := server.Default(server.WithHostPorts(":8080"))

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
		hertz: h,
	}
}

func (s *Server) Start() error {
	if err := s.hertz.Run(); err != nil {
		log.Printf("Hertz server error: %v", err)
		return err
	}
	return nil
}
