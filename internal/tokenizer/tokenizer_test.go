package tokenizer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateTokens_Basic(t *testing.T) {
	text := "Hello world, this is a test for the tokenizer package."

	result := EstimateTokens(text, "gpt-4", DefaultConfig())
	assert.GreaterOrEqual(t, result, int64(5))

	assert.Equal(t, int64(0), EstimateTokens("", "gpt-4", DefaultConfig()))
	assert.Equal(t, int64(0), EstimateTokens("   ", "gpt-4", DefaultConfig()))
}

func TestEstimateTokens_CustomConfig(t *testing.T) {
	text := "Short text for custom config test."

	cfg := Config{
		ModelFactors: map[string]float64{
			"custom-model": 2.0,
		},
		DefaultFactor: 4.0,
	}

	result := EstimateTokens(text, "custom-model", cfg)
	assert.Greater(t, result, int64(0))

	result2 := EstimateTokens(text, "unknown-model", cfg)
	assert.Greater(t, result2, int64(0))
}

func TestEstimatePromptAndCompletionTokens(t *testing.T) {
	input := "User message"
	output := "This is a longer assistant reply with several sentences."

	p := EstimatePromptTokens(input, "claude-3", DefaultConfig())
	c := EstimateCompletionTokens(output, "claude-3", DefaultConfig())

	assert.Greater(t, p, int64(0))
	assert.Greater(t, c, int64(0))
}

func TestEstimateTokensRough(t *testing.T) {
	assert.Equal(t, int64(0), EstimateTokensRough(""))
	assert.Greater(t, EstimateTokensRough("hello world"), int64(0))
}

// Aggressive additional tests

func TestEstimateTokens_VariousLanguages(t *testing.T) {
	english := "This is a test sentence in English."
	chinese := "这是一个中文测试句子，用于验证分词估算。"
	code := "func main() { fmt.Println(\"hello\") }"

	for _, text := range []string{english, chinese, code} {
		result := EstimateTokens(text, "gpt-4", DefaultConfig())
		assert.Greater(t, result, int64(0), "should estimate tokens for: %s", text)
	}
}

func TestEstimateTokens_LongText(t *testing.T) {
	longText := strings.Repeat("This is a repeated sentence for testing long input. ", 50)

	result := EstimateTokens(longText, "gpt-4o", DefaultConfig())
	assert.Greater(t, result, int64(100))
}

func TestEstimateTokens_DifferentModels(t *testing.T) {
	text := "A sample text to compare model estimation differences."

	models := []string{"gpt-4", "gpt-3.5-turbo", "claude-3", "gemini", "unknown-model"}

	for _, model := range models {
		result := EstimateTokens(text, model, DefaultConfig())
		assert.GreaterOrEqual(t, result, int64(0))
	}
}

func TestConfig_EmptyFactors(t *testing.T) {
	cfg := Config{
		ModelFactors:  nil,
		DefaultFactor: 0,
	}

	result := EstimateTokens("test text", "any-model", cfg)
	assert.Greater(t, result, int64(0))
}

func TestEstimateTokens_ZeroAndNegative(t *testing.T) {
	assert.Equal(t, int64(0), EstimateTokens("", "gpt-4", DefaultConfig()))
	assert.Equal(t, int64(0), EstimateTokens("a", "gpt-4", Config{DefaultFactor: 0}))
}

// Additional aggressive test coverage
func TestEstimateTokens_LongAndComplex(t *testing.T) {
	longText := strings.Repeat("This is a complex test sentence with punctuation, numbers 123, and mixed content! ", 200)
	result := EstimateTokens(longText, "gpt-4o", DefaultConfig())
	assert.Greater(t, result, int64(500))
}

func TestEstimateTokens_ConsistencyAcrossCalls(t *testing.T) {
	text := "Repeated call consistency test."
	cfg := DefaultConfig()

	r1 := EstimateTokens(text, "gpt-4", cfg)
	r2 := EstimateTokens(text, "gpt-4", cfg)
	assert.Equal(t, r1, r2)
}
