package proxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"go-llm-server/internal/utils"
	"go-llm-server/pkg/db"
	"go-llm-server/pkg/logger"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

const embeddingCacheBypassHeader = "X-Embedding-Cache-Bypass"

type embeddingCacheMetadata struct {
	model              string
	totalInputs        int
	hits               map[int]*db.EmbeddingRecord
	missOrder          []embeddingInputMeta
	startTime          time.Time
	requestID          string
	originalWasArray   bool
	missRequestIndex   map[int]embeddingInputMeta
	responseDataObject string
}

type embeddingInputMeta struct {
	Index        int
	RequestIndex int
	Normalized   string
	Raw          interface{}
}

type embeddingAPIResponse struct {
	Object string                   `json:"object"`
	Data   []embeddingResponseDatum `json:"data"`
	Model  string                   `json:"model"`
	Usage  *embeddingUsage          `json:"usage,omitempty"`
	ID     string                   `json:"id,omitempty"`
	Error  map[string]interface{}   `json:"error,omitempty"`
	Extra  map[string]interface{}   `json:"-"`
}

type embeddingResponseDatum struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

type embeddingUsage struct {
	PromptTokens *int `json:"prompt_tokens,omitempty"`
	TotalTokens  *int `json:"total_tokens,omitempty"`
}

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

func (h *Handler) handleEmbeddingCachePreProxy(w http.ResponseWriter, r *http.Request) (bool, *embeddingCacheMetadata) {
	if r.Body == nil {
		return false, nil
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Warn("Failed to read embedding request body",
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
		logger.Warn("Failed to unmarshal embedding request",
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

	inputs, inputWasArray, err := extractEmbeddingInputs(inputRaw)
	if err != nil || len(inputs) == 0 {
		if err != nil {
			logger.Warn("Failed to normalize embedding inputs",
				zap.String("requestId", utils.GetRequestID(r)),
				zap.Error(err))
		}
		return false, nil
	}

	requestID := utils.GetRequestID(r)

	hits := make(map[int]*db.EmbeddingRecord)
	missOrder := make([]embeddingInputMeta, 0)
	for _, input := range inputs {
		rec, err := h.storage.GetEmbedding(r.Context(), input.Normalized, modelName)
		if err != nil {
			logger.Warn("Embedding cache lookup failed",
				zap.String("requestId", requestID),
				zap.String("model", modelName),
				zap.Error(err))
			w.Header().Set("X-Embedding-Cache", "BYPASS")
			return false, nil
		}
		if rec != nil {
			hits[input.Index] = rec
			continue
		}
		input.RequestIndex = len(missOrder)
		missOrder = append(missOrder, input)
	}

	if len(missOrder) == 0 {
		responseBytes, err := marshalEmbeddingResponseFromRecords(modelName, inputs, hits)
		if err != nil {
			logger.Warn("Failed to marshal embedding cache HIT response",
				zap.String("requestId", requestID),
				zap.Error(err))
			return false, nil
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Embedding-Cache", "HIT")
		_, _ = w.Write(responseBytes)
		logger.Info("Served embeddings from cache",
			zap.String("requestId", requestID),
			zap.String("model", modelName),
			zap.Int("hits", len(inputs)))
		return true, nil
	}

	// Rewrite payload to include only missing inputs
	payload["input"] = buildMissInputPayload(missOrder, inputWasArray)
	newBody, err := json.Marshal(payload)
	if err != nil {
		logger.Warn("Failed to marshal embedding payload for misses",
			zap.String("requestId", requestID),
			zap.Error(err))
		return false, nil
	}

	r.Body = io.NopCloser(bytes.NewReader(newBody))
	r.ContentLength = int64(len(newBody))
	r.Header.Set("Content-Length", strconv.Itoa(len(newBody)))

	meta := &embeddingCacheMetadata{
		model:            modelName,
		totalInputs:      len(inputs),
		hits:             hits,
		missOrder:        missOrder,
		startTime:        time.Now(),
		requestID:        requestID,
		originalWasArray: inputWasArray,
		missRequestIndex: make(map[int]embeddingInputMeta, len(missOrder)),
	}
	for _, miss := range missOrder {
		meta.missRequestIndex[miss.RequestIndex] = miss
	}

	return false, meta
}

func extractEmbeddingInputs(raw interface{}) ([]embeddingInputMeta, bool, error) {
	switch v := raw.(type) {
	case string:
		return []embeddingInputMeta{
			{Index: 0, Normalized: v, Raw: v},
		}, false, nil
	case json.Number:
		return []embeddingInputMeta{
			{Index: 0, Normalized: v.String(), Raw: v},
		}, false, nil
	case []interface{}:
		inputs := make([]embeddingInputMeta, 0, len(v))
		for idx, item := range v {
			normalized, err := normalizeEmbeddingValue(item)
			if err != nil {
				return nil, true, err
			}
			inputs = append(inputs, embeddingInputMeta{
				Index:      idx,
				Normalized: normalized,
				Raw:        item,
			})
		}
		return inputs, true, nil
	case map[string]interface{}, []string, []float64, []int:
		normalized, err := normalizeEmbeddingValue(v)
		if err != nil {
			return nil, false, err
		}
		return []embeddingInputMeta{
			{Index: 0, Normalized: normalized, Raw: v},
		}, false, nil
	default:
		normalized, err := normalizeEmbeddingValue(v)
		if err != nil {
			return nil, false, err
		}
		return []embeddingInputMeta{
			{Index: 0, Normalized: normalized, Raw: v},
		}, false, nil
	}
}

func normalizeEmbeddingValue(value interface{}) (string, error) {
	switch val := value.(type) {
	case string:
		return val, nil
	case json.Number:
		return val.String(), nil
	default:
		data, err := json.Marshal(val)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}

func buildMissInputPayload(misses []embeddingInputMeta, originalWasArray bool) interface{} {
	if !originalWasArray && len(misses) == 1 {
		return misses[0].Raw
	}
	values := make([]interface{}, 0, len(misses))
	for _, miss := range misses {
		values = append(values, miss.Raw)
	}
	return values
}

func marshalEmbeddingResponseFromRecords(model string, inputs []embeddingInputMeta, hits map[int]*db.EmbeddingRecord) ([]byte, error) {
	data := make([]embeddingResponseDatum, 0, len(inputs))
	totalTokens := 0
	dataObject := "embedding"
	for idx := range inputs {
		rec, ok := hits[idx]
		if !ok {
			continue
		}
		data = append(data, embeddingResponseDatum{
			Object:    dataObject,
			Index:     idx,
			Embedding: rec.Embedding,
		})
		if rec.TokenCount != nil {
			totalTokens += *rec.TokenCount
		}
	}
	response := embeddingAPIResponse{
		Object: "list",
		Data:   data,
		Model:  model,
	}
	if totalTokens > 0 {
		response.Usage = &embeddingUsage{
			TotalTokens: &totalTokens,
		}
	}
	return json.Marshal(response)
}

func (h *Handler) handleEmbeddingCachePostResponse(resp *http.Response, meta *embeddingCacheMetadata) error {
	if resp == nil || resp.Request == nil || meta == nil {
		return nil
	}

	if resp.StatusCode != http.StatusOK || resp.Body == nil {
		resp.Header.Set("X-Embedding-Cache", "MISS")
		return nil
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Warn("Failed to read embedding response body",
			zap.String("requestId", meta.requestID),
			zap.Error(err))
		return nil
	}
	_ = resp.Body.Close()

	decompressed := bodyBytes
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" {
		if strings.Contains(strings.ToLower(encoding), "gzip") {
			gr, err := gzip.NewReader(bytes.NewReader(bodyBytes))
			if err != nil {
				logger.Warn("Failed to decompress embedding response",
					zap.String("requestId", meta.requestID),
					zap.Error(err))
				return nil
			}
			var buf bytes.Buffer
			if _, err := io.Copy(&buf, gr); err != nil {
				logger.Warn("Failed to copy decompressed embedding response",
					zap.String("requestId", meta.requestID),
					zap.Error(err))
				_ = gr.Close()
				return nil
			}
			_ = gr.Close()
			decompressed = buf.Bytes()
			resp.Header.Del("Content-Encoding")
		}
	}

	var payload embeddingAPIResponse
	if err := json.Unmarshal(decompressed, &payload); err != nil {
		logger.Warn("Failed to parse embedding response payload",
			zap.String("requestId", meta.requestID),
			zap.Error(err))
		return nil
	}

	endTime := time.Now()
	newRecords := make(map[int]*db.EmbeddingRecord)
	for idx, item := range payload.Data {
		metaInput, ok := meta.missRequestIndex[idx]
		if !ok && idx < len(meta.missOrder) {
			metaInput = meta.missOrder[idx]
		}
		if metaInput.Normalized == "" && item.Index < len(meta.missOrder) {
			metaInput = meta.missOrder[item.Index]
		}
		if metaInput.Normalized == "" {
			continue
		}

		record := &db.EmbeddingRecord{
			InputText: metaInput.Normalized,
			ModelName: meta.model,
			Embedding: item.Embedding,
			RequestID: meta.requestID,
			StartTime: &meta.startTime,
			EndTime:   &endTime,
		}
		if err := h.storage.UpsertEmbedding(resp.Request.Context(), record); err != nil {
			logger.Warn("Failed to persist embedding cache entry",
				zap.String("requestId", meta.requestID),
				zap.String("model", meta.model),
				zap.Error(err))
		}
		newRecords[metaInput.Index] = record
	}

	combined := make([]embeddingResponseDatum, 0, meta.totalInputs)
	totalTokens := 0
	dataObject := meta.responseDataObject
	if dataObject == "" && len(payload.Data) > 0 {
		dataObject = payload.Data[0].Object
	}
	if dataObject == "" {
		dataObject = "embedding"
	}

	for i := 0; i < meta.totalInputs; i++ {
		var rec *db.EmbeddingRecord
		if hit, ok := meta.hits[i]; ok {
			rec = hit
		} else if miss, ok := newRecords[i]; ok {
			rec = miss
		}
		if rec == nil {
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
	if payload.Usage != nil && payload.Usage.TotalTokens != nil {
		totalTokens += *payload.Usage.TotalTokens
	}

	combinedPayload := embeddingAPIResponse{
		Object: payload.Object,
		Data:   combined,
		Model:  firstNonEmpty(payload.Model, meta.model),
		ID:     payload.ID,
	}
	if totalTokens > 0 {
		combinedPayload.Usage = &embeddingUsage{
			TotalTokens: &totalTokens,
		}
	}

	finalBytes, err := json.Marshal(combinedPayload)
	if err != nil {
		logger.Warn("Failed to marshal combined embedding payload",
			zap.String("requestId", meta.requestID),
			zap.Error(err))
		return nil
	}

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

	logger.Info("Processed embedding cache miss/partial",
		zap.String("requestId", meta.requestID),
		zap.String("model", meta.model),
		zap.String("cacheStatus", cacheStatus),
		zap.Int("hits", len(meta.hits)),
		zap.Int("misses", len(newRecords)))

	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
