package wasm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

var (
	// ErrPluginNotFound is returned when a plugin is not found.
	ErrPluginNotFound = errors.New("plugin not found")

	// ErrPluginInvalid is returned when a plugin is invalid.
	ErrPluginInvalid = errors.New("invalid plugin")

	// ErrPluginMissingExport is returned when a required export is missing.
	ErrPluginMissingExport = errors.New("plugin missing required export")
)

// Runtime is the WASM runtime manager for loading and executing plugins.
type Runtime struct {
	// ctx is the context for WASM execution
	ctx context.Context

	// runtime is the wazero runtime instance
	runtime wazero.Runtime

	// plugins is the cache of loaded compiled modules
	plugins map[string]wazero.CompiledModule

	// mutex for thread-safe plugin access
	mu sync.RWMutex

	// pluginDir is the directory containing plugin .wasm files
	pluginDir string

	// metadata holds plugin metadata for each loaded plugin
	metadata map[string]*PluginMetadata
}

// NewRuntime creates a new WASM runtime.
func NewRuntime(ctx context.Context, pluginDir string) (*Runtime, error) {
	if pluginDir == "" {
		pluginDir = "./plugins"
	}

	// Check if plugin directory exists
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create plugin directory: %w", err)
	}

	r := &Runtime{
		ctx:       ctx,
		plugins:   make(map[string]wazero.CompiledModule),
		metadata:  make(map[string]*PluginMetadata),
		pluginDir: pluginDir,
	}

	// Create a new wazero runtime
	r.runtime = wazero.NewRuntime(ctx)

	// Enable WASI for proper memory management in plugins
	wasi_snapshot_preview1.MustInstantiate(ctx, r.runtime)

	return r, nil
}

// Close closes the runtime and releases resources.
func (r *Runtime) Close() {
	if r.runtime != nil {
		r.runtime.Close(r.ctx)
	}
}

// LoadPlugin loads a .wasm plugin from the specified path.
func (r *Runtime) LoadPlugin(name string) (*Plugin, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if already loaded
	if compiled, ok := r.plugins[name]; ok {
		return r.instantiatePlugin(compiled, name)
	}

	// Load the .wasm file
	path := filepath.Join(r.pluginDir, name+".wasm")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin file %s: %w", path, err)
	}

	// Compile the WASM module
	compiled, err := r.runtime.CompileModule(r.ctx, data)
	if err != nil {
		return nil, fmt.Errorf("failed to compile plugin %s: %w", name, err)
	}

	r.plugins[name] = compiled

	// Extract metadata
	r.extractMetadata(name, compiled)

	return r.instantiatePlugin(compiled, name)
}

// LoadPluginsFromDir loads all .wasm plugins from the plugin directory.
func (r *Runtime) LoadPluginsFromDir() (map[string]*Plugin, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	plugins := make(map[string]*Plugin)

	entries, err := os.ReadDir(r.pluginDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".wasm") {
			continue
		}

		// Remove .wasm extension for the plugin name
		baseName := strings.TrimSuffix(name, ".wasm")

		plugin, err := r.loadPluginLocked(baseName)
		if err != nil {
			// Log error but continue loading other plugins
			fmt.Printf("Failed to load plugin %s: %v\n", baseName, err)
			continue
		}

		plugins[baseName] = plugin
	}

	return plugins, nil
}

// loadPluginLocked loads a plugin (must be called with lock held).
func (r *Runtime) loadPluginLocked(name string) (*Plugin, error) {
	if compiled, ok := r.plugins[name]; ok {
		return r.instantiatePlugin(compiled, name)
	}

	path := filepath.Join(r.pluginDir, name+".wasm")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin file %s: %w", path, err)
	}

	compiled, err := r.runtime.CompileModule(r.ctx, data)
	if err != nil {
		return nil, fmt.Errorf("failed to compile plugin %s: %w", name, err)
	}

	r.plugins[name] = compiled
	r.extractMetadata(name, compiled)

	return r.instantiatePlugin(compiled, name)
}

// instantiatePlugin creates a new instance of a plugin.
func (r *Runtime) instantiatePlugin(compiled wazero.CompiledModule, name string) (*Plugin, error) {
	// Create module config with proper settings
	moduleConfig := wazero.NewModuleConfig().
		WithSysWalltime()

	// Instantiate the module
	m, err := r.runtime.InstantiateModule(r.ctx, compiled, moduleConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate plugin %s: %w", name, err)
	}
	// NOTE: Do NOT Close here. Lifetime managed by Runtime.
	// Previous defer m.Close caused all plugins to be immediately unusable.

	// Check for required exports
	var plugin *Plugin

	// Check for OnRequest export
	if fn := m.ExportedFunction("on_request"); fn != nil {
		plugin = &Plugin{}
		plugin.OnRequest = func(input *PluginInput) (*PluginOutput, error) {
			return r.callOnRequest(m, fn, input)
		}
	}

	// Check for OnResponse export
	if fn := m.ExportedFunction("on_response"); fn != nil {
		if plugin == nil {
			plugin = &Plugin{}
		}
		plugin.OnResponse = func(output *PluginOutput) (*PluginOutput, error) {
			return r.callOnResponse(m, fn, output)
		}
	}

	// Check for OnError export
	if fn := m.ExportedFunction("on_error"); fn != nil {
		if plugin == nil {
			plugin = &Plugin{}
		}
		plugin.OnError = func(input *ErrorInput) (*ErrorOutput, error) {
			return r.callOnError(m, fn, input)
		}
	}

	if plugin == nil {
		return nil, ErrPluginMissingExport
	}

	return plugin, nil
}

// GetMetadata returns the metadata for a plugin.
func (r *Runtime) GetMetadata(name string) (*PluginMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metadata, ok := r.metadata[name]
	if !ok {
		return nil, ErrPluginNotFound
	}

	return metadata, nil
}

// ListPlugins lists all available plugins.
func (r *Runtime) ListPlugins() ([]*PluginMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*PluginMetadata, 0, len(r.metadata))
	for _, metadata := range r.metadata {
		result = append(result, metadata)
	}

	return result, nil
}

// UnloadPlugin unloads a plugin.
func (r *Runtime) UnloadPlugin(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.plugins, name)
	delete(r.metadata, name)

	return nil
}

// CallOnRequest calls the OnRequest export of a plugin.
// NOTE: WASM memory passing is currently disabled for safety (known broken allocator).
// Plugins will not execute until a proper ABI (host functions for alloc + linear memory protocol) is implemented.
func (r *Runtime) callOnRequest(m api.Module, fn api.Function, input *PluginInput) (*PluginOutput, error) {
	return nil, errors.New("WASM plugin execution temporarily disabled for safety (memory protocol incomplete)")
}

// CallOnResponse calls the OnResponse export of a plugin.
// NOTE: WASM memory passing is currently disabled for safety (known broken allocator).
func (r *Runtime) callOnResponse(m api.Module, fn api.Function, output *PluginOutput) (*PluginOutput, error) {
	return nil, errors.New("WASM plugin execution temporarily disabled for safety (memory protocol incomplete)")
}

// CallOnError calls the OnError export of a plugin.
// NOTE: WASM memory passing is currently disabled for safety (known broken allocator).
func (r *Runtime) callOnError(m api.Module, fn api.Function, input *ErrorInput) (*ErrorOutput, error) {
	return nil, errors.New("WASM plugin execution temporarily disabled for safety (memory protocol incomplete)")
}

// extractMetadata extracts metadata from a plugin.
func (r *Runtime) extractMetadata(name string, compiled wazero.CompiledModule) {
	metadata := &PluginMetadata{
		Name:    name,
		Version: "unknown",
		Hooks:   make([]string, 0),
		Path:    filepath.Join(r.pluginDir, name+".wasm"),
	}

	// Check which hooks are available
	exportedFuncs := compiled.ExportedFunctions()
	for exp := range exportedFuncs {
		switch exp {
		case "on_request":
			metadata.Hooks = append(metadata.Hooks, "OnRequest")
		case "on_response":
			metadata.Hooks = append(metadata.Hooks, "OnResponse")
		case "on_error":
			metadata.Hooks = append(metadata.Hooks, "OnError")
		}
	}

	r.metadata[name] = metadata
}

// ExecuteChain executes a chain of plugins.
func (r *Runtime) ExecuteChain(ctx context.Context, pluginNames []string, input *PluginInput) (*PluginOutput, error) {
	chain := NewPluginChain()

	for _, name := range pluginNames {
		plugin, err := r.LoadPlugin(name)
		if err != nil {
			return nil, fmt.Errorf("failed to load plugin %s: %w", name, err)
		}
		chain.Add(plugin)
	}

	return chain.ExecuteOnRequest(ctx, input)
}

// Log is a helper function to log messages from plugins.
func Log(level string, format string, args ...interface{}) {
	fmt.Printf("[%s] "+format+"\n", append([]interface{}{level}, args...)...)
}

// ReadFile reads a file and returns its contents.
func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteFile writes data to a file.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}
