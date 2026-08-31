package keycloakCoreRequests

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSendIsStudentRequest_StatusMapping asserts the status-to-result mapping:
// 200 authorizes, 401/403 map to ErrNotStudentOfCourse, and any other non-200
// returns a hard error so the middleware fails closed.
func TestSendIsStudentRequest_StatusMapping(t *testing.T) {
	tests := []struct {
		name               string
		coreStatus         int
		coreBody           string
		wantErr            bool
		wantErrMsg         string
		wantNotStudent     bool
		wantStudentOfPhase bool
	}{
		{
			name:               "200 authorized",
			coreStatus:         http.StatusOK,
			coreBody:           `{"isStudentOfCoursePhase":true,"courseParticipationID":"00000000-0000-0000-0000-000000000001"}`,
			wantErr:            false,
			wantStudentOfPhase: true,
		},
		{
			name:           "401 unauthenticated fails closed",
			coreStatus:     http.StatusUnauthorized,
			wantErr:        true,
			wantErrMsg:     "not student of course",
			wantNotStudent: true,
		},
		{
			// Regression guard for GHSA-4wgm-2wvm-fphm: core denies cross-course
			// access with 403, which must map to ErrNotStudentOfCourse.
			name:           "403 cross-course fails closed",
			coreStatus:     http.StatusForbidden,
			coreBody:       `{"error":"no matching permission found"}`,
			wantErr:        true,
			wantErrMsg:     "not student of course",
			wantNotStudent: true,
		},
		{
			name:       "500 unexpected returns hard error",
			coreStatus: http.StatusInternalServerError,
			wantErr:    true,
			wantErrMsg: "unexpected core response",
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

			resp, err := SendIsStudentRequest(*coreURL, "Bearer token", uuid.New())

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				assert.Equal(t, tt.wantNotStudent, errors.Is(err, ErrNotStudentOfCourse))
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantStudentOfPhase, resp.IsStudentOfCoursePhase)
		})
	}
}
