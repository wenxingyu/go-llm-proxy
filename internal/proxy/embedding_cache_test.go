package proxy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractEmbeddingInputs(t *testing.T) {
	inputs, isArray, err := extractEmbeddingInputs("hello")
	require.NoError(t, err)
	require.False(t, isArray)
	require.Len(t, inputs, 1)
	require.Equal(t, "hello", inputs[0].Value)
}

func TestBuildMissInputPayload(t *testing.T) {
	misses := []embeddingInputMeta{
		{Index: 0, Value: "foo"},
		{Index: 1, Value: "bar"},
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
