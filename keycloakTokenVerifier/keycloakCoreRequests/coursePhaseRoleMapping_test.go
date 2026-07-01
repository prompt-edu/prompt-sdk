package keycloakCoreRequests

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendCoursePhaseRoleMappingRequest_StatusMapping(t *testing.T) {
	tests := []struct {
		name        string
		coreStatus  int
		coreBody    string
		wantErr     bool
		wantPrefix  string
	}{
		{
			name:       "200 returns role mapping",
			coreStatus: http.StatusOK,
			coreBody:   `{"courseLecturerRole":"WS24-Test-Lecturer","courseEditorRole":"WS24-Test-Editor","customRolePrefix":"WS24-Test-cg-"}`,
			wantErr:    false,
			wantPrefix: "WS24-Test-cg-",
		},
		{
			// Fail closed: a 403 must not yield an empty mapping (empty CustomRolePrefix
			// would let the auth middleware match un-prefixed custom roles).
			name:       "403 fails closed",
			coreStatus: http.StatusForbidden,
			wantErr:    true,
		},
		{
			name:       "500 fails closed",
			coreStatus: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.coreStatus)
				if tt.coreBody != "" {
					_, _ = w.Write([]byte(tt.coreBody))
				}
			}))
			defer server.Close()

			coreURL, err := url.Parse(server.URL)
			require.NoError(t, err)

			resp, err := SendCoursePhaseRoleMappingRequest(*coreURL, "Bearer token", uuid.New())

			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, resp.CustomRolePrefix)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantPrefix, resp.CustomRolePrefix)
			}
		})
	}
}
