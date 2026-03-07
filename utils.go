package promptSDK

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/prompt-edu/prompt-sdk/utils"
)

func CORSMiddleware(clientHost string) gin.HandlerFunc {
	return utils.CORS(clientHost)
}

func DeferDBRollback(tx pgx.Tx, ctx context.Context) {
	utils.DeferRollback(tx, ctx)
}

func GetEnv(key, defaultValue string) string {
	return utils.GetEnv(key, defaultValue)
}

func FetchJSON(url, authHeader string) ([]byte, error) {
	return utils.FetchJSON(url, authHeader)
}
