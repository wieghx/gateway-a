package main

import (
	"log"
	"os"

	"github.com/gu/gateway-a/internal/server"
)

func main() {
	srv := server.NewServer()

	log.Println("Starting AI Gateway...")

	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func getPort() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return "8080"
}
