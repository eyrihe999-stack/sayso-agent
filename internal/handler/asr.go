package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"sayso-agent/internal/model"
	"sayso-agent/internal/service"
)

// ASRHandler 处理 ASR 相关 HTTP 请求
type ASRHandler struct {
	asrService *service.ASRService
}

// NewASRHandler 创建 ASR 处理器
func NewASRHandler(svc *service.ASRService) *ASRHandler {
	return &ASRHandler{asrService: svc}
}

// Process 接收内部传入的 ASR 文本并处理
// POST /api/v1/asr/process
func (h *ASRHandler) Process(c *gin.Context) {
	var req model.ASRRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	resp, err := h.asrService.Process(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"task_id": resp.TaskID,
			"error":   err.Error(),
			"result":  resp,
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}
