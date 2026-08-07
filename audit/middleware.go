package audit

import "github.com/gin-gonic/gin"

// Option configures the audit middleware.
type Option func(*runtime)

// WithActorExtractor overrides how the actor is read from the request context.
// Core passes an extractor that reads its flat context keys; phases use the
// default (SDK TokenUser).
func WithActorExtractor(extractor ActorExtractor) Option {
	return func(rt *runtime) {
		if extractor != nil {
			rt.extractor = extractor
		}
	}
}

// WithSourceService sets the source_service stamped on locally-persisted events
// (core uses "core"). For phases shipping over HTTP, core overrides it from the
// authenticated service identity, so it may be left empty.
func WithSourceService(name string) Option {
	return func(rt *runtime) {
		rt.source = name
	}
}

// Middleware returns a Gin middleware that automatically records mutating
// requests through sink. It must run after the route's auth middleware. A
// request is recorded only when it uses a mutating method (POST/PUT/PATCH/
// DELETE), resolves to a known actor, is not suppressed, and either succeeded
// (2xx -> success) or was denied (403 -> denied); other statuses are ignored.
// Passing a nil sink returns a pass-through no-op, so a service can register the
// middleware unconditionally and let the feature toggle decide.
func Middleware(sink Sink, opts ...Option) gin.HandlerFunc {
	if sink == nil {
		return func(c *gin.Context) { c.Next() }
	}
	rt := &runtime{
		sink:      sink,
		extractor: defaultActorExtractor,
		sem:       make(chan struct{}, maxConcurrentDeliveries),
	}
	for _, opt := range opts {
		opt(rt)
	}

	return func(c *gin.Context) {
		c.Set(runtimeContextKey, rt)
		c.Next()

		if c.GetBool(skipContextKey) || c.GetBool(recordedContextKey) {
			return
		}
		outcome, ok := outcomeForStatus(c.Writer.Status())
		if !ok || !isMutating(c.Request.Method) {
			return
		}
		actor, ok := rt.extractor(c)
		if !ok {
			return
		}
		rt.write(buildEvent(c, actor, outcome))
	}
}

func outcomeForStatus(status int) (string, bool) {
	switch {
	case status >= 200 && status < 300:
		return OutcomeSuccess, true
	case status == 403:
		return OutcomeDenied, true
	default:
		return "", false
	}
}

func buildEvent(c *gin.Context, actor Actor, outcome string) Event {
	routeTemplate := c.FullPath()
	action := deriveAction(c.Request.Method, routeTemplate)
	if label, ok := c.Get(actionContextKey); ok {
		if s, ok := label.(string); ok && s != "" {
			action = s
		}
	}

	e := Event{
		Action:        action,
		ActionKey:     c.Request.Method + " " + routeTemplate,
		Outcome:       outcome,
		CourseID:      firstParam(c, "courseId", "courseID"),
		CoursePhaseID: c.Param("coursePhaseID"),
		EntityID:      firstParam(c, "uuid", "id"),
		HTTPMethod:    c.Request.Method,
		HTTPPath:      routeTemplate,
		HTTPStatus:    c.Writer.Status(),
	}
	applyActor(&e, actor)
	return e
}

func firstParam(c *gin.Context, names ...string) string {
	for _, name := range names {
		if v := c.Param(name); v != "" {
			return v
		}
	}
	return ""
}
