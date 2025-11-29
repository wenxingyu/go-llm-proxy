package proxy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractEmbeddingInputs(t *testing.T) {
	inputs, isArray, err := extractEmbeddingInputs("hello")
	require.NoError(t, err)
	require.False(t, isArray)
	require.Len(t, inputs, 1)
	require.Equal(t, "hello", inputs[0].Normalized)

	rawArray := []interface{}{"foo", json.Number("42")}
	inputs, isArray, err = extractEmbeddingInputs(rawArray)
	require.NoError(t, err)
	require.True(t, isArray)
	require.Len(t, inputs, 2)
	require.Equal(t, "foo", inputs[0].Normalized)
	require.Equal(t, "42", inputs[1].Normalized)
}

func TestBuildMissInputPayload(t *testing.T) {
	misses := []embeddingInputMeta{
		{Index: 0, Raw: "foo"},
		{Index: 1, Raw: "bar"},
	}
	payload := buildMissInputPayload(misses[:1], false)
	require.Equal(t, "foo", payload)

	payload = buildMissInputPayload(misses, true)
	slice, ok := payload.([]interface{})
	require.True(t, ok)
	require.Len(t, slice, 2)
	require.Equal(t, "foo", slice[0])
	require.Equal(t, "bar", slice[1])
}
