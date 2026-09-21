package promptSDK

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func validServiceOptions() ServiceOptions {
	return ServiceOptions{
		ServiceName:    "assessment",
		BasePath:       "/assessment/api",
		DBEnvPrefix:    "ASSESSMENT",
		DefaultDBPort:  "5435",
		DefaultAddress: "localhost:8085",
		RegisterRoutes: func(api, coursePhase *gin.RouterGroup, conn *pgxpool.Pool) error { return nil },
	}
}

func TestServiceOptionsValidate(t *testing.T) {
	require.NoError(t, validServiceOptions().validate())

	for _, tc := range []struct {
		field  string
		mutate func(*ServiceOptions)
	}{
		{"ServiceName", func(o *ServiceOptions) { o.ServiceName = "" }},
		{"BasePath", func(o *ServiceOptions) { o.BasePath = "" }},
		{"DBEnvPrefix", func(o *ServiceOptions) { o.DBEnvPrefix = "" }},
		{"DefaultDBPort", func(o *ServiceOptions) { o.DefaultDBPort = "" }},
		{"DefaultAddress", func(o *ServiceOptions) { o.DefaultAddress = "" }},
		{"RegisterRoutes", func(o *ServiceOptions) { o.RegisterRoutes = nil }},
	} {
		opts := validServiceOptions()
		tc.mutate(&opts)
		require.ErrorContains(t, opts.validate(), tc.field, "a missing %s must fail startup", tc.field)
	}
}
