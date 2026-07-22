package utils

import (
	"fmt"
	"net/http"

	"github.com/getsentry/sentry-go"
	log "github.com/sirupsen/logrus"
)

const (
	sentryLoggerName  = "logrus"
	defaultErrorDepth = 10
)

// logrusToSentryLevel maps the error-class logrus levels to Sentry levels.
var logrusToSentryLevel = map[log.Level]sentry.Level{
	log.ErrorLevel: sentry.LevelError,
	log.FatalLevel: sentry.LevelFatal,
	log.PanicLevel: sentry.LevelFatal,
}

// sentryEventHook captures error-level logrus entries as Sentry issues. It replaces
// sentrylogrus.NewEventHookFromClient (deprecated in 0.47, removed in 0.48) with an equivalent
// event builder, so error/fatal/panic logs still create issues rather than the structured logs
// NewLogHookFromClient would produce. It preserves the fields the old hook promoted (request, user,
// transaction, fingerprint), forwards remaining fields as tags, and forwards entry.Context via a
// hint so attribution, grouping, and BeforeSend keep working.
type sentryEventHook struct {
	levels []log.Level
}

func newSentryEventHook(levels []log.Level) *sentryEventHook {
	return &sentryEventHook{levels: levels}
}

func (h *sentryEventHook) Levels() []log.Level {
	return h.levels
}

func (h *sentryEventHook) Fire(entry *log.Entry) error {
	event := entryToSentryEvent(entry)

	var hint *sentry.EventHint
	if entry.Context != nil {
		hint = &sentry.EventHint{Context: entry.Context}
	}

	sentry.CurrentHub().CaptureEventWithHint(event, hint)
	return nil
}

func entryToSentryEvent(entry *log.Entry) *sentry.Event {
	fields := make(log.Fields, len(entry.Data))
	for key, value := range entry.Data {
		fields[key] = value
	}

	event := sentry.NewEvent()
	event.Level = logrusToSentryLevel[entry.Level]
	event.Message = entry.Message
	event.Timestamp = entry.Time
	event.Logger = sentryLoggerName

	switch request := fields["request"].(type) {
	case *http.Request:
		delete(fields, "request")
		event.Request = sentry.NewRequest(request)
	case sentry.Request:
		delete(fields, "request")
		event.Request = &request
	case *sentry.Request:
		delete(fields, "request")
		event.Request = request
	}

	if err, ok := fields[log.ErrorKey].(error); ok && err != nil {
		delete(fields, log.ErrorKey)
		event.SetException(err, errorDepth())
	}

	switch user := fields["user"].(type) {
	case sentry.User:
		delete(fields, "user")
		event.User = user
	case *sentry.User:
		delete(fields, "user")
		event.User = *user
	}

	if transaction, ok := fields["transaction"].(string); ok {
		delete(fields, "transaction")
		event.Transaction = transaction
	}

	if fingerprint, ok := fields["fingerprint"].([]string); ok {
		delete(fields, "fingerprint")
		event.Fingerprint = fingerprint
	}

	for key, value := range fields {
		event.Tags[key] = fmt.Sprint(value)
	}

	return event
}

func errorDepth() int {
	if client := sentry.CurrentHub().Client(); client != nil {
		return client.Options().MaxErrorDepth
	}
	return defaultErrorDepth
}
