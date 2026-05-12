// Package main provides examples for using the WASM plugin system.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gu/gateway-a/wasm"
)

func main() {
	ctx := context.Background()

	// Check for special arguments before setting pluginDir
	pluginDir := "./plugins"
	showGolangDemo := false

	if len(os.Args) > 1 {
		if os.Args[1] == "golang" {
			showGolangDemo = true
		} else {
			pluginDir = os.Args[1]
		}
	}

	if showGolangDemo {
		fmt.Println("\n--- Running Go-based plugin demo ---")
		golangPluginDemo()
		return
	}

	fmt.Printf("Creating plugin runtime in directory: %s\n", pluginDir)

	// Check if plugin directory exists
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		fmt.Println("Plugin directory does not exist. Creating plugins directory...")
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			fmt.Printf("Failed to create plugin directory: %v\n", err)
			return
		}
	}

	// Create the WASM runtime
	runtime, err := wasm.NewRuntime(ctx, pluginDir)
	if err != nil {
		fmt.Printf("Failed to create runtime: %v\n", err)
		return
	}
	defer runtime.Close()

	fmt.Println("WASM runtime created successfully!")

	// List available plugins
	plugins, err := runtime.ListPlugins()
	if err != nil {
		fmt.Printf("Failed to list plugins: %v\n", err)
	} else {
		fmt.Printf("\nFound %d plugins:\n", len(plugins))
		for _, p := range plugins {
			fmt.Printf("  - %s (v%s): %s\n", p.Name, p.Version, p.Description)
		}
	}

	// Create test input
	input := &wasm.PluginInput{
		Method:   "GET",
		URL:      "https://api.example.com/v1/users",
		Headers:  map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
		Body:     []byte(`{"limit": 10}`),
		TenantID: "tenant-123",
		APIKey:   "test-api-key",
		Context:  make(map[string]interface{}),
	}

	fmt.Printf("\nTest Input:\n")
	fmt.Printf("  Method: %s\n", input.Method)
	fmt.Printf("  URL: %s\n", input.URL)
	fmt.Printf("  TenantID: %s\n", input.TenantID)

	// Try to load and execute plugins (if any exist)
	pluginNames := []string{}
	if plugins, _ := runtime.ListPlugins(); len(plugins) > 0 {
		for _, p := range plugins {
			pluginNames = append(pluginNames, p.Name)
		}

		// Execute OnRequest chain
		fmt.Printf("\nExecuting OnRequest chain for plugins: %v\n", pluginNames)
		output, err := runtime.ExecuteChain(ctx, pluginNames, input)
		if err != nil {
			fmt.Printf("Chain execution error: %v\n", err)
		} else if output != nil {
			fmt.Printf("Chain output:\n")
			fmt.Printf("  Status: %d\n", output.Status)
			fmt.Printf("  Continue: %v\n", output.Continue)
			fmt.Printf("  Metadata: %v\n", output.Metadata)
		} else {
			fmt.Println("No plugins executed (no OnRequest hooks)")
		}
	}

	// Show SDK usage example
	fmt.Println("\n--- SDK Usage Example ---")
	fmt.Println("Use the Plugin struct to define custom plugin logic in Go:")
	fmt.Println("plugin := &wasm.Plugin{")
	fmt.Println("    OnRequest: func(input *wasm.PluginInput) (*wasm.PluginOutput, error) {")
	fmt.Println("        // Modify request headers")
	fmt.Println("        input.Headers[\"X-Plugin-Processed\"] = \"true\"")
	fmt.Println("        return &wasm.PluginOutput{")
	fmt.Println("            Status:   200,")
	fmt.Println("            Continue: true,")
	fmt.Println("        }, nil")
	fmt.Println("    },")
	fmt.Println("    OnResponse: func(output *wasm.PluginOutput) (*wasm.PluginOutput, error) {")
	fmt.Println("        // Modify response")
	fmt.Println("        output.Headers[\"X-Plugin-Added\"] = \"true\"")
	fmt.Println("        return output, nil")
	fmt.Println("    },")
	fmt.Println("    OnError: func(input *wasm.ErrorInput) (*wasm.ErrorOutput, error) {")
	fmt.Println("        // Custom error handling")
	fmt.Println("        return &wasm.ErrorOutput{")
	fmt.Println("            Status: 400,")
	fmt.Println("            Body:   []byte(\"Custom error message\"),")
	fmt.Println("        }, nil")
	fmt.Println("    },")
	fmt.Println("}")
}

// golangPluginDemo runs a demo of a Go-based plugin
func golangPluginDemo() {
	ctx := context.Background()

	// Create plugin runtime
	runtime, err := wasm.NewRuntime(ctx, "./plugins")
	if err != nil {
		fmt.Printf("Failed to create runtime: %v\n", err)
		return
	}
	defer runtime.Close()

	// Create and add the logging plugin
	plugin := wasm.NewPlugin()
	plugin.OnRequest = func(input *wasm.PluginInput) (*wasm.PluginOutput, error) {
		wasm.Log("info", "Request: %s %s", input.Method, input.URL)
		wasm.Log("info", "Tenant: %s", input.TenantID)

		return &wasm.PluginOutput{
			Status:   200,
			Headers:  input.Headers,
			Body:     input.Body,
			Metadata: make(map[string]string),
			Continue: true,
		}, nil
	}

	plugin.OnResponse = func(output *wasm.PluginOutput) (*wasm.PluginOutput, error) {
		wasm.Log("info", "Response status: %d", output.Status)
		wasm.Log("info", "Response size: %d bytes", len(output.Body))

		if output.Metadata == nil {
			output.Metadata = make(map[string]string)
		}
		output.Metadata["logged"] = "true"

		return output, nil
	}

	plugin.OnError = func(input *wasm.ErrorInput) (*wasm.ErrorOutput, error) {
		wasm.Log("error", "Error: %s - %s", input.ErrorCode, input.ErrorMessage)

		return &wasm.ErrorOutput{
			Status:  500,
			Body:    []byte("internal error"),
			Headers: map[string]string{"Content-Type": "text/plain"},
		}, nil
	}

	// Build a plugin chain
	chain := wasm.NewPluginChain().Add(plugin)

	// Execute OnRequest chain
	input := &wasm.PluginInput{
		Method:   "GET",
		URL:      "https://api.example.com/v1/users",
		Headers:  map[string]string{"Content-Type": "application/json"},
		Body:     []byte(`{"limit": 10}`),
		TenantID: "tenant-123",
		APIKey:   "test-api-key",
		Context:  make(map[string]interface{}),
	}

	output, err := chain.ExecuteOnRequest(ctx, input)
	if err != nil {
		fmt.Printf("OnRequest error: %v\n", err)
		return
	}

	if output != nil {
		fmt.Println("\nOnRequest output:")
		fmt.Printf("  Status: %d\n", output.Status)
		fmt.Printf("  Continue: %v\n", output.Continue)
		fmt.Printf("  Metadata: %v\n", output.Metadata)
	}

	// Execute OnResponse chain
	responseOutput := &wasm.PluginOutput{
		Status:   200,
		Headers:  map[string]string{"Content-Type": "application/json"},
		Body:     []byte(`{"users": []}`),
		Metadata: make(map[string]string),
	}

	response, err := chain.ExecuteOnResponse(responseOutput)
	if err != nil {
		fmt.Printf("OnResponse error: %v\n", err)
		return
	}

	if response != nil {
		fmt.Println("\nOnResponse output:")
		fmt.Printf("  Status: %d\n", response.Status)
		fmt.Printf("  Continue: %v\n", response.Continue)
		fmt.Printf("  Metadata: %v\n", response.Metadata)
	}

	// Test error handling
	errorInput := &wasm.ErrorInput{
		PluginInput:  input,
		ErrorCode:    "test_error",
		ErrorMessage: "test error message",
	}

	errorOutput, err := chain.ExecuteOnError(errorInput)
	if err != nil {
		fmt.Printf("OnError error: %v\n", err)
		return
	}

	if errorOutput != nil {
		fmt.Println("\nOnError output:")
		fmt.Printf("  Status: %d\n", errorOutput.Status)
		fmt.Printf("  Body: %s\n", string(errorOutput.Body))
	}

	fmt.Println("\nGo-based plugin demo completed successfully!")
}
