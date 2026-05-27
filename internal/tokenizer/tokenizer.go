package tokenizer


// Tokenizer defines the interface for token counting / estimation.
// 
// This is the key abstraction that allows the rest of the system (especially
// streaming fallback and usage recording) to work with either the current
// heuristic implementation or a real tokenizer in the future (e.g. tiktoken-go,
// OpenAI's tokenizer, etc.) with zero changes to the calling code.
type Tokenizer interface {
	CountTokens(text string, model string) int64
}

// HeuristicTokenizer implements Tokenizer using the improved word+character
// heuristics with model-specific factors. This is the current production default.
type HeuristicTokenizer struct {
	cfg Config
}

// NewHeuristicTokenizer returns a ready-to-use heuristic tokenizer.
func NewHeuristicTokenizer(cfg Config) *HeuristicTokenizer {
	if cfg.ModelFactors == nil {
		cfg = DefaultConfig()
	}
	if cfg.DefaultFactor <= 0 {
		cfg.DefaultFactor = 3.8
	}
	return &HeuristicTokenizer{cfg: cfg}
}

func (h *HeuristicTokenizer) CountTokens(text string, model string) int64 {
	return EstimateTokens(text, model, h.cfg)
}

// Global default tokenizer instance used by the convenience functions.
var defaultTokenizer Tokenizer = NewHeuristicTokenizer(DefaultConfig())

// SetDefaultTokenizer allows replacing the global default tokenizer at runtime
// (useful for tests or when swapping in a real tokenizer).
func SetDefaultTokenizer(t Tokenizer) {
	if t != nil {
		defaultTokenizer = t
	}
}

// Config allows customizing the token estimation behavior.
type Config struct {
	// ModelFactors overrides the default per-model multipliers.
	// If nil or empty, defaults are used.
	ModelFactors map[string]float64

	// DefaultFactor is used when the model is not found.
	// If <= 0, a conservative default is applied.
	DefaultFactor float64
}

// DefaultConfig returns a reasonable default configuration.
func DefaultConfig() Config {
	return Config{
		ModelFactors: map[string]float64{
			"gpt-4":          3.8,
			"gpt-4o":         3.6,
			"gpt-3.5-turbo":  4.0,
			"claude-3":       3.7,
			"claude-3-5":     3.5,
			"gemini":         4.2,
		},
		DefaultFactor: 3.8,
	}
}

// EstimateTokens provides an improved client-side token estimate.
// It now delegates to the current default Tokenizer (which can be swapped).
func EstimateTokens(text string, model string, cfg Config) int64 {
	// If custom config is provided, use a one-off heuristic instance
	if cfg.ModelFactors != nil || cfg.DefaultFactor > 0 {
		return NewHeuristicTokenizer(cfg).CountTokens(text, model)
	}
	return defaultTokenizer.CountTokens(text, model)
}

// EstimatePromptTokens and EstimateCompletionTokens are convenience wrappers.
func EstimatePromptTokens(text string, model string, cfg Config) int64 {
	return EstimateTokens(text, model, cfg)
}

func EstimateCompletionTokens(text string, model string, cfg Config) int64 {
	return EstimateTokens(text, model, cfg)
}

// EstimateTokensRough is a very simple legacy estimator kept for compatibility.
func EstimateTokensRough(text string) int64 {
	if text == "" {
		return 0
	}
	return int64(float64(len(text)) / 2.8)
}
