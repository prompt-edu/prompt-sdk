package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSink captures events synchronously for assertions.
type fakeSink struct {
	mu     sync.Mutex
	events []Event
}

func (f *fakeSink) Record(_ context.Context, e Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
	return nil
}

func (f *fakeSink) all() []Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Event, len(f.events))
	copy(out, f.events)
	return out
}

func staticActor(c *gin.Context) (Actor, bool) {
	if c.GetBool("noActor") {
		return Actor{}, false
	}
	return Actor{ID: "u1", Name: "Ada Lovelace", Email: "ada@tum.de", Role: "Lecturer", Roles: []string{"Lecturer"}}, true
}

// run executes a single request through a router with the audit middleware and
// returns the events the sink saw. The middleware writes in a background
// goroutine, so we wait on a WaitGroup toggled by a wrapper sink.
func run(t *testing.T, sink Sink, method, path string, register func(r *gin.Engine)) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware(sink, WithActorExtractor(staticActor), WithSourceService("core")))
	register(r)

	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
}

// waitSink blocks in Record until released, letting tests deterministically
// observe the async write.
type waitSink struct {
	*fakeSink
	wg *sync.WaitGroup
}

func (s waitSink) Record(ctx context.Context, e Event) error {
	defer s.wg.Done()
	return s.fakeSink.Record(ctx, e)
}

func newWaitSink(n int) (waitSink, *sync.WaitGroup) {
	wg := &sync.WaitGroup{}
	wg.Add(n)
	return waitSink{fakeSink: &fakeSink{}, wg: wg}, wg
}

// waitOrFail waits for the expected async deliveries, failing the test instead
// of hanging if they never arrive.
func waitOrFail(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for audit event delivery")
	}
}

func TestMiddleware_LogsSuccessfulMutation(t *testing.T) {
	sink, wg := newWaitSink(1)
	run(t, sink, http.MethodPost, "/api/course_phase/p1/slots", func(r *gin.Engine) {
		r.POST("/api/course_phase/:coursePhaseID/slots", func(c *gin.Context) { c.Status(http.StatusCreated) })
	})
	waitOrFail(t, wg)

	events := sink.all()
	require.Len(t, events, 1)
	e := events[0]
	assert.Equal(t, OutcomeSuccess, e.Outcome)
	assert.Equal(t, "Created slot", e.Action)
	assert.Equal(t, "POST /api/course_phase/:coursePhaseID/slots", e.ActionKey)
	assert.Equal(t, "p1", e.CoursePhaseID)
	assert.Equal(t, "u1", e.ActorID)
	assert.Equal(t, "Lecturer", e.ActorRole)
	assert.Equal(t, "core", e.SourceService)
}

func TestMiddleware_LogsDeniedAs403(t *testing.T) {
	sink, wg := newWaitSink(1)
	run(t, sink, http.MethodDelete, "/api/teams/t1", func(r *gin.Engine) {
		r.DELETE("/api/teams/:uuid", func(c *gin.Context) { c.Status(http.StatusForbidden) })
	})
	waitOrFail(t, wg)

	events := sink.all()
	require.Len(t, events, 1)
	assert.Equal(t, OutcomeDenied, events[0].Outcome)
	assert.Equal(t, "t1", events[0].EntityID)
	assert.Equal(t, "Deleted team", events[0].Action)
}

func TestMiddleware_SkipsReadsAndNoise(t *testing.T) {
	for _, tc := range []struct {
		name, method string
		status       int
		noActor      bool
	}{
		{"GET read", http.MethodGet, http.StatusOK, false},
		// A real unauthenticated 401 has no resolvable actor, so it is skipped;
		// the actor gate is what filters token-invalid noise from genuine denials.
		{"401 unauth", http.MethodPost, http.StatusUnauthorized, true},
		{"404 not found", http.MethodPost, http.StatusNotFound, false},
		{"422 validation", http.MethodPost, http.StatusUnprocessableEntity, false},
		{"no actor", http.MethodPost, http.StatusCreated, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink := &fakeSink{}
			run(t, sink, tc.method, "/api/things", func(r *gin.Engine) {
				r.Handle(tc.method, "/api/things", func(c *gin.Context) {
					c.Set("noActor", tc.noActor)
					c.Status(tc.status)
				})
			})
			assert.Empty(t, sink.all())
		})
	}
}

func TestMiddleware_Logs401DenialWithActor(t *testing.T) {
	// The SDK auth middleware aborts authorization denials with 401 (not 403);
	// a 401 that still has a resolvable actor is a genuine denied attempt.
	sink, wg := newWaitSink(1)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware(sink, WithActorExtractor(staticActor), WithSourceService("core")))
	authz := func(c *gin.Context) { c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "denied"}) }
	r.POST("/api/course_phase/:coursePhaseID/grades", authz, func(c *gin.Context) { c.Status(http.StatusOK) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/course_phase/p1/grades", nil))
	waitOrFail(t, wg)

	events := sink.all()
	require.Len(t, events, 1)
	assert.Equal(t, OutcomeDenied, events[0].Outcome)
	assert.Equal(t, "u1", events[0].ActorID)
}

func TestRecord_UsesDescribeLabelWhenActionBlank(t *testing.T) {
	sink, wg := newWaitSink(1)
	run(t, sink, http.MethodPost, "/api/courses/c1/publish", func(r *gin.Engine) {
		r.POST("/api/courses/:courseId/publish", Describe("Published grades"), func(c *gin.Context) {
			Record(c, Event{EntityType: "grade", EntityID: "g1"}) // no Action
			c.Status(http.StatusOK)
		})
	})
	waitOrFail(t, wg)

	events := sink.all()
	require.Len(t, events, 1)
	assert.Equal(t, "Published grades", events[0].Action) // Describe label, not blank
}

func TestRecord_StampsRealStatusAndErrorOutcome(t *testing.T) {
	// "record, then do the work" where the work then fails: the buffered event
	// must carry the real 500 status and an error outcome, not a premature success.
	sink, wg := newWaitSink(1)
	run(t, sink, http.MethodPost, "/api/grades", func(r *gin.Engine) {
		r.POST("/api/grades", func(c *gin.Context) {
			Record(c, Event{Action: "Published grades"})
			c.Status(http.StatusInternalServerError)
		})
	})
	waitOrFail(t, wg)

	events := sink.all()
	require.Len(t, events, 1)
	assert.Equal(t, OutcomeError, events[0].Outcome)
	assert.Equal(t, http.StatusInternalServerError, events[0].HTTPStatus)
}

func TestPrimaryRole_MatchesCoursePrefixedRoles(t *testing.T) {
	// Course roles arrive course-prefixed and must match by suffix; platform
	// roles match exactly and win over course roles.
	assert.Equal(t, "Lecturer", primaryRole(map[string]bool{"ios24-Lecturer": true}))
	assert.Equal(t, "Editor", primaryRole(map[string]bool{"ios24-Editor": true}))
	assert.Equal(t, "PROMPT_Admin", primaryRole(map[string]bool{"PROMPT_Admin": true, "ios24-Lecturer": true}))
	// PROMPT_Lecturer must not be misread as a course Lecturer by the suffix match.
	assert.Equal(t, "PROMPT_Lecturer", primaryRole(map[string]bool{"PROMPT_Lecturer": true}))
	// Unknown roles fall back deterministically to the smallest.
	assert.Equal(t, "aaa", primaryRole(map[string]bool{"zzz": true, "aaa": true}))
}

func TestMiddleware_DescribeOverridesLabel(t *testing.T) {
	sink, wg := newWaitSink(1)
	run(t, sink, http.MethodPost, "/api/courses/c1/copy", func(r *gin.Engine) {
		r.POST("/api/courses/:courseId/copy", Describe("Copied course"), func(c *gin.Context) { c.Status(http.StatusOK) })
	})
	waitOrFail(t, wg)

	events := sink.all()
	require.Len(t, events, 1)
	assert.Equal(t, "Copied course", events[0].Action)
	assert.Equal(t, "c1", events[0].CourseID)
}

func TestMiddleware_SkipPreventsEntry(t *testing.T) {
	sink := &fakeSink{}
	run(t, sink, http.MethodPost, "/api/sync/pull", func(r *gin.Engine) {
		r.POST("/api/sync/pull", Skip(), func(c *gin.Context) { c.Status(http.StatusOK) })
	})
	assert.Empty(t, sink.all())
}

func TestMiddleware_SuppressPreventsEntry(t *testing.T) {
	sink := &fakeSink{}
	run(t, sink, http.MethodPost, "/api/noop", func(r *gin.Engine) {
		r.POST("/api/noop", func(c *gin.Context) {
			Suppress(c)
			c.Status(http.StatusOK)
		})
	})
	assert.Empty(t, sink.all())
}

func TestRecord_AutoSuppressesBackstop(t *testing.T) {
	sink, wg := newWaitSink(1)
	run(t, sink, http.MethodPost, "/api/grades", func(r *gin.Engine) {
		r.POST("/api/grades", func(c *gin.Context) {
			Record(c, Event{Action: "Published grades", EntityType: "grade", EntityID: "g1", EntityName: "team Alpha"})
			c.Status(http.StatusOK)
		})
	})
	waitOrFail(t, wg)

	events := sink.all()
	require.Len(t, events, 1) // exactly one: the explicit Record, not the backstop
	assert.Equal(t, "Published grades", events[0].Action)
	assert.Equal(t, "team Alpha", events[0].EntityName)
	assert.Equal(t, "u1", events[0].ActorID) // actor filled from extractor
}

func TestRecord_StillWritesWhenSuppressed(t *testing.T) {
	sink, wg := newWaitSink(1)
	run(t, sink, http.MethodPost, "/api/thing", func(r *gin.Engine) {
		r.POST("/api/thing", func(c *gin.Context) {
			Suppress(c) // silences the backstop
			Record(c, Event{Action: "Did the thing on purpose"})
			c.Status(http.StatusOK)
		})
	})
	waitOrFail(t, wg)

	events := sink.all()
	require.Len(t, events, 1)
	assert.Equal(t, "Did the thing on purpose", events[0].Action)
}

func TestMiddleware_CapturesDenialFromAbortingMiddleware(t *testing.T) {
	sink, wg := newWaitSink(1)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware(sink, WithActorExtractor(staticActor), WithSourceService("core")))
	// A downstream authorization middleware that aborts before the handler runs.
	authz := func(c *gin.Context) { c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "denied"}) }
	r.POST("/api/course_phase/:coursePhaseID/grades", authz, func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/api/course_phase/p1/grades", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)
	waitOrFail(t, wg)

	events := sink.all()
	require.Len(t, events, 1)
	assert.Equal(t, OutcomeDenied, events[0].Outcome)
	assert.Equal(t, "u1", events[0].ActorID)
}

func TestRecord_SkipsWhenNoActor(t *testing.T) {
	sink := &fakeSink{}
	run(t, sink, http.MethodPost, "/api/thing", func(r *gin.Engine) {
		r.POST("/api/thing", func(c *gin.Context) {
			c.Set("noActor", true)
			Record(c, Event{Action: "Orphan event"}) // no actor -> must not write a blank-actor entry
			c.Status(http.StatusOK)
		})
	})
	assert.Empty(t, sink.all())
}

func TestMiddleware_NilSinkIsNoOp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware(nil))
	called := false
	r.POST("/api/thing", func(c *gin.Context) { called = true; c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/api/thing", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)
	assert.True(t, called)
}

func TestEnabled(t *testing.T) {
	t.Setenv("AUDIT_ENABLED", "true")
	assert.True(t, Enabled())
	t.Setenv("AUDIT_ENABLED", "")
	assert.False(t, Enabled())
}

func TestNewCoreSink_DisabledReturnsNil(t *testing.T) {
	t.Setenv("AUDIT_ENABLED", "true")
	t.Setenv("AUDIT_INGEST_KEY", "")
	assert.Nil(t, NewCoreSink("http://core", "interview"))

	t.Setenv("AUDIT_ENABLED", "")
	t.Setenv("AUDIT_INGEST_KEY", "secret")
	assert.Nil(t, NewCoreSink("http://core", "interview"))
}

func TestNewCoreSink_SendsHeaders(t *testing.T) {
	t.Setenv("AUDIT_ENABLED", "true")
	t.Setenv("AUDIT_INGEST_KEY", "secret")

	var gotService, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotService = r.Header.Get("X-Audit-Service")
		gotToken = r.Header.Get("X-Audit-Token")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	sink := NewCoreSink(srv.URL, "interview")
	require.NotNil(t, sink)
	require.NoError(t, sink.Record(context.Background(), Event{Action: "Created slot"}))
	assert.Equal(t, "interview", gotService)
	assert.Equal(t, "secret", gotToken)
}

func TestNewCoreSink_JoinsPathWithoutDoubleSlash(t *testing.T) {
	t.Setenv("AUDIT_ENABLED", "true")
	t.Setenv("AUDIT_INGEST_KEY", "secret")
	s, ok := NewCoreSink("http://core:8080/", "interview").(*coreSink)
	require.True(t, ok)
	assert.Equal(t, "http://core:8080/api/audit", s.url) // trailing slash collapsed
}

func TestCoreSink_RetryPolicy(t *testing.T) {
	t.Setenv("AUDIT_ENABLED", "true")
	t.Setenv("AUDIT_INGEST_KEY", "secret")
	for _, tc := range []struct {
		name         string
		status       int
		wantAttempts int32
		wantErr      bool
	}{
		{"2xx succeeds", http.StatusNoContent, 1, false},
		{"4xx fails fast", http.StatusUnauthorized, 1, true},
		{"3xx is not success and fails fast", http.StatusFound, 1, true},
		{"5xx retries", http.StatusInternalServerError, 3, true},
		{"429 retries", http.StatusTooManyRequests, 3, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var attempts int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&attempts, 1)
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			sink := NewCoreSink(srv.URL, "interview")
			require.NotNil(t, sink)
			err := sink.Record(context.Background(), Event{Action: "x"})
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.wantAttempts, atomic.LoadInt32(&attempts))
		})
	}
}

func TestInsecureExternalURL(t *testing.T) {
	for in, want := range map[string]bool{
		"https://core.example.com": false, // TLS
		"http://server-core:8080":  false, // internal docker service name
		"http://localhost:8080":    false,
		"http://127.0.0.1:8080":    false,
		"http://10.0.0.5:8080":     false, // private IP
		"http://192.168.1.10":      false, // private IP
		"http://core.example.com":  true,  // public FQDN over plaintext
		"http://8.8.8.8:8080":      true,  // public IP over plaintext
		"server-core:8080":         true,  // scheme-less: parses with no host, warn
		"":                         true,  // empty: malformed config, warn
	} {
		assert.Equal(t, want, insecureExternalURL(in), in)
	}
}

func TestDeriveAction(t *testing.T) {
	assert.Equal(t, "Created slot", deriveAction("POST", "/api/course_phase/:id/slots"))
	assert.Equal(t, "Updated participation", deriveAction("PATCH", "/api/.../participations"))
	assert.Equal(t, "Deleted team", deriveAction("DELETE", "/api/teams/:uuid"))
}

func TestSingularize(t *testing.T) {
	for in, want := range map[string]string{
		"slots":          "slot",
		"teams":          "team",
		"participations": "participation",
		"status":         "status",   // -us: keep
		"campus":         "campus",   // -us: keep
		"analysis":       "analysis", // -is: keep
		"class":          "class",    // -ss: keep
	} {
		assert.Equal(t, want, singularize(in), in)
	}
}
