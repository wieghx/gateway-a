package wasm_test

import (
	"context"
	"testing"

	"github.com/wieghx/gateway-a/wasm"
)

func TestPluginInput(t *testing.T) {
	input := &wasm.PluginInput{
		Method:   "GET",
		URL:      "https://api.example.com/users",
		Headers:  map[string]string{"Content-Type": "application/json"},
		Body:     []byte(`{"test": "data"}`),
		TenantID: "tenant-123",
		APIKey:   "test-key",
		Context:  make(map[string]interface{}),
	}

	if input.Method != "GET" {
		t.Errorf("Expected method GET, got %s", input.Method)
	}
	if input.URL != "https://api.example.com/users" {
		t.Errorf("Expected URL https://api.example.com/users, got %s", input.URL)
	}
	if input.TenantID != "tenant-123" {
		t.Errorf("Expected tenant-123, got %s", input.TenantID)
	}
}

func TestPluginOutput(t *testing.T) {
	output := &wasm.PluginOutput{
		Status:   200,
		Headers:  map[string]string{"Content-Type": "application/json"},
		Body:     []byte(`{"result": "success"}`),
		Metadata: make(map[string]string),
		Continue: true,
	}

	if output.Status != 200 {
		t.Errorf("Expected status 200, got %d", output.Status)
	}
	if output.Body == nil {
		t.Error("Expected non-nil body")
	}
}

func TestErrorInput(t *testing.T) {
	input := &wasm.ErrorInput{
		PluginInput:  &wasm.PluginInput{TenantID: "tenant-123"},
		ErrorCode:    "test_error",
		ErrorMessage: "test error message",
	}

	if input.ErrorCode != "test_error" {
		t.Errorf("Expected test_error, got %s", input.ErrorCode)
	}
}

func TestErrorOutput(t *testing.T) {
	output := &wasm.ErrorOutput{
		Status:  500,
		Body:    []byte("error message"),
		Headers: map[string]string{"Content-Type": "text/plain"},
	}

	if output.Status != 500 {
		t.Errorf("Expected status 500, got %d", output.Status)
	}
}

func TestPluginChain(t *testing.T) {
	chain := wasm.NewPluginChain()

	// Add a plugin that always continues
	chain.Add(&wasm.Plugin{
		OnRequest: func(input *wasm.PluginInput) (*wasm.PluginOutput, error) {
			return &wasm.PluginOutput{
				Status:   200,
				Continue: true,
			}, nil
		},
	})

	input := &wasm.PluginInput{
		Method:   "POST",
		URL:      "https://api.example.com/test",
		TenantID: "test-tenant",
	}

	ctx := context.Background()
	output, err := chain.ExecuteOnRequest(ctx, input)
	if err != nil {
		t.Fatalf("ExecuteOnRequest failed: %v", err)
	}

	if output == nil {
		t.Error("Expected non-nil output")
	}
	if !output.Continue {
		t.Error("Expected Continue to be true")
	}
}

func TestPluginChainWithEarlyReturn(t *testing.T) {
	chain := wasm.NewPluginChain()

	// Add a plugin that returns early
	chain.Add(&wasm.Plugin{
		OnRequest: func(input *wasm.PluginInput) (*wasm.PluginOutput, error) {
			return &wasm.PluginOutput{
				Status:   401,
				Continue: false,
				Body:     []byte("Unauthorized"),
			}, nil
		},
	})

	// Add a second plugin that should not be executed
	chain.Add(&wasm.Plugin{
		OnRequest: func(input *wasm.PluginInput) (*wasm.PluginOutput, error) {
			t.Error("Second plugin should not be executed")
			return nil, nil
		},
	})

	ctx := context.Background()
	output, err := chain.ExecuteOnRequest(ctx, &wasm.PluginInput{})
	if err != nil {
		t.Fatalf("ExecuteOnRequest failed: %v", err)
	}

	if output == nil {
		t.Error("Expected non-nil output")
	}
	if output.Status != 401 {
		t.Errorf("Expected status 401, got %d", output.Status)
	}
	if output.Continue {
		t.Error("Expected Continue to be false")
	}
}

func TestPluginOnResponse(t *testing.T) {
	chain := wasm.NewPluginChain()

	chain.Add(&wasm.Plugin{
		OnResponse: func(output *wasm.PluginOutput) (*wasm.PluginOutput, error) {
			if output.Metadata == nil {
				output.Metadata = make(map[string]string)
			}
			output.Metadata["processed"] = "true"
			return output, nil
		},
	})

	input := &wasm.PluginOutput{
		Status: 200,
		Metadata: make(map[string]string),
	}

	output, err := chain.ExecuteOnResponse(input)
	if err != nil {
		t.Fatalf("ExecuteOnResponse failed: %v", err)
	}

	if output.Metadata["processed"] != "true" {
		t.Error("Expected metadata to be set")
	}
}

func TestPluginOnError(t *testing.T) {
	chain := wasm.NewPluginChain()

	chain.Add(&wasm.Plugin{
		OnError: func(input *wasm.ErrorInput) (*wasm.ErrorOutput, error) {
			return &wasm.ErrorOutput{
				Status: 503,
				Body:   []byte("Service unavailable"),
			}, nil
		},
	})

	input := &wasm.ErrorInput{
		ErrorCode:    "test",
		ErrorMessage: "test error",
	}

	output, err := chain.ExecuteOnError(input)
	if err != nil {
		t.Fatalf("ExecuteOnError failed: %v", err)
	}

	if output == nil {
		t.Error("Expected non-nil output")
	}
	if output.Status != 503 {
		t.Errorf("Expected status 503, got %d", output.Status)
	}
}

func TestNewPlugin(t *testing.T) {
	plugin := wasm.NewPlugin()
	if plugin == nil {
		t.Error("Expected non-nil plugin")
	}
}