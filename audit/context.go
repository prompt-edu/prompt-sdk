package audit

import (
	"context"
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
		log.Warn("audit: delivery concurrency limit reached, dropping audit event")
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
// the actor, source service and HTTP context when left blank, and suppresses
// the middleware's automatic backstop entry for this request so there is no
// duplicate. Use it for high-stakes actions that need an EntityID/EntityName or
// before/after Metadata, for multiple events per request, or for background
// work (pass the initiating human's actor fields explicitly). Best-effort; for
// atomic guarantees use the core RecordTx helper.
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

	if e.Outcome == "" {
		e.Outcome = OutcomeSuccess
	}
	if e.HTTPMethod == "" {
		e.HTTPMethod = c.Request.Method
	}
	if e.HTTPPath == "" {
		e.HTTPPath = c.FullPath()
	}
	if e.ActionKey == "" {
		e.ActionKey = c.Request.Method + " " + c.FullPath()
	}
	rt.write(e)
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
	return Actor{
		ID:    tu.ID,
		Name:  strings.TrimSpace(tu.FirstName + " " + tu.LastName),
		Email: tu.Email,
		Role:  primaryRole(tu.Roles),
		Roles: roles,
	}, true
}

// primaryRole picks the highest-privilege role present, used as the filterable
// actor_role.
func primaryRole(roles map[string]bool) string {
	for _, role := range []string{
		keycloakTokenVerifier.PromptAdmin,
		keycloakTokenVerifier.PromptLecturer,
		keycloakTokenVerifier.CourseLecturer,
		keycloakTokenVerifier.CourseEditor,
		keycloakTokenVerifier.CourseStudent,
	} {
		if roles[role] {
			return role
		}
	}
	for role := range roles {
		return role
	}
	return ""
}
