package proxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go-llm-server/internal/utils"
	"go-llm-server/pkg/db"
	"go-llm-server/pkg/logger"

	"go.uber.org/zap"
)

const llmCacheBypassHeader = "X-LLM-Cache-Bypass"

var llmCacheContextKey = cacheContextKey{}

type llmCacheMetadata struct {
	prompt      string
	model       string
	temperature *float32
	maxTokens   *int
	stream      bool
	startTime   time.Time
	requestID   string
}

func (h *Handler) shouldUseLLMCache(r *http.Request) bool {
	if h.storage == nil || r == nil {
		return false
	}

	if r.Header.Get(llmCacheBypassHeader) != "" {
		return false
	}

	return r.URL.Path == "/chat/completions" && r.Method == http.MethodPost
}

func (h *Handler) handleLLMCachePreProxy(w http.ResponseWriter, r *http.Request) (bool, *llmCacheMetadata) {
	if r == nil || r.Body == nil {
		return false, nil
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Warn("Failed to read request body for LLM cache lookup",
			zap.String("requestId", utils.GetRequestID(r)),
			zap.Error(err))
		r.Body = io.NopCloser(bytes.NewReader(nil))
		return false, nil
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	if len(bodyBytes) == 0 {
		return false, nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		logger.Warn("Failed to unmarshal request body for LLM cache lookup",
			zap.String("requestId", utils.GetRequestID(r)),
			zap.Error(err))
	}

	streamBool := false
	if streamVal, ok := payload["stream"]; ok {
		if v, ok := streamVal.(bool); ok {
			streamBool = v
		} else {
			return false, nil
		}
	}
	if streamBool {
		return false, nil
	}

	model, _ := payload["model"].(string)
	if model == "" {
		return false, nil
	}

	var temperature *float32
	if v, ok := payload["temperature"]; ok {
		switch t := v.(type) {
		case float64:
			temp := float32(t)
			temperature = &temp
		case float32:
			temp := t
			temperature = &temp
		}
	}

	var maxTokens *int
	if v, ok := payload["max_tokens"]; ok {
		switch t := v.(type) {
		case float64:
			mt := int(t)
			maxTokens = &mt
		case float32:
			mt := int(t)
			maxTokens = &mt
		case int:
			mt := t
			maxTokens = &mt
		}
	}

	request := string(bodyBytes)
	rec, err := h.storage.GetLLM(r.Context(), request, model)
	if err != nil {
		logger.Warn("LLM cache lookup failed",
			zap.String("requestId", utils.GetRequestID(r)),
			zap.String("model", model),
			zap.Error(err))
	} else if rec != nil && len(rec.Response) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-LLM-Cache", "HIT")
		_, _ = w.Write(rec.Response)
		logger.Info("Served response from LLM cache",
			zap.String("requestId", utils.GetRequestID(r)),
			zap.String("model", model))
		return true, nil
	}

	return false, &llmCacheMetadata{
		prompt:      request,
		model:       model,
		temperature: temperature,
		maxTokens:   maxTokens,
		stream:      false,
		startTime:   time.Now(),
		requestID:   utils.GetRequestID(r),
	}
}

func (h *Handler) handleLLMCachePostResponse(resp *http.Response, meta *llmCacheMetadata) error {
	if resp.StatusCode != http.StatusOK || resp.Body == nil {
		return nil
	}

	resp.Header.Set("X-LLM-Cache", "MISS")

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Warn("Failed to read response body for LLM cache storage",
			zap.Error(err))
		return nil
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	if len(bodyBytes) > 0 {
		resp.ContentLength = int64(len(bodyBytes))
		resp.Header.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
	} else {
		resp.ContentLength = 0
		resp.Header.Del("Content-Length")
	}

	bodyToStore := bodyBytes
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" {
		if strings.Contains(strings.ToLower(encoding), "gzip") {
			gr, err := gzip.NewReader(bytes.NewReader(bodyBytes))
			if err != nil {
				logger.Warn("Failed to decompress gzip response for LLM cache",
					zap.String("model", meta.model),
					zap.Error(err))
			} else {
				var decompressed bytes.Buffer
				if _, err := io.Copy(&decompressed, gr); err != nil {
					logger.Warn("Failed to copy decompressed response for LLM cache",
						zap.String("model", meta.model),
						zap.Error(err))
				} else {
					bodyToStore = decompressed.Bytes()
				}
				_ = gr.Close()
			}
		}
	}

	if !utf8.Valid(bodyToStore) {
		logger.Warn("LLM response is not valid UTF-8, skipping cache",
			zap.String("model", meta.model))
		return nil
	}

	var totalTokensPtr, promptTokensPtr, completionTokensPtr *int
	var responsePayload struct {
		Usage *struct {
			TotalTokens      *int `json:"total_tokens"`
			PromptTokens     *int `json:"prompt_tokens"`
			CompletionTokens *int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(bodyToStore, &responsePayload); err == nil {
		if responsePayload.Usage != nil {
			if responsePayload.Usage.TotalTokens != nil {
				total := *responsePayload.Usage.TotalTokens
				totalTokensPtr = &total
			}
			if responsePayload.Usage.PromptTokens != nil {
				prompt := *responsePayload.Usage.PromptTokens
				promptTokensPtr = &prompt
			}
			if responsePayload.Usage.CompletionTokens != nil {
				completion := *responsePayload.Usage.CompletionTokens
				completionTokensPtr = &completion
			}
		}
	}

	promptJSON, err := ensureJSONFormat(meta.prompt)
	if err != nil {
		logger.Warn("Failed to convert prompt to JSON format",
			zap.String("model", meta.model),
			zap.Error(err))
		promptJSON = json.RawMessage(fmt.Sprintf(`"%s"`, strings.ReplaceAll(meta.prompt, `"`, `\"`)))
	}

	responseJSON, err := ensureJSONFormat(string(bodyToStore))
	if err != nil {
		logger.Warn("Failed to convert response to JSON format",
			zap.String("model", meta.model),
			zap.Error(err))
		responseJSON = json.RawMessage(fmt.Sprintf(`"%s"`, strings.ReplaceAll(string(bodyToStore), `"`, `\"`)))
	}

	endTime := time.Now()
	startTime := meta.startTime

	llmRecord := &db.LLMRecord{
		RequestID:        meta.requestID,
		Request:          promptJSON,
		ModelName:        meta.model,
		Temperature:      meta.temperature,
		MaxTokens:        meta.maxTokens,
		Response:         responseJSON,
		TotalTokens:      totalTokensPtr,
		PromptTokens:     promptTokensPtr,
		CompletionTokens: completionTokensPtr,
		StartTime:        &startTime,
		EndTime:          &endTime,
	}

	if err := h.storage.UpsertLLM(resp.Request.Context(), llmRecord); err != nil {
		logger.Warn("Failed to store response in LLM cache",
			zap.String("model", meta.model),
			zap.Error(err))
	} else {
		resp.Header.Set("X-LLM-Cache", "MISS")
		logger.Info("Stored response in LLM cache",
			zap.String("model", meta.model))
	}

	return nil
}

// ensureJSONFormat 确保输入是有效的 JSON 格式，如果不是则尝试解析或包装
func ensureJSONFormat(input string) (json.RawMessage, error) {
	// 尝试解析为 JSON，如果成功直接返回
	var jsonVal interface{}
	if err := json.Unmarshal([]byte(input), &jsonVal); err == nil {
		// 是有效的 JSON，直接返回
		return json.RawMessage(input), nil
	}
	// 如果不是有效的 JSON，将其包装为 JSON 字符串
	jsonBytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(jsonBytes), nil
}
