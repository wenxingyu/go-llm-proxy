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
