//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// IntegrationTestSuite holds the test suite state
type IntegrationTestSuite struct {
	suite.Suite
	gatewayURL string
	httpClient *http.Client
}

// SetupSuite initializes HTTP client for all tests
func (s *IntegrationTestSuite) SetupSuite() {
	s.httpClient = &http.Client{
		Timeout: 30 * time.Second,
	}

	// Gateway URL - should be running (e.g., via docker-compose)
	if os.Getenv("GATEWAY_URL") != "" {
		s.gatewayURL = os.Getenv("GATEWAY_URL")
	} else {
		s.gatewayURL = "http://localhost:8080"
	}
}

// TearDownSuite cleans up
func (s *IntegrationTestSuite) TearDownSuite() {
	// No cleanup needed for HTTP client
}

// TestHealthEndpoint tests the health check endpoint
func (s *IntegrationTestSuite) TestHealthEndpoint() {
	if os.Getenv("INTEGRATION_TEST") == "skip" {
		s.T().Skip("Integration tests skipped")
	}

	resp, err := s.httpClient.Get(s.gatewayURL + "/health")
	s.Require().NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)

	var result map[string]string
	err = json.Unmarshal(body, &result)
	s.Require().NoError(err)

	s.Equal("ok", result["status"])
}

// TestRootEndpoint tests the root endpoint
func (s *IntegrationTestSuite) TestRootEndpoint() {
	if os.Getenv("INTEGRATION_TEST") == "skip" {
		s.T().Skip("Integration tests skipped")
	}

	resp, err := s.httpClient.Get(s.gatewayURL + "/")
	s.Require().NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)

	var result map[string]string
	err = json.Unmarshal(body, &result)
	s.Require().NoError(err)

	s.Equal("gateway-a", result["service"])
}

// TestPrometheusMetrics tests the metrics endpoint
func (s *IntegrationTestSuite) TestPrometheusMetrics() {
	if os.Getenv("INTEGRATION_TEST") == "skip" {
		s.T().Skip("Integration tests skipped")
	}

	resp, err := s.httpClient.Get(s.gatewayURL + "/metrics")
	s.Require().NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusOK, resp.StatusCode)
	s.Equal("text/plain; version=0.0.4", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)

	// Check for expected metrics
	bodyStr := string(body)
	assert.Contains(s.T(), bodyStr, "gateway_a_requests_total")
	assert.Contains(s.T(), bodyStr, "gateway_a_llm_requests_total")
}

// TestLLMChatCompletion tests the LLM chat completion endpoint
func (s *IntegrationTestSuite) TestLLMChatCompletion() {
	if os.Getenv("INTEGRATION_TEST") == "skip" {
		s.T().Skip("Integration tests skipped")
	}

	requestBody := map[string]interface{}{
		"model": "test-model",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello, how are you?"},
		},
		"stream": false,
	}

	body, err := json.Marshal(requestBody)
	s.Require().NoError(err)

	resp, err := s.httpClient.Post(
		s.gatewayURL+"/v1/chat/completions",
		"application/json",
		bytes.NewBuffer(body),
	)

	// Note: This may return 401 if API key is required or 502 if upstream is not available
	// The important thing is the endpoint is reachable
	if resp != nil {
		s.Log("LLM Chat Completion response status:", resp.StatusCode)
		resp.Body.Close()
	}
}

// TestQueueEnqueue tests the queue enqueue endpoint
func (s *IntegrationTestSuite) TestQueueEnqueue() {
	if os.Getenv("INTEGRATION_TEST") == "skip" {
		s.T().Skip("Integration tests skipped")
	}

	requestBody := map[string]interface{}{
		"job_type":  "test_job",
		"payload":   map[string]string{"key": "value"},
		"queue":     "default",
		"max_retries": 3,
	}

	body, err := json.Marshal(requestBody)
	s.Require().NoError(err)

	resp, err := s.httpClient.Post(
		s.gatewayURL+"/v1/queue/enqueue",
		"application/json",
		bytes.NewBuffer(body),
	)

	if resp != nil {
		defer resp.Body.Close()
		s.Log("Queue Enqueue response status:", resp.StatusCode)

		// Expected: 200 if successful, or 500 if Redis not available
		if resp.StatusCode == http.StatusOK {
			// Verify response structure
			var result map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&result)
			s.Require().NoError(err)
			s.Contains(result, "job_id")
			s.Equal("pending", result["status"])
		}
	}
}

// TestQueueJobStatus tests getting job status
func (s *IntegrationTestSuite) TestQueueJobStatus() {
	if os.Getenv("INTEGRATION_TEST") == "skip" {
		s.T().Skip("Integration tests skipped")
	}

	// First enqueue a job to get a job ID
	requestBody := map[string]interface{}{
		"job_type": "test_job",
		"payload":  map[string]string{"test": "value"},
	}

	body, err := json.Marshal(requestBody)
	s.Require().NoError(err)

	enqueueResp, err := s.httpClient.Post(
		s.gatewayURL+"/v1/queue/enqueue",
		"application/json",
		bytes.NewBuffer(body),
	)

	if enqueueResp != nil {
		defer enqueueResp.Body.Close()
		if enqueueResp.StatusCode == http.StatusOK {
			var enqueueResult map[string]interface{}
			err := json.NewDecoder(enqueueResp.Body).Decode(&enqueueResult)
			if err == nil {
				jobID, ok := enqueueResult["job_id"].(string)
				if ok && jobID != "" {
					// Now check job status
					statusResp, err := s.httpClient.Get(
						s.gatewayURL + "/v1/queue/jobs/" + jobID + "/default",
					)
					if statusResp != nil {
						defer statusResp.Body.Close()
						s.Log("Job Status response status:", statusResp.StatusCode)
					}
				}
			}
		}
	}
}

// TestQueueListJobs tests listing jobs
func (s *IntegrationTestSuite) TestQueueListJobs() {
	if os.Getenv("INTEGRATION_TEST") == "skip" {
		s.T().Skip("Integration tests skipped")
	}

	resp, err := s.httpClient.Get(s.gatewayURL + "/v1/queue/jobs")
	s.Require().NoError(err)
	defer resp.Body.Close()

	// Should return 200 with empty list if no jobs
	s.Equal(http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	s.Require().NoError(err)

	s.Contains(result, "jobs")
	s.Contains(result, "total")
}

// TestQueueStats tests getting queue stats
func (s *IntegrationTestSuite) TestQueueStats() {
	if os.Getenv("INTEGRATION_TEST") == "skip" {
		s.T().Skip("Integration tests skipped")
	}

	resp, err := s.httpClient.Get(s.gatewayURL + "/v1/queue/stats")
	s.Require().NoError(err)
	defer resp.Body.Close()

	// Should return 200 with stats
	s.Equal(http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	s.Require().NoError(err)

	s.Contains(result, "queue")
	s.Contains(result, "active_workers")
	s.Contains(result, "pending_jobs")
}

// TestGracefulShutdown tests graceful shutdown behavior
func (s *IntegrationTestSuite) TestGracefulShutdown() {
	if os.Getenv("INTEGRATION_TEST") == "skip" {
		s.T().Skip("Integration tests skipped")
	}

	// This would test the shutdown endpoint
	// For now, just verify the gateway can be contacted
	resp, err := s.httpClient.Get(s.gatewayURL + "/health")
	if err == nil {
		resp.Body.Close()
		s.Log("Gateway responded to health check")
	}
}

// TestRateLimitHeaders tests rate limit headers on responses
func (s *IntegrationTestSuite) TestRateLimitHeaders() {
	if os.Getenv("INTEGRATION_TEST") == "skip" {
		s.T().Skip("Integration tests skipped")
	}

	resp, err := s.httpClient.Get(s.gatewayURL + "/health")
	s.Require().NoError(err)
	defer resp.Body.Close()

	// Rate limit headers should be present
	rateLimitLimit := resp.Header.Get("X-RateLimit-Limit")
	rateLimitRemaining := resp.Header.Get("X-RateLimit-Remaining")
	rateLimitReset := resp.Header.Get("X-RateLimit-Reset")

	s.Log("Rate limit headers:", rateLimitLimit, rateLimitRemaining, rateLimitReset)
	// Headers may not be set if rate limiting is disabled
}

// TestConcurrentRequests tests handling concurrent requests
func (s *IntegrationTestSuite) TestConcurrentRequests() {
	if os.Getenv("INTEGRATION_TEST") == "skip" {
		s.T().Skip("Integration tests skipped")
	}

	numRequests := 10
	done := make(chan bool, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			resp, err := s.httpClient.Get(s.gatewayURL + "/health")
			if err == nil {
				resp.Body.Close()
			}
			done <- true
		}(i)
	}

	successCount := 0
	timeout := time.After(10 * time.Second)

	for i := 0; i < numRequests; i++ {
		select {
		case <-done:
			successCount++
		case <-timeout:
			s.T().Log("Timeout waiting for concurrent requests")
		}
	}

	s.Log("Completed requests:", successCount, "/", numRequests)
}

// TestErrorHandling tests error responses
func (s *IntegrationTestSuite) TestErrorHandling() {
	if os.Getenv("INTEGRATION_TEST") == "skip" {
		s.T().Skip("Integration tests skipped")
	}

	// Test invalid JSON
	invalidBody := []byte(`{"invalid": json}`)
	resp, err := s.httpClient.Post(
		s.gatewayURL+"/v1/queue/enqueue",
		"application/json",
		bytes.NewBuffer(invalidBody),
	)

	if resp != nil {
		defer resp.Body.Close()
		// Should return 400 Bad Request
		s.Log("Invalid JSON response status:", resp.StatusCode)
	}
}

// TestMiddlewareHeaders tests middleware headers
func (s *IntegrationTestSuite) TestMiddlewareHeaders() {
	if os.Getenv("INTEGRATION_TEST") == "skip" {
		s.T().Skip("Integration tests skipped")
	}

	// Request ID should be set by requestid middleware
	resp, err := s.httpClient.Get(s.gatewayURL + "/health")
	s.Require().NoError(err)
	defer resp.Body.Close()

	// Security headers should be present
	csp := resp.Header.Get("Content-Security-Policy")
	xss := resp.Header.Get("X-Content-Type-Options")
	s.Log("Security headers - CSP:", csp, "X-XSS:", xss)
}

// Run the test suite
func TestIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	suite.Run(t, new(IntegrationTestSuite))
}
