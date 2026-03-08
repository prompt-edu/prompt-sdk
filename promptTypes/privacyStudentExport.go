package promptTypes

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type StudentExportResult struct {
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

type StudentExportHandler interface {
	HandleExportStudentData(c *gin.Context, req StudentIdentifyingRequest) (StudentExportResult, error)
}

func RegisterStudentExportEndpoint(router *gin.RouterGroup, authMiddleware gin.HandlerFunc, handler StudentExportHandler) {
	router.POST(PrivacyRouteStudentDataExport, authMiddleware, func(c *gin.Context) {
		var req StudentIdentifyingRequest
		if errRead := c.ShouldBindJSON(&req); errRead != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": errRead.Error()})
			return
		}

		res, errExport := handler.HandleExportStudentData(c, req)
		if errExport != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": errExport.Error()})
			return
		}

		c.JSON(http.StatusOK, res)
	})
}
