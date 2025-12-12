package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go-llm-server/internal/utils"
	"go-llm-server/pkg/db"
	"go-llm-server/pkg/logger"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const embeddingCacheBypassHeader = "X-Embedding-Cache-Bypass"

var embeddingCacheContextKey = cacheContextKey{}

type embeddingCacheMetadata struct {
	model      string
	total      int
	hits       map[int]*db.EmbeddingRecord // original index -> record
	misses     []embeddingInputMeta        // misses in the order sent upstream
	dimensions *int
	startTime  time.Time
	requestID  string
}

type embeddingInputMeta struct {
	Index int
	Value string
}

type embeddingAPIResponse struct {
	ID      string                   `json:"id,omitempty"`
	Object  string                   `json:"object"`
	Created int                      `json:"created"`
	Model   string                   `json:"model"`
	Data    []embeddingResponseDatum `json:"data"`
	Usage   *embeddingUsage          `json:"usage,omitempty"`
}

type embeddingResponseDatum struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

type embeddingUsage struct {
	PromptTokens        int     `json:"prompt_tokens"`
	TotalTokens         int     `json:"total_tokens"`
	CompletionTokens    int     `json:"completion_tokens"`
	PromptTokensDetails *string `json:"prompt_tokens_details"`
}

// shouldUseEmbeddingCache 保持原判断：storage 存在, POST, URL 包含 embeddings，并且未显式绕过
func (h *Handler) shouldUseEmbeddingCache(r *http.Request) bool {
	if h.storage == nil || r == nil {
		return false
	}
	if r.Header.Get(embeddingCacheBypassHeader) != "" {
		return false
	}
	if r.Method != http.MethodPost {
		return false
	}
	return strings.Contains(r.URL.Path, "embeddings")
}

// handleEmbeddingCachePreProxy: 读取请求 body，尝试命中 cache。
// 如果全部命中：直接返回 response 给 client 并返回 (true, nil)
// 否则：重写请求 body 只包含 misses，返回 (false, meta) 以便在后续处理响应时合并
func (h *Handler) handleEmbeddingCachePreProxy(w http.ResponseWriter, r *http.Request) (bool, *embeddingCacheMetadata) {
	if r.Body == nil {
		return false, nil
	}
	bodyBytes, err := io.ReadAll(r.Body)
	defer func() { _ = r.Body.Close() }()
	if err != nil {
		logger.Warn("embedding-cache: failed to read request body",
			zap.String("requestId", utils.GetRequestID(r)),
			zap.Error(err))
		// restore empty body
		r.Body = io.NopCloser(bytes.NewReader(nil))
		return false, nil
	}
	if len(bodyBytes) == 0 {
		return false, nil
	}

	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	var payload map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		logger.Warn("embedding-cache: failed to unmarshal request body",
			zap.String("requestId", utils.GetRequestID(r)),
			zap.Error(err))
		return false, nil
	}

	modelName, _ := payload["model"].(string)
	if modelName == "" {
		return false, nil
	}

	inputRaw, ok := payload["input"]
	if !ok {
		return false, nil
	}

	dimensions := extractDimensionsField(payload)

	inputs, inputWasArray, err := extractEmbeddingInputs(inputRaw)
	if err != nil || len(inputs) == 0 {
		if err != nil {
			logger.Warn("embedding-cache: invalid input",
				zap.String("requestId", utils.GetRequestID(r)),
				zap.Error(err))
		}
		return false, nil
	}

	requestID := utils.GetRequestID(r)

	hits := make(map[int]*db.EmbeddingRecord)
	misses := make([]embeddingInputMeta, 0, len(inputs))

	// 查询 cache
	for _, input := range inputs {
		rec, err := h.storage.GetEmbedding(r.Context(), input.Value, modelName, dimensions)
		if err != nil {
			logger.Warn("embedding-cache: storage lookup failed",
				zap.String("requestId", requestID),
				zap.String("model", modelName),
				zap.Error(err))
			// 在遇到存储错误时直接绕过 cache（并通知下游）
			w.Header().Set("X-Embedding-Cache", "BYPASS")
			return false, nil
		}
		if rec != nil {
			hits[input.Index] = rec
		} else {
			// miss: append to misses (preserve original index)
			misses = append(misses, input)
		}
	}

	// 全部命中：直接返回合并的 response
	if len(misses) == 0 {
		responseBytes, err := marshalEmbeddingResponseFromRecords(modelName, inputs, hits)
		if err != nil {
			logger.Warn("embedding-cache: failed to marshal HIT response",
				zap.String("requestId", requestID),
				zap.Error(err))
			return false, nil
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Embedding-Cache", "HIT")
		_, _ = w.Write(responseBytes)
		logger.Info("embedding-cache: served embeddings from cache",
			zap.String("requestId", requestID),
			zap.String("model", modelName),
			zap.Int("hits", len(inputs)))
		return true, nil
	}

	// 有 miss：重写请求 body 只包含 misses
	payload["input"] = buildMissInputPayload(misses, inputWasArray)
	newBody, err := json.Marshal(payload)
	if err != nil {
		logger.Warn("embedding-cache: failed to marshal payload for misses",
			zap.String("requestId", requestID),
			zap.Error(err))
		return false, nil
	}
	r.Body = io.NopCloser(bytes.NewReader(newBody))
	r.ContentLength = int64(len(newBody))
	r.Header.Set("Content-Length", strconv.Itoa(len(newBody)))

	meta := &embeddingCacheMetadata{
		model:      modelName,
		total:      len(inputs),
		hits:       hits,
		misses:     misses,
		dimensions: dimensions,
		startTime:  time.Now(),
		requestID:  requestID,
	}

	return false, meta
}

func extractDimensionsField(payload map[string]interface{}) *int {
	var dimensions *int
	if dimVal, ok := payload["dimensions"]; ok {
		switch v := dimVal.(type) {
		case float64:
			d := int(v)
			dimensions = &d
		case int:
			d := v
			dimensions = &d
		case json.Number:
			if parsed, err := v.Int64(); err == nil {
				d := int(parsed)
				dimensions = &d
			}
		}
	}
	return dimensions
}

// extractEmbeddingInputs: 只支持 string 或 []string
// 返回 inputs slice（每个包含原始的索引与字符串），以及一个布尔表示原始 input 是否为数组
func extractEmbeddingInputs(raw interface{}) ([]embeddingInputMeta, bool, error) {
	switch v := raw.(type) {
	case string:
		return []embeddingInputMeta{{Index: 0, Value: v}}, false, nil
	case []string:
		out := make([]embeddingInputMeta, 0, len(v))
		for i, s := range v {
			out = append(out, embeddingInputMeta{Index: i, Value: s})
		}
		return out, true, nil
	case []interface{}:
		// json.Unmarshal 会把数组解析成 []interface{}，这里兜底转换
		out := make([]embeddingInputMeta, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, true, fmt.Errorf("invalid input: only string or []string are allowed")
			}
			out = append(out, embeddingInputMeta{Index: i, Value: s})
		}
		return out, true, nil
	default:
		return nil, false, fmt.Errorf("invalid input: only string or []string are allowed")
	}
}

// buildMissInputPayload: 根据 originalWasArray 决定是返回单值还是数组
func buildMissInputPayload(misses []embeddingInputMeta, originalWasArray bool) interface{} {
	if !originalWasArray && len(misses) == 1 {
		return misses[0].Value
	}
	values := make([]interface{}, 0, len(misses))
	for _, m := range misses {
		values = append(values, m.Value)
	}
	return values
}

// marshalEmbeddingResponseFromRecords: 根据 inputs 顺序（原始索引），从 hits map 生成 API 响应（只包含命中的部分）
func marshalEmbeddingResponseFromRecords(model string, inputs []embeddingInputMeta, hits map[int]*db.EmbeddingRecord) ([]byte, error) {
	data := make([]embeddingResponseDatum, 0, len(inputs))
	totalTokens := 0
	dataObject := "embedding"
	for _, in := range inputs {
		if rec, ok := hits[in.Index]; ok && rec != nil {
			data = append(data, embeddingResponseDatum{
				Object:    dataObject,
				Index:     in.Index,
				Embedding: rec.Embedding,
			})
			if rec.TokenCount != nil {
				totalTokens += *rec.TokenCount
			}
		}
	}
	response := embeddingAPIResponse{
		ID:      strings.ToLower("emb-" + uuid.New().String()),
		Object:  "list",
		Created: int(time.Now().Unix()),
		Data:    data,
		Model:   model,
	}
	response.Usage = &embeddingUsage{
		PromptTokens: totalTokens,
		TotalTokens:  totalTokens,
	}
	return json.Marshal(response)
}

// handleEmbeddingCachePostResponse: 在 upstream 返回后，将 misses 的 embedding 保存并与 hits 合并，替换 response body 返回给 client
func (h *Handler) handleEmbeddingCachePostResponse(resp *http.Response, meta *embeddingCacheMetadata) error {
	if resp == nil || resp.Request == nil || meta == nil {
		return nil
	}
	// 非 200 或 空 body：直接标注 MISS 并返回（不修改）
	if resp.StatusCode != http.StatusOK || resp.Body == nil {
		resp.Header.Set("X-Embedding-Cache", "MISS")
		return nil
	}

	rawBodyBytes, err := utils.ReadResponseBody(resp, meta.requestID)
	if err != nil {
		return err
	}
	var payload embeddingAPIResponse
	if err := json.Unmarshal(rawBodyBytes, &payload); err != nil {
		logger.Warn("embedding-cache: failed to parse upstream response",
			zap.String("requestId", meta.requestID),
			zap.Error(err))
		return err
	}

	// 保存 upstream 返回的 embeddings（假设 upstream 返回的 data index 是 0..n-1，顺序对应我们发送的 misses）
	newRecords := make(map[int]*db.EmbeddingRecord) // original index -> record
	endTime := time.Now()
	totalTokens := 0
	singleRecordTokens := 0
	if len(payload.Data) == 1 {
		singleRecordTokens = payload.Usage.TotalTokens
	}
	for _, data := range payload.Data {
		// data.Index is index within the returned misses array
		if data.Index < 0 || data.Index >= len(meta.misses) {
			// 忽略异常 index
			continue
		}
		miss := meta.misses[data.Index] // miss holds original Index and Value
		if miss.Value == "" {
			continue
		}
		rec := &db.EmbeddingRecord{
			RequestID:  meta.requestID,
			InputText:  miss.Value,
			ModelName:  meta.model,
			Dimensions: meta.dimensions,
			Embedding:  data.Embedding,
			TokenCount: &singleRecordTokens,
			StartTime:  &meta.startTime,
			EndTime:    &endTime,
		}
		if err := h.storage.UpsertEmbedding(resp.Request.Context(), rec); err != nil {
			logger.Warn("embedding-cache: failed to persist embedding",
				zap.String("requestId", meta.requestID),
				zap.String("model", meta.model),
				zap.Error(err))
			// persist 失败不影响返回；但不会把 rec 写入 newRecords
			continue
		}
		newRecords[miss.Index] = rec
	}

	// 合并 hits 与 newRecords，按原始顺序输出
	combined := make([]embeddingResponseDatum, 0, meta.total)
	dataObject := payload.DataObject() // helper from below to pick object (guard)
	if dataObject == "" {
		dataObject = "embedding"
	}
	for i := 0; i < meta.total; i++ {
		var rec *db.EmbeddingRecord
		if hrec, ok := meta.hits[i]; ok {
			rec = hrec
		} else if nrec, ok := newRecords[i]; ok {
			rec = nrec
		}
		if rec == nil {
			// skip missing entries (shouldn't happen normally)
			continue
		}
		combined = append(combined, embeddingResponseDatum{
			Object:    dataObject,
			Index:     i,
			Embedding: rec.Embedding,
		})
		if rec.TokenCount != nil {
			totalTokens += *rec.TokenCount
		}
	}
	// 累加 upstream report 的 token 使用量（如果有）
	if payload.Usage != nil {
		totalTokens += payload.Usage.TotalTokens
	}

	combinedPayload := embeddingAPIResponse{
		ID:      payload.ID,
		Object:  payload.Object,
		Created: int(time.Now().Unix()),
		Model:   firstNonEmpty(payload.Model, meta.model),
		Data:    combined,
	}
	if totalTokens >= 0 {
		combinedPayload.Usage = &embeddingUsage{
			PromptTokens: totalTokens,
			TotalTokens:  totalTokens,
		}
	}

	finalBytes, err := json.Marshal(combinedPayload)
	if err != nil {
		logger.Warn("embedding-cache: failed to marshal combined payload",
			zap.String("requestId", meta.requestID),
			zap.Error(err))
		return nil
	}

	// 替换 response body 和 headers
	resp.Body = io.NopCloser(bytes.NewReader(finalBytes))
	resp.ContentLength = int64(len(finalBytes))
	resp.Header.Set("Content-Length", strconv.Itoa(len(finalBytes)))
	resp.Header.Set("Content-Type", "application/json")

	cacheStatus := "MISS"
	if len(meta.hits) > 0 && len(newRecords) > 0 {
		cacheStatus = "PARTIAL"
	} else if len(meta.hits) > 0 {
		cacheStatus = "HIT"
	}
	resp.Header.Set("X-Embedding-Cache", cacheStatus)

	logger.Info("embedding-cache: processed upstream response",
		zap.String("requestId", meta.requestID),
		zap.String("model", meta.model),
		zap.String("cacheStatus", cacheStatus),
		zap.Int("hits", len(meta.hits)),
		zap.Int("misses", len(newRecords)))

	return nil
}

// firstNonEmpty: 返回第一个非空字符串
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// DataObject helper: returns the object field value from payload.Data if present (defensive)
func (p *embeddingAPIResponse) DataObject() string {
	if p == nil || len(p.Data) == 0 {
		return ""
	}
	return p.Data[0].Object
}
