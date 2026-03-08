package utils

import "github.com/gin-gonic/gin"

// HandleError is a utility function that sends a JSON error response with the specified status code.
func HandleError(c *gin.Context, statusCode int, err error) {
	c.JSON(statusCode, gin.H{"error": err.Error()})
}
