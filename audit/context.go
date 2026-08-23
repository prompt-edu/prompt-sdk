package audit

import (
	"context"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier"
)

const (
	runtimeContextKey  = "auditRuntime"
	actionContextKey   = "auditAction"
	skipContextKey     = "auditSkip"
	recordedContextKey = "auditRecorded"
	// recordedEventsKey holds the []Event buffered by in-request Record calls,
	// flushed by the middleware after the handler returns (see flushRecordedEvents).
	recordedEventsKey = "auditRecordedEvents"
)

// Actor identifies the human who performed an action.
type Actor struct {
	ID    string
	Name  string
	Email string
	Role  string
	Roles []string
}

// ActorExtractor reads the authenticated actor from the request context. The
// default reads the SDK TokenUser; core injects one reading its flat context
// keys via WithActorExtractor.
type ActorExtractor func(c *gin.Context) (Actor, bool)

// maxConcurrentDeliveries bounds how many audit events may be in flight to the
// sink at once. Delivery is best-effort: when the limit is reached (e.g. a slow
// or unreachable core, whose HTTP sink retries for ~30s per event), further
// events are dropped with a logged warning rather than spawning unbounded
// goroutines.
const maxConcurrentDeliveries = 64

// runtime holds the per-service audit configuration, stashed in the gin context
// by Middleware so Describe/Record can reach the sink.
type runtime struct {
	sink      Sink
	extractor ActorExtractor
	source    string
	sem       chan struct{}
}

func (rt *runtime) write(e Event) {
	if e.SourceService == "" {
		e.SourceService = rt.source
	}
	select {
	case rt.sem <- struct{}{}:
		go func() {
			defer func() { <-rt.sem }()
			if err := rt.sink.Record(context.Background(), e); err != nil {
				log.WithError(err).Error("audit: failed to record event")
			}
		}()
	default:
		// Best-effort delivery: when the in-flight limit is reached (e.g. core is
		// unreachable and every slot is blocked on retries) the event is dropped.
		// Log the action and actor so the gap is at least reconstructible.
		log.WithFields(log.Fields{"action": e.Action, "actorID": e.ActorID}).
			Warn("audit: delivery concurrency limit reached, dropping audit event")
	}
}

func runtimeFrom(c *gin.Context) (*runtime, bool) {
	v, ok := c.Get(runtimeContextKey)
	if !ok {
		return nil, false
	}
	rt, ok := v.(*runtime)
	return rt, ok
}

// Describe returns a route-scoped middleware that names the action for the
// audited request, e.g. router.POST("/publish", audit.Describe("Published
// grades"), handler). One line, declarative; the middleware emits a single
// entry using this label.
func Describe(label string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(actionContextKey, label)
	}
}

// Skip returns a route- or group-scoped middleware that excludes matching
// requests from audit logging, e.g. api.Group("/sync", audit.Skip()).
func Skip() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(skipContextKey, true)
	}
}

// Suppress excludes the current request from automatic audit logging. Call it
// from a handler when a particular request should not be recorded (e.g. an
// update that changed nothing).
func Suppress(c *gin.Context) {
	c.Set(skipContextKey, true)
}

// Record emits a fully-specified audit event from within a handler. It fills in
// the actor, action label, source service and HTTP context when left blank, and
// suppresses the middleware's automatic backstop entry for this request so there
// is no duplicate. Use it for high-stakes actions that need an EntityID/
// EntityName or before/after Metadata, or for multiple events per request.
//
// Outcome/status handling: when Outcome is left blank (the common "record, then
// do the work" case) the event is buffered and flushed by the middleware after
// the handler returns, so it carries the request's real final status and outcome
// rather than a premature success. When Outcome is set explicitly — e.g. a
// background job that has already completed and reports its own result — the
// event is delivered immediately.
//
// For background work, do NOT hand this the live *gin.Context: gin recycles it
// into a pool once the handler returns. Pass c.Copy() (and set the actor fields
// and Outcome explicitly). Best-effort; for atomic guarantees use the core
// RecordTx helper.
func Record(c *gin.Context, e Event) {
	rt, ok := runtimeFrom(c)
	if !ok {
		return
	}

	// Every audited action must trace to a human. If no actor is supplied and
	// none can be extracted, do not record — and do not suppress the automatic
	// backstop, so a later-resolvable actor can still be captured.
	if e.ActorID == "" {
		actor, ok := rt.extractor(c)
		if !ok {
			return
		}
		applyActor(&e, actor)
	}
	c.Set(recordedContextKey, true)

	if e.HTTPMethod == "" {
		e.HTTPMethod = c.Request.Method
	}
	if e.HTTPPath == "" {
		e.HTTPPath = c.FullPath()
	}
	if e.ActionKey == "" {
		e.ActionKey = c.Request.Method + " " + c.FullPath()
	}
	if e.Action == "" {
		e.Action = actionLabel(c, c.Request.Method, c.FullPath())
	}
	// Marshalling happens asynchronously; snapshot the caller's map so a handler
	// that keeps mutating Metadata after Record does not race the delivery.
	if e.Metadata != nil {
		e.Metadata = cloneMetadata(e.Metadata)
	}

	// Explicit outcome => deliver now (background/final event). No outcome yet =>
	// buffer and let the middleware stamp the real status/outcome after c.Next().
	if e.Outcome != "" {
		rt.write(e)
		return
	}
	bufferEvent(c, e)
}

// bufferEvent appends an in-request event to the per-request buffer flushed by
// the middleware after the handler returns.
func bufferEvent(c *gin.Context, e Event) {
	var buf []Event
	if v, ok := c.Get(recordedEventsKey); ok {
		if existing, ok := v.([]Event); ok {
			buf = existing
		}
	}
	c.Set(recordedEventsKey, append(buf, e))
}

// actionLabel returns the explicit Describe label if one was set for the route,
// otherwise the derived default. Shared by Record and the middleware backstop.
func actionLabel(c *gin.Context, method, routeTemplate string) string {
	if label, ok := c.Get(actionContextKey); ok {
		if s, ok := label.(string); ok && s != "" {
			return s
		}
	}
	return deriveAction(method, routeTemplate)
}

func cloneMetadata(m map[string]any) map[string]any {
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

func applyActor(e *Event, actor Actor) {
	e.ActorID = actor.ID
	e.ActorName = actor.Name
	e.ActorEmail = actor.Email
	e.ActorRole = actor.Role
	e.ActorRoles = actor.Roles
}

// defaultActorExtractor reads the SDK TokenUser populated by the authentication
// middleware. Used by phase services.
func defaultActorExtractor(c *gin.Context) (Actor, bool) {
	tu, ok := keycloakTokenVerifier.GetTokenUser(c)
	if !ok || tu.ID == "" {
		return Actor{}, false
	}
	roles := make([]string, 0, len(tu.Roles))
	for role := range tu.Roles {
		roles = append(roles, role)
	}
	sort.Strings(roles) // stable order (map iteration is randomized)
	return Actor{
		ID:    tu.ID,
		Name:  strings.TrimSpace(tu.FirstName + " " + tu.LastName),
		Email: tu.Email,
		Role:  primaryRole(tu.Roles),
		Roles: roles,
	}, true
}

// primaryRole picks the highest-privilege role present, used as the filterable
// actor_role. The platform roles are matched exactly, but course roles arrive
// course-prefixed (e.g. "ios24-Lecturer"), so they are matched by suffix — the
// same scheme the SDK auth middleware uses.
func primaryRole(roles map[string]bool) string {
	if roles[keycloakTokenVerifier.PromptAdmin] {
		return keycloakTokenVerifier.PromptAdmin
	}
	if roles[keycloakTokenVerifier.PromptLecturer] {
		return keycloakTokenVerifier.PromptLecturer
	}
	for _, courseRole := range []string{
		keycloakTokenVerifier.CourseLecturer,
		keycloakTokenVerifier.CourseEditor,
		keycloakTokenVerifier.CourseStudent,
	} {
		for userRole := range roles {
			if strings.HasSuffix(userRole, courseRole) {
				return courseRole
			}
		}
	}
	// No known role: pick the lexicographically smallest so the value is stable
	// across requests (map iteration order is randomized).
	fallback := ""
	for role := range roles {
		if fallback == "" || role < fallback {
			fallback = role
		}
	}
	return fallback
}
