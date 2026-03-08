package promptTypes

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type StudentDeletionHandler interface {
	HandleDeleteStudentData(c *gin.Context, req StudentIdentifyingRequest) error
}

func RegisterStudentDeletionEndpoint(router *gin.RouterGroup, authMiddleware gin.HandlerFunc, handler StudentDeletionHandler) {
	router.POST(PrivacyRouteStudentDataDeletion, authMiddleware, func(c *gin.Context) {
		var req StudentIdentifyingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := handler.HandleDeleteStudentData(c, req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": "Student data deletion request executed"})
	})
}
