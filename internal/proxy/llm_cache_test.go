package proxy

import (
	"bytes"
	"compress/gzip"
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

type fakeLLMCacheStorage struct {
	getLLMFn    func(ctx context.Context, request, modelName string) (*db.LLMRecord, error)
	upsertLLMFn func(ctx context.Context, rec *db.LLMRecord) error
}

func (f *fakeLLMCacheStorage) GetEmbedding(context.Context, string, string, *int) (*db.EmbeddingRecord, error) {
	return nil, nil
}

func (f *fakeLLMCacheStorage) UpsertEmbedding(context.Context, *db.EmbeddingRecord) error {
	return nil
}

func (f *fakeLLMCacheStorage) GetLLM(ctx context.Context, request, modelName string) (*db.LLMRecord, error) {
	if f.getLLMFn != nil {
		return f.getLLMFn(ctx, request, modelName)
	}
	return nil, nil
}

func (f *fakeLLMCacheStorage) UpsertLLM(ctx context.Context, rec *db.LLMRecord) error {
	if f.upsertLLMFn != nil {
		return f.upsertLLMFn(ctx, rec)
	}
	return nil
}

func newLLMTestHandler(storage cacheStorage) *Handler {
	cfg := &config.Config{
		TargetMap: map[string]string{
			"/chat/completions": "https://api.example.com/v1",
		},
	}
	return &Handler{
		cfg:       cfg,
		lbManager: NewLoadBalancerManager(),
		storage:   storage,
	}
}

func assertAnError() error {
	return fmt.Errorf("boom")
}

type errorReadCloser struct {
	err error
}

func (e *errorReadCloser) Read([]byte) (int, error) {
	return 0, e.err
}

func (e *errorReadCloser) Close() error {
	return nil
}

func TestShouldUseLLMCache(t *testing.T) {
	handler := newLLMTestHandler(nil)

	require.False(t, handler.shouldUseLLMCache(nil))

	req := httptest.NewRequest(http.MethodPost, "/chat/completions", nil)
	req.Header.Set(llmCacheBypassHeader, "1")
	require.False(t, handler.shouldUseLLMCache(req))

	req = httptest.NewRequest(http.MethodGet, "/chat/completions", nil)
	require.False(t, handler.shouldUseLLMCache(req))

	req = httptest.NewRequest(http.MethodPost, "/other", nil)
	require.False(t, handler.shouldUseLLMCache(req))

	handler.storage = &fakeLLMCacheStorage{}
	req = httptest.NewRequest(http.MethodPost, "/chat/completions", nil)
	require.True(t, handler.shouldUseLLMCache(req))
}

func TestHandleLLMCachePreProxy_Hit(t *testing.T) {
	cached := &db.LLMRecord{Response: []byte(`{"cached":true}`)}
	handler := newLLMTestHandler(&fakeLLMCacheStorage{
		getLLMFn: func(ctx context.Context, request, modelName string) (*db.LLMRecord, error) {
			require.Equal(t, "gpt-3.5-turbo", modelName)
			require.Contains(t, request, `"model":"gpt-3.5-turbo"`)
			return cached, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/chat/completions",
		strings.NewReader(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`))
	resp := httptest.NewRecorder()

	handled, meta := handler.handleLLMCachePreProxy(resp, req)
	require.True(t, handled)
	require.Nil(t, meta)
	require.Equal(t, "HIT", resp.Header().Get("X-LLM-Cache"))

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	require.Equal(t, true, payload["cached"])
}

func TestHandleLLMCachePreProxy_Miss(t *testing.T) {
	var lookedUpRequest string
	handler := newLLMTestHandler(&fakeLLMCacheStorage{
		getLLMFn: func(ctx context.Context, request, modelName string) (*db.LLMRecord, error) {
			lookedUpRequest = request
			return nil, nil
		},
	})

	body := `{"model":"gpt-4","temperature":0.7,"max_tokens":256,"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(body))
	resp := httptest.NewRecorder()

	handled, meta := handler.handleLLMCachePreProxy(resp, req)
	require.False(t, handled)
	require.NotNil(t, meta)
	require.Equal(t, body, lookedUpRequest)
	require.Equal(t, "gpt-4", meta.model)
	require.NotNil(t, meta.temperature)
	require.InDelta(t, 0.7, *meta.temperature, 0.001)
	require.NotNil(t, meta.maxTokens)
	require.Equal(t, 256, *meta.maxTokens)
	require.False(t, meta.stream)
	require.WithinDuration(t, time.Now(), meta.startTime, time.Second)

	bodyBytes, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.JSONEq(t, body, string(bodyBytes))
}

func TestHandleLLMCachePreProxy_ReadError(t *testing.T) {
	handler := newLLMTestHandler(&fakeLLMCacheStorage{})
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", nil)
	req.Body = &errorReadCloser{err: assertAnError()}
	resp := httptest.NewRecorder()

	handled, meta := handler.handleLLMCachePreProxy(resp, req)
	require.False(t, handled)
	require.Nil(t, meta)

	bodyBytes, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Len(t, bodyBytes, 0)
}

func TestHandleLLMCachePreProxy_InvalidJSON(t *testing.T) {
	handler := newLLMTestHandler(&fakeLLMCacheStorage{})
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{invalid`))
	resp := httptest.NewRecorder()

	handled, meta := handler.handleLLMCachePreProxy(resp, req)
	require.False(t, handled)
	require.Nil(t, meta)
}

func TestHandleLLMCachePreProxy_GetError(t *testing.T) {
	handler := newLLMTestHandler(&fakeLLMCacheStorage{
		getLLMFn: func(ctx context.Context, request, modelName string) (*db.LLMRecord, error) {
			return nil, assertAnError()
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/chat/completions",
		strings.NewReader(`{"model":"gpt-4"}`))
	resp := httptest.NewRecorder()

	handled, meta := handler.handleLLMCachePreProxy(resp, req)
	require.False(t, handled)
	require.NotNil(t, meta)
	require.Equal(t, "gpt-4", meta.model)
}

func TestHandleLLMCachePreProxy_StreamBypass(t *testing.T) {
	handler := newLLMTestHandler(&fakeLLMCacheStorage{})
	req := httptest.NewRequest(http.MethodPost, "/chat/completions",
		strings.NewReader(`{"model":"gpt-4","stream":"yes"}`))
	resp := httptest.NewRecorder()

	handled, meta := handler.handleLLMCachePreProxy(resp, req)
	require.False(t, handled)
	require.Nil(t, meta)
}

func TestHandleLLMCachePreProxy_StreamTrue(t *testing.T) {
	handler := newLLMTestHandler(&fakeLLMCacheStorage{})
	req := httptest.NewRequest(http.MethodPost, "/chat/completions",
		strings.NewReader(`{"model":"gpt-4","stream":true}`))
	resp := httptest.NewRecorder()

	handled, meta := handler.handleLLMCachePreProxy(resp, req)
	require.False(t, handled)
	require.Nil(t, meta)
}

func TestHandleLLMCachePreProxy_EmptyBodyAndMissingModel(t *testing.T) {
	handler := newLLMTestHandler(&fakeLLMCacheStorage{})

	req := httptest.NewRequest(http.MethodPost, "/chat/completions", nil)
	resp := httptest.NewRecorder()
	handled, meta := handler.handleLLMCachePreProxy(resp, req)
	require.False(t, handled)
	require.Nil(t, meta)

	req = httptest.NewRequest(http.MethodPost, "/chat/completions",
		strings.NewReader(`{"stream":false}`))
	resp = httptest.NewRecorder()
	handled, meta = handler.handleLLMCachePreProxy(resp, req)
	require.False(t, handled)
	require.Nil(t, meta)
}

func TestHandleLLMCachePostResponse_StoreGzip(t *testing.T) {
	var stored *db.LLMRecord
	handler := newLLMTestHandler(&fakeLLMCacheStorage{
		upsertLLMFn: func(ctx context.Context, rec *db.LLMRecord) error {
			stored = rec
			return nil
		},
	})

	responsePayload := `{"choices":[{"message":{"content":"ok"}}],"usage":{"total_tokens":10,"prompt_tokens":4,"completion_tokens":6}}`
	var compressed bytes.Buffer
	gw := gzip.NewWriter(&compressed)
	_, _ = gw.Write([]byte(responsePayload))
	_ = gw.Close()

	meta := &llmCacheMetadata{
		prompt: `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`,
		model:  "gpt-4",
		temperature: func() *float32 {
			v := float32(0.5)
			return &v
		}(),
		maxTokens: func() *int {
			v := 128
			return &v
		}(),
		startTime: time.Now().Add(-time.Second),
		requestID: "req-1",
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(compressed.Bytes())),
		Header:     make(http.Header),
		Request:    httptest.NewRequest(http.MethodPost, "/chat/completions", nil).WithContext(context.Background()),
	}
	resp.Header.Set("Content-Encoding", "gzip")

	err := handler.handleLLMCachePostResponse(resp, meta)
	require.NoError(t, err)
	require.Equal(t, "MISS", resp.Header.Get("X-LLM-Cache"))
	require.NotNil(t, stored)
	require.Equal(t, "gpt-4", stored.ModelName)
	require.Equal(t, meta.temperature, stored.Temperature)
	require.Equal(t, meta.maxTokens, stored.MaxTokens)
	require.NotNil(t, stored.TotalTokens)
	require.Equal(t, 10, *stored.TotalTokens)
	require.NotNil(t, stored.PromptTokens)
	require.NotNil(t, stored.CompletionTokens)

	// Stored response should be the decompressed JSON.
	require.Equal(t, json.RawMessage(responsePayload), stored.Response)

	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, compressed.Bytes(), bodyBytes)
	require.Equal(t, int64(len(compressed.Bytes())), resp.ContentLength)
}

func TestHandleLLMCachePostResponse_InvalidUTF8(t *testing.T) {
	upsertCalled := false
	handler := newLLMTestHandler(&fakeLLMCacheStorage{
		upsertLLMFn: func(ctx context.Context, rec *db.LLMRecord) error {
			upsertCalled = true
			return nil
		},
	})

	meta := &llmCacheMetadata{
		prompt:    `{"model":"gpt-4"}`,
		model:     "gpt-4",
		startTime: time.Now(),
		requestID: "req-2",
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte{0xff, 0xfe})),
		Header:     make(http.Header),
		Request:    httptest.NewRequest(http.MethodPost, "/chat/completions", nil).WithContext(context.Background()),
	}

	err := handler.handleLLMCachePostResponse(resp, meta)
	require.NoError(t, err)
	require.False(t, upsertCalled)
	require.Equal(t, "MISS", resp.Header.Get("X-LLM-Cache"))
}

func TestHandleLLMCachePostResponse_NonOKOrNilBody(t *testing.T) {
	handler := newLLMTestHandler(&fakeLLMCacheStorage{
		upsertLLMFn: func(ctx context.Context, rec *db.LLMRecord) error {
			return assertAnError()
		},
	})
	meta := &llmCacheMetadata{
		prompt:    "{}",
		model:     "gpt-4",
		startTime: time.Now(),
		requestID: "req-3",
	}

	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Header:     make(http.Header),
		Request:    httptest.NewRequest(http.MethodPost, "/chat/completions", nil).WithContext(context.Background()),
	}
	require.NoError(t, handler.handleLLMCachePostResponse(resp, meta))

	resp = &http.Response{
		StatusCode: http.StatusOK,
		Body:       nil,
		Header:     make(http.Header),
		Request:    httptest.NewRequest(http.MethodPost, "/chat/completions", nil).WithContext(context.Background()),
	}
	require.NoError(t, handler.handleLLMCachePostResponse(resp, meta))
}

func TestHandleLLMCachePostResponse_EmptyBody(t *testing.T) {
	var stored *db.LLMRecord
	handler := newLLMTestHandler(&fakeLLMCacheStorage{
		upsertLLMFn: func(ctx context.Context, rec *db.LLMRecord) error {
			stored = rec
			return nil
		},
	})

	meta := &llmCacheMetadata{
		prompt:    `{"model":"gpt-3"}`,
		model:     "gpt-3",
		startTime: time.Now(),
		requestID: "req-4",
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Header:     make(http.Header),
		Request:    httptest.NewRequest(http.MethodPost, "/chat/completions", nil).WithContext(context.Background()),
	}

	require.NoError(t, handler.handleLLMCachePostResponse(resp, meta))
	require.NotNil(t, stored)
	require.Equal(t, int64(0), resp.ContentLength)
	require.Equal(t, "", resp.Header.Get("Content-Length"))
}

func TestHandleLLMCachePostResponse_InvalidGzipStillStores(t *testing.T) {
	var stored *db.LLMRecord
	handler := newLLMTestHandler(&fakeLLMCacheStorage{
		upsertLLMFn: func(ctx context.Context, rec *db.LLMRecord) error {
			stored = rec
			return nil
		},
	})

	meta := &llmCacheMetadata{
		prompt:    `{"model":"gpt-4"}`,
		model:     "gpt-4",
		startTime: time.Now(),
		requestID: "req-5",
	}

	// Marked gzip but not actually compressed to trigger gzip reader error.
	body := []byte(`{"message":"not-gzip"}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
		Request:    httptest.NewRequest(http.MethodPost, "/chat/completions", nil).WithContext(context.Background()),
	}
	resp.Header.Set("Content-Encoding", "gzip")

	require.NoError(t, handler.handleLLMCachePostResponse(resp, meta))
	require.NotNil(t, stored)
	require.Equal(t, json.RawMessage(body), stored.Response)
}

func TestEnsureJSONFormat(t *testing.T) {
	raw, err := ensureJSONFormat(`{"foo":1}`)
	require.NoError(t, err)
	require.Equal(t, json.RawMessage(`{"foo":1}`), raw)

	raw, err = ensureJSONFormat("plain-text")
	require.NoError(t, err)

	var val string
	require.NoError(t, json.Unmarshal(raw, &val))
	require.Equal(t, "plain-text", val)
}
