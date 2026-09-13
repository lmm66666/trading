package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"trading/business"
)

// StockHandler 股票数据 HTTP 处理器
type StockHandler struct {
	svc                business.StockDataService
	financialSvc       business.FinancialReportService
	scheduler          business.Scheduler
	financialScheduler business.FinancialScheduler
	signalSvc          business.SignalService
	querySvc           business.QueryService
	macroSvc           business.MacroService
	kernel             KernelServices
}

// NewStockHandler 创建 StockHandler
func NewStockHandler(svc business.StockDataService, financialSvc business.FinancialReportService, scheduler business.Scheduler, financialScheduler business.FinancialScheduler, signalSvc business.SignalService, querySvc business.QueryService, macroSvc business.MacroService) *StockHandler {
	return &StockHandler{svc: svc, financialSvc: financialSvc, scheduler: scheduler, financialScheduler: financialScheduler, signalSvc: signalSvc, querySvc: querySvc, macroSvc: macroSvc}
}

// response 统一 JSON 响应结构
type response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func respondSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, response{Code: 0, Message: "success", Data: data})
}

func respondError(c *gin.Context, status int, message string) {
	c.JSON(status, response{Code: status, Message: message, Data: nil})
}

// respondInternalError 记录详细错误到服务端日志，向客户端返回脱敏的通用 500 响应，
// 避免泄漏数据库结构、SQL 语句、内部文件路径等敏感信息。
func respondInternalError(c *gin.Context, op string, err error) {
	log.Printf("[api] %s failed: method=%s path=%s err=%v", op, c.Request.Method, c.Request.URL.Path, err)
	c.JSON(http.StatusInternalServerError, response{
		Code:    http.StatusInternalServerError,
		Message: "internal server error",
		Data:    nil,
	})
}
