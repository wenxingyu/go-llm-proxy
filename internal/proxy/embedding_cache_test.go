package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-llm-server/internal/config"
	"go-llm-server/pkg/db"

	"github.com/stretchr/testify/require"
)

type fakeCacheStorage struct {
	getEmbeddingFn    func(ctx context.Context, inputText, modelName string, dimensions *int) (*db.EmbeddingRecord, error)
	upsertEmbeddingFn func(ctx context.Context, rec *db.EmbeddingRecord) error
}

func (f *fakeCacheStorage) GetEmbedding(ctx context.Context, inputText, modelName string, dimensions *int) (*db.EmbeddingRecord, error) {
	if f.getEmbeddingFn != nil {
		return f.getEmbeddingFn(ctx, inputText, modelName, dimensions)
	}
	return nil, nil
}

func (f *fakeCacheStorage) UpsertEmbedding(ctx context.Context, rec *db.EmbeddingRecord) error {
	if f.upsertEmbeddingFn != nil {
		return f.upsertEmbeddingFn(ctx, rec)
	}
	return nil
}

// LLM cache methods are unused in these tests.
func (f *fakeCacheStorage) GetLLM(context.Context, string, string) (*db.LLMRecord, error) {
	return nil, nil
}
func (f *fakeCacheStorage) UpsertLLM(context.Context, *db.LLMRecord) error {
	return nil
}

func newTestHandlerWithStorage(storage cacheStorage) *Handler {
	cfg := &config.Config{
		TargetMap: map[string]string{
			"/v1/embeddings": "https://api.example.com/v1",
		},
	}
	return &Handler{
		cfg:       cfg,
		lbManager: NewLoadBalancerManager(),
		storage:   storage,
	}
}

func TestHandleEmbeddingCachePreProxy_Hit(t *testing.T) {
	rec := &db.EmbeddingRecord{Embedding: []float64{0.1, 0.2}}
	storage := &fakeCacheStorage{
		getEmbeddingFn: func(ctx context.Context, inputText, modelName string, dimensions *int) (*db.EmbeddingRecord, error) {
			require.Equal(t, "hello", inputText)
			return rec, nil
		},
	}
	handler := newTestHandlerWithStorage(storage)

	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"text-embedding","input":"hello"}`))
	resp := httptest.NewRecorder()

	handled, meta := handler.handleEmbeddingCachePreProxy(resp, req)
	require.True(t, handled)
	require.Nil(t, meta)
	require.Equal(t, "HIT", resp.Header().Get("X-Embedding-Cache"))

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	data := payload["data"].([]interface{})
	require.Len(t, data, 1)
}

// TestHandleEmbeddingCachePreProxy_AllHit tests when all inputs are cached (multiple inputs)
func TestHandleEmbeddingCachePreProxy_AllHit(t *testing.T) {
	storage := &fakeCacheStorage{
		getEmbeddingFn: func(ctx context.Context, inputText, modelName string, dimensions *int) (*db.EmbeddingRecord, error) {
			// Return different embeddings for different inputs
			switch inputText {
			case "hello":
				return &db.EmbeddingRecord{Embedding: []float64{0.1, 0.2}, TokenCount: intPtr(5)}, nil
			case "world":
				return &db.EmbeddingRecord{Embedding: []float64{0.3, 0.4}, TokenCount: intPtr(6)}, nil
			case "test":
				return &db.EmbeddingRecord{Embedding: []float64{0.5, 0.6}, TokenCount: intPtr(4)}, nil
			default:
				return nil, nil
			}
		},
	}
	handler := newTestHandlerWithStorage(storage)

	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"text-embedding","input":["hello","world","test"]}`))
	resp := httptest.NewRecorder()

	handled, meta := handler.handleEmbeddingCachePreProxy(resp, req)
	require.True(t, handled)
	require.Nil(t, meta)
	require.Equal(t, "HIT", resp.Header().Get("X-Embedding-Cache"))

	var payload embeddingAPIResponse
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	require.Len(t, payload.Data, 3)
	require.Equal(t, 0, payload.Data[0].Index)
	require.Equal(t, []float64{0.1, 0.2}, payload.Data[0].Embedding)
	require.Equal(t, 1, payload.Data[1].Index)
	require.Equal(t, []float64{0.3, 0.4}, payload.Data[1].Embedding)
	require.Equal(t, 2, payload.Data[2].Index)
	require.Equal(t, []float64{0.5, 0.6}, payload.Data[2].Embedding)
	require.NotNil(t, payload.Usage)
	require.Equal(t, 15, payload.Usage.TotalTokens) // 5 + 6 + 4
}

// TestHandleEmbeddingCachePreProxy_AllMiss tests when no inputs are cached
func TestHandleEmbeddingCachePreProxy_AllMiss(t *testing.T) {
	storage := &fakeCacheStorage{
		getEmbeddingFn: func(ctx context.Context, inputText, modelName string, dimensions *int) (*db.EmbeddingRecord, error) {
			// All misses - return nil for all
			return nil, nil
		},
	}
	handler := newTestHandlerWithStorage(storage)

	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"text-embedding","input":["foo","bar","baz"]}`))
	resp := httptest.NewRecorder()

	handled, meta := handler.handleEmbeddingCachePreProxy(resp, req)
	require.False(t, handled)
	require.NotNil(t, meta)
	require.Equal(t, 3, meta.total)
	require.Equal(t, 0, len(meta.hits))
	require.Equal(t, 3, len(meta.misses))
	require.Equal(t, "text-embedding", meta.model)

	// Request body should contain all inputs (since all are misses)
	bodyBytes, _ := io.ReadAll(req.Body)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(bodyBytes, &payload))
	inputArray := payload["input"].([]interface{})
	require.Len(t, inputArray, 3)
	require.Equal(t, "foo", inputArray[0])
	require.Equal(t, "bar", inputArray[1])
	require.Equal(t, "baz", inputArray[2])
}

// TestHandleEmbeddingCachePreProxy_AllMiss_SingleInput tests when single input is not cached
func TestHandleEmbeddingCachePreProxy_AllMiss_SingleInput(t *testing.T) {
	storage := &fakeCacheStorage{
		getEmbeddingFn: func(ctx context.Context, inputText, modelName string, dimensions *int) (*db.EmbeddingRecord, error) {
			return nil, nil
		},
	}
	handler := newTestHandlerWithStorage(storage)

	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"text-embedding","input":"new-text"}`))
	resp := httptest.NewRecorder()

	handled, meta := handler.handleEmbeddingCachePreProxy(resp, req)
	require.False(t, handled)
	require.NotNil(t, meta)
	require.Equal(t, 1, meta.total)
	require.Equal(t, 0, len(meta.hits))
	require.Equal(t, 1, len(meta.misses))
	require.Equal(t, "new-text", meta.misses[0].Value)

	// Request body should contain the single input as string (since original was not array)
	bodyBytes, _ := io.ReadAll(req.Body)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(bodyBytes, &payload))
	require.Equal(t, "new-text", payload["input"])
}

func TestHandleEmbeddingCachePreProxy_Partial(t *testing.T) {
	callCount := 0
	storage := &fakeCacheStorage{
		getEmbeddingFn: func(ctx context.Context, inputText, modelName string, dimensions *int) (*db.EmbeddingRecord, error) {
			defer func() { callCount++ }()
			if callCount == 0 {
				return &db.EmbeddingRecord{Embedding: []float64{0.1, 0.2}}, nil
			}
			return nil, nil
		},
	}
	handler := newTestHandlerWithStorage(storage)

	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"text-embedding","input":["foo","bar"]}`))
	resp := httptest.NewRecorder()

	handled, meta := handler.handleEmbeddingCachePreProxy(resp, req)
	require.False(t, handled)
	require.NotNil(t, meta)
	require.Equal(t, 2, meta.total)
	require.Equal(t, 1, len(meta.hits))
	require.Equal(t, 1, len(meta.misses))

	bodyBytes, _ := io.ReadAll(req.Body)
	require.JSONEq(t, `{"model":"text-embedding","input":["bar"]}`, string(bodyBytes))
}

func TestHandleEmbeddingCachePreProxy_BypassOnError(t *testing.T) {
	storage := &fakeCacheStorage{
		getEmbeddingFn: func(ctx context.Context, inputText, modelName string, dimensions *int) (*db.EmbeddingRecord, error) {
			return nil, fmt.Errorf("boom")
		},
	}
	handler := newTestHandlerWithStorage(storage)

	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"text-embedding","input":"hello"}`))
	resp := httptest.NewRecorder()

	handled, meta := handler.handleEmbeddingCachePreProxy(resp, req)
	require.False(t, handled)
	require.Nil(t, meta)
	require.Equal(t, "BYPASS", resp.Header().Get("X-Embedding-Cache"))
}

func TestHandleEmbeddingCachePostResponse_Partial(t *testing.T) {
	var persisted []*db.EmbeddingRecord
	storage := &fakeCacheStorage{
		upsertEmbeddingFn: func(ctx context.Context, rec *db.EmbeddingRecord) error {
			persisted = append(persisted, rec)
			return nil
		},
	}
	handler := newTestHandlerWithStorage(storage)

	meta := &embeddingCacheMetadata{
		model: "text-embedding",
		total: 2,
		hits: map[int]*db.EmbeddingRecord{
			0: {Embedding: []float64{0.1, 0.2}},
		},
		misses: []embeddingInputMeta{
			{Index: 1, Value: "bar"},
		},
		startTime: time.Now(),
		requestID: "req-123",
	}

	upstream := `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.3,0.4]}],"model":"text-embedding","usage":{"total_tokens":10}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(upstream))),
		Header:     make(http.Header),
		Request:    httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil).WithContext(context.Background()),
	}
	ctx := context.WithValue(resp.Request.Context(), embeddingCacheContextKey, meta)
	resp.Request = resp.Request.WithContext(ctx)

	require.NoError(t, handler.handleEmbeddingCachePostResponse(resp, meta))
	require.Equal(t, "PARTIAL", resp.Header.Get("X-Embedding-Cache"))
	require.Len(t, persisted, 1)

	bodyBytes, _ := io.ReadAll(resp.Body)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(bodyBytes, &payload))
	data := payload["data"].([]interface{})
	require.Len(t, data, 2)
}

// TestHandleEmbeddingCachePostResponse_AllMiss tests when all inputs were misses
func TestHandleEmbeddingCachePostResponse_AllMiss(t *testing.T) {
	var persisted []*db.EmbeddingRecord
	storage := &fakeCacheStorage{
		upsertEmbeddingFn: func(ctx context.Context, rec *db.EmbeddingRecord) error {
			persisted = append(persisted, rec)
			return nil
		},
	}
	handler := newTestHandlerWithStorage(storage)

	meta := &embeddingCacheMetadata{
		model:     "text-embedding",
		total:     3,
		hits:      make(map[int]*db.EmbeddingRecord), // Empty hits
		misses:    []embeddingInputMeta{{Index: 0, Value: "foo"}, {Index: 1, Value: "bar"}, {Index: 2, Value: "baz"}},
		startTime: time.Now(),
		requestID: "req-456",
	}

	upstream := `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]},{"object":"embedding","index":1,"embedding":[0.3,0.4]},{"object":"embedding","index":2,"embedding":[0.5,0.6]}],"model":"text-embedding","usage":{"total_tokens":15}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(upstream))),
		Header:     make(http.Header),
		Request:    httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil).WithContext(context.Background()),
	}

	require.NoError(t, handler.handleEmbeddingCachePostResponse(resp, meta))
	require.Equal(t, "MISS", resp.Header.Get("X-Embedding-Cache"))
	require.Len(t, persisted, 3) // All 3 should be persisted

	bodyBytes, _ := io.ReadAll(resp.Body)
	var payload embeddingAPIResponse
	require.NoError(t, json.Unmarshal(bodyBytes, &payload))
	require.Len(t, payload.Data, 3)
	require.Equal(t, 0, payload.Data[0].Index)
	require.Equal(t, []float64{0.1, 0.2}, payload.Data[0].Embedding)
	require.Equal(t, 1, payload.Data[1].Index)
	require.Equal(t, []float64{0.3, 0.4}, payload.Data[1].Embedding)
	require.Equal(t, 2, payload.Data[2].Index)
	require.Equal(t, []float64{0.5, 0.6}, payload.Data[2].Embedding)
}

// TestHandleEmbeddingCachePostResponse_AllMiss_SingleInput tests when single input was a miss
func TestHandleEmbeddingCachePostResponse_AllMiss_SingleInput(t *testing.T) {
	var persisted []*db.EmbeddingRecord
	storage := &fakeCacheStorage{
		upsertEmbeddingFn: func(ctx context.Context, rec *db.EmbeddingRecord) error {
			persisted = append(persisted, rec)
			return nil
		},
	}
	handler := newTestHandlerWithStorage(storage)

	meta := &embeddingCacheMetadata{
		model:     "text-embedding",
		total:     1,
		hits:      make(map[int]*db.EmbeddingRecord), // Empty hits
		misses:    []embeddingInputMeta{{Index: 0, Value: "new-text"}},
		startTime: time.Now(),
		requestID: "req-789",
	}

	upstream := `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.7,0.8]}],"model":"text-embedding","usage":{"total_tokens":5}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(upstream))),
		Header:     make(http.Header),
		Request:    httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil).WithContext(context.Background()),
	}

	require.NoError(t, handler.handleEmbeddingCachePostResponse(resp, meta))
	require.Equal(t, "MISS", resp.Header.Get("X-Embedding-Cache"))
	require.Len(t, persisted, 1)

	bodyBytes, _ := io.ReadAll(resp.Body)
	var payload embeddingAPIResponse
	require.NoError(t, json.Unmarshal(bodyBytes, &payload))
	require.Len(t, payload.Data, 1)
	require.Equal(t, 0, payload.Data[0].Index)
	require.Equal(t, []float64{0.7, 0.8}, payload.Data[0].Embedding)
	require.NotNil(t, payload.Usage)
	require.Equal(t, 5, payload.Usage.TotalTokens)
}

// TestHandleEmbeddingCachePostResponse_AllHit tests edge case when all were hits (shouldn't normally happen, but tests the logic)
func TestHandleEmbeddingCachePostResponse_AllHit(t *testing.T) {
	storage := &fakeCacheStorage{
		upsertEmbeddingFn: func(ctx context.Context, rec *db.EmbeddingRecord) error {
			return nil
		},
	}
	handler := newTestHandlerWithStorage(storage)

	tokenCount1 := 5
	tokenCount2 := 6
	meta := &embeddingCacheMetadata{
		model: "text-embedding",
		total: 2,
		hits: map[int]*db.EmbeddingRecord{
			0: {Embedding: []float64{0.1, 0.2}, TokenCount: &tokenCount1},
			1: {Embedding: []float64{0.3, 0.4}, TokenCount: &tokenCount2},
		},
		misses:    []embeddingInputMeta{}, // Empty misses
		startTime: time.Now(),
		requestID: "req-999",
	}

	// Even though all were hits, if we somehow get here, upstream should return empty or minimal response
	upstream := `{"object":"list","data":[],"model":"text-embedding","usage":{"total_tokens":0}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(upstream))),
		Header:     make(http.Header),
		Request:    httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil).WithContext(context.Background()),
	}

	require.NoError(t, handler.handleEmbeddingCachePostResponse(resp, meta))
	// When hits > 0 and newRecords == 0, status should be HIT
	require.Equal(t, "HIT", resp.Header.Get("X-Embedding-Cache"))

	bodyBytes, _ := io.ReadAll(resp.Body)
	var payload embeddingAPIResponse
	require.NoError(t, json.Unmarshal(bodyBytes, &payload))
	require.Len(t, payload.Data, 2) // Should have both hits
	require.Equal(t, 0, payload.Data[0].Index)
	require.Equal(t, []float64{0.1, 0.2}, payload.Data[0].Embedding)
	require.Equal(t, 1, payload.Data[1].Index)
	require.Equal(t, []float64{0.3, 0.4}, payload.Data[1].Embedding)
	require.NotNil(t, payload.Usage)
	require.Equal(t, 11, payload.Usage.TotalTokens) // 5 + 6
}

// Helper function to create int pointer
func intPtr(i int) *int {
	return &i
}

// ========== Unit Tests for Helper Functions ==========

// TestExtractEmbeddingInputs tests extractEmbeddingInputs with various input types
func TestExtractEmbeddingInputs(t *testing.T) {
	t.Run("single string", func(t *testing.T) {
		inputs, isArray, err := extractEmbeddingInputs("hello")
		require.NoError(t, err)
		require.False(t, isArray)
		require.Len(t, inputs, 1)
		require.Equal(t, 0, inputs[0].Index)
		require.Equal(t, "hello", inputs[0].Value)
	})

	t.Run("string slice", func(t *testing.T) {
		inputs, isArray, err := extractEmbeddingInputs([]string{"foo", "bar", "baz"})
		require.NoError(t, err)
		require.True(t, isArray)
		require.Len(t, inputs, 3)
		require.Equal(t, 0, inputs[0].Index)
		require.Equal(t, "foo", inputs[0].Value)
		require.Equal(t, 1, inputs[1].Index)
		require.Equal(t, "bar", inputs[1].Value)
		require.Equal(t, 2, inputs[2].Index)
		require.Equal(t, "baz", inputs[2].Value)
	})

	t.Run("interface slice from JSON", func(t *testing.T) {
		// Simulate JSON unmarshal result
		var raw interface{}
		jsonStr := `["test1", "test2"]`
		require.NoError(t, json.Unmarshal([]byte(jsonStr), &raw))
		inputs, isArray, err := extractEmbeddingInputs(raw)
		require.NoError(t, err)
		require.True(t, isArray)
		require.Len(t, inputs, 2)
		require.Equal(t, "test1", inputs[0].Value)
		require.Equal(t, "test2", inputs[1].Value)
	})

	t.Run("empty string slice", func(t *testing.T) {
		inputs, isArray, err := extractEmbeddingInputs([]string{})
		require.NoError(t, err)
		require.True(t, isArray)
		require.Len(t, inputs, 0)
	})

	t.Run("invalid type - number", func(t *testing.T) {
		inputs, isArray, err := extractEmbeddingInputs(123)
		require.Error(t, err)
		require.False(t, isArray)
		require.Nil(t, inputs)
		require.Contains(t, err.Error(), "only string or []string")
	})

	t.Run("invalid type - map", func(t *testing.T) {
		inputs, isArray, err := extractEmbeddingInputs(map[string]string{"key": "value"})
		require.Error(t, err)
		require.False(t, isArray)
		require.Nil(t, inputs)
		require.Contains(t, err.Error(), "only string or []string")
	})

	t.Run("invalid interface slice - non-string element", func(t *testing.T) {
		var raw interface{}
		jsonStr := `["test", 123]`
		require.NoError(t, json.Unmarshal([]byte(jsonStr), &raw))
		inputs, isArray, err := extractEmbeddingInputs(raw)
		require.Error(t, err)
		require.True(t, isArray)
		require.Nil(t, inputs)
		require.Contains(t, err.Error(), "only string or []string")
	})
}

// TestBuildMissInputPayload tests buildMissInputPayload with various scenarios
func TestBuildMissInputPayload(t *testing.T) {
	t.Run("single miss, original not array", func(t *testing.T) {
		misses := []embeddingInputMeta{
			{Index: 0, Value: "foo"},
		}
		payload := buildMissInputPayload(misses, false)
		require.Equal(t, "foo", payload)
	})

	t.Run("single miss, original was array", func(t *testing.T) {
		misses := []embeddingInputMeta{
			{Index: 0, Value: "foo"},
		}
		payload := buildMissInputPayload(misses, true)
		slice, ok := payload.([]interface{})
		require.True(t, ok)
		require.Len(t, slice, 1)
		require.Equal(t, "foo", slice[0])
	})

	t.Run("multiple misses, original was array", func(t *testing.T) {
		misses := []embeddingInputMeta{
			{Index: 0, Value: "foo"},
			{Index: 1, Value: "bar"},
		}
		payload := buildMissInputPayload(misses, true)
		slice, ok := payload.([]interface{})
		require.True(t, ok)
		require.Len(t, slice, 2)
		require.Equal(t, "foo", slice[0])
		require.Equal(t, "bar", slice[1])
	})

	t.Run("multiple misses, original not array", func(t *testing.T) {
		misses := []embeddingInputMeta{
			{Index: 0, Value: "foo"},
			{Index: 1, Value: "bar"},
		}
		payload := buildMissInputPayload(misses, false)
		slice, ok := payload.([]interface{})
		require.True(t, ok)
		require.Len(t, slice, 2)
		require.Equal(t, "foo", slice[0])
		require.Equal(t, "bar", slice[1])
	})

	t.Run("empty misses", func(t *testing.T) {
		misses := []embeddingInputMeta{}
		payload := buildMissInputPayload(misses, false)
		slice, ok := payload.([]interface{})
		require.True(t, ok)
		require.Len(t, slice, 0)
	})
}

// TestExtractDimensionsField tests extractDimensionsField with various types
func TestExtractDimensionsField(t *testing.T) {
	t.Run("dimensions as float64", func(t *testing.T) {
		payload := map[string]interface{}{
			"dimensions": float64(512),
		}
		dim := extractDimensionsField(payload)
		require.NotNil(t, dim)
		require.Equal(t, 512, *dim)
	})

	t.Run("dimensions as int", func(t *testing.T) {
		payload := map[string]interface{}{
			"dimensions": 256,
		}
		dim := extractDimensionsField(payload)
		require.NotNil(t, dim)
		require.Equal(t, 256, *dim)
	})

	t.Run("dimensions as json.Number", func(t *testing.T) {
		payload := map[string]interface{}{
			"dimensions": json.Number("128"),
		}
		dim := extractDimensionsField(payload)
		require.NotNil(t, dim)
		require.Equal(t, 128, *dim)
	})

	t.Run("dimensions missing", func(t *testing.T) {
		payload := map[string]interface{}{
			"model": "text-embedding-ada-002",
		}
		dim := extractDimensionsField(payload)
		require.Nil(t, dim)
	})

	t.Run("dimensions as invalid json.Number", func(t *testing.T) {
		payload := map[string]interface{}{
			"dimensions": json.Number("invalid"),
		}
		dim := extractDimensionsField(payload)
		require.Nil(t, dim)
	})

	t.Run("dimensions as string (should be nil)", func(t *testing.T) {
		payload := map[string]interface{}{
			"dimensions": "512",
		}
		dim := extractDimensionsField(payload)
		require.Nil(t, dim)
	})
}

// TestMarshalEmbeddingResponseFromRecords tests marshalEmbeddingResponseFromRecords
func TestMarshalEmbeddingResponseFromRecords(t *testing.T) {
	t.Run("single record", func(t *testing.T) {
		inputs := []embeddingInputMeta{
			{Index: 0, Value: "hello"},
		}
		tokenCount := 10
		hits := map[int]*db.EmbeddingRecord{
			0: {
				Embedding:  []float64{0.1, 0.2, 0.3},
				TokenCount: &tokenCount,
			},
		}
		bytes, err := marshalEmbeddingResponseFromRecords("text-embedding-ada-002", inputs, hits)
		require.NoError(t, err)

		var resp embeddingAPIResponse
		require.NoError(t, json.Unmarshal(bytes, &resp))
		require.Equal(t, "list", resp.Object)
		require.Equal(t, "text-embedding-ada-002", resp.Model)
		require.Len(t, resp.Data, 1)
		require.Equal(t, 0, resp.Data[0].Index)
		require.Equal(t, []float64{0.1, 0.2, 0.3}, resp.Data[0].Embedding)
		require.NotNil(t, resp.Usage)
		require.Equal(t, 10, resp.Usage.TotalTokens)
		require.Equal(t, 10, resp.Usage.PromptTokens)
	})

	t.Run("multiple records", func(t *testing.T) {
		inputs := []embeddingInputMeta{
			{Index: 0, Value: "hello"},
			{Index: 1, Value: "world"},
		}
		tokenCount1 := 5
		tokenCount2 := 7
		hits := map[int]*db.EmbeddingRecord{
			0: {
				Embedding:  []float64{0.1, 0.2},
				TokenCount: &tokenCount1,
			},
			1: {
				Embedding:  []float64{0.3, 0.4},
				TokenCount: &tokenCount2,
			},
		}
		bytes, err := marshalEmbeddingResponseFromRecords("text-embedding-ada-002", inputs, hits)
		require.NoError(t, err)

		var resp embeddingAPIResponse
		require.NoError(t, json.Unmarshal(bytes, &resp))
		require.Len(t, resp.Data, 2)
		require.Equal(t, 0, resp.Data[0].Index)
		require.Equal(t, 1, resp.Data[1].Index)
		require.NotNil(t, resp.Usage)
		require.Equal(t, 12, resp.Usage.TotalTokens)
	})

	t.Run("record without token count", func(t *testing.T) {
		inputs := []embeddingInputMeta{
			{Index: 0, Value: "hello"},
		}
		hits := map[int]*db.EmbeddingRecord{
			0: {
				Embedding:  []float64{0.1, 0.2},
				TokenCount: nil,
			},
		}
		bytes, err := marshalEmbeddingResponseFromRecords("text-embedding-ada-002", inputs, hits)
		require.NoError(t, err)

		var resp embeddingAPIResponse
		require.NoError(t, json.Unmarshal(bytes, &resp))
		require.NotNil(t, resp.Usage)
		require.Equal(t, 0, resp.Usage.TotalTokens)
	})

	t.Run("missing record in hits", func(t *testing.T) {
		inputs := []embeddingInputMeta{
			{Index: 0, Value: "hello"},
			{Index: 1, Value: "world"},
		}
		hits := map[int]*db.EmbeddingRecord{
			0: {
				Embedding: []float64{0.1, 0.2},
			},
		}
		bytes, err := marshalEmbeddingResponseFromRecords("text-embedding-ada-002", inputs, hits)
		require.NoError(t, err)

		var resp embeddingAPIResponse
		require.NoError(t, json.Unmarshal(bytes, &resp))
		require.Len(t, resp.Data, 1)
		require.Equal(t, 0, resp.Data[0].Index)
	})
}

// TestFirstNonEmpty tests firstNonEmpty helper function
func TestFirstNonEmpty(t *testing.T) {
	t.Run("first non-empty", func(t *testing.T) {
		result := firstNonEmpty("", "hello", "world")
		require.Equal(t, "hello", result)
	})

	t.Run("all empty", func(t *testing.T) {
		result := firstNonEmpty("", "  ", "")
		require.Equal(t, "", result)
	})

	t.Run("first is non-empty", func(t *testing.T) {
		result := firstNonEmpty("hello", "world")
		require.Equal(t, "hello", result)
	})

	t.Run("whitespace only", func(t *testing.T) {
		result := firstNonEmpty("   ", "\t", "valid")
		require.Equal(t, "valid", result)
	})

	t.Run("single empty", func(t *testing.T) {
		result := firstNonEmpty("")
		require.Equal(t, "", result)
	})

	t.Run("no arguments", func(t *testing.T) {
		result := firstNonEmpty()
		require.Equal(t, "", result)
	})
}

// TestEmbeddingAPIResponse_DataObject tests DataObject method
func TestEmbeddingAPIResponse_DataObject(t *testing.T) {
	t.Run("with data", func(t *testing.T) {
		resp := &embeddingAPIResponse{
			Data: []embeddingResponseDatum{
				{Object: "embedding"},
			},
		}
		require.Equal(t, "embedding", resp.DataObject())
	})

	t.Run("empty data", func(t *testing.T) {
		resp := &embeddingAPIResponse{
			Data: []embeddingResponseDatum{},
		}
		require.Equal(t, "", resp.DataObject())
	})

	t.Run("nil data", func(t *testing.T) {
		resp := &embeddingAPIResponse{
			Data: nil,
		}
		require.Equal(t, "", resp.DataObject())
	})

	t.Run("nil response", func(t *testing.T) {
		var resp *embeddingAPIResponse
		require.Equal(t, "", resp.DataObject())
	})
}

// TestShouldUseEmbeddingCache tests shouldUseEmbeddingCache
func TestShouldUseEmbeddingCache(t *testing.T) {
	cfg := &config.Config{
		TargetMap: map[string]string{
			"/v1/embeddings": "https://api.example.com/v1",
		},
	}
	handler := &Handler{
		cfg:       cfg,
		lbManager: NewLoadBalancerManager(),
	}

	t.Run("nil storage", func(t *testing.T) {
		handler.storage = nil
		req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
		require.False(t, handler.shouldUseEmbeddingCache(req))
	})

	t.Run("nil request", func(t *testing.T) {
		handler.storage = &fakeCacheStorage{}
		require.False(t, handler.shouldUseEmbeddingCache(nil))
	})

	t.Run("bypass header set", func(t *testing.T) {
		handler.storage = &fakeCacheStorage{}
		req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
		req.Header.Set(embeddingCacheBypassHeader, "1")
		require.False(t, handler.shouldUseEmbeddingCache(req))
	})

	t.Run("wrong method", func(t *testing.T) {
		handler.storage = &fakeCacheStorage{}
		req := httptest.NewRequest(http.MethodGet, "/v1/embeddings", nil)
		require.False(t, handler.shouldUseEmbeddingCache(req))
	})

	t.Run("wrong path", func(t *testing.T) {
		handler.storage = &fakeCacheStorage{}
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		require.False(t, handler.shouldUseEmbeddingCache(req))
	})

	t.Run("valid request", func(t *testing.T) {
		handler.storage = &fakeCacheStorage{}
		req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
		require.True(t, handler.shouldUseEmbeddingCache(req))
	})

	t.Run("path contains embeddings", func(t *testing.T) {
		handler.storage = &fakeCacheStorage{}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/embeddings/model", nil)
		require.True(t, handler.shouldUseEmbeddingCache(req))
	})
}
