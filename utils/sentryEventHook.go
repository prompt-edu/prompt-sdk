package utils

import (
	"github.com/getsentry/sentry-go"
	log "github.com/sirupsen/logrus"
)

// logrusToSentryLevel maps the error-class logrus levels to Sentry levels.
var logrusToSentryLevel = map[log.Level]sentry.Level{
	log.ErrorLevel: sentry.LevelError,
	log.FatalLevel: sentry.LevelFatal,
	log.PanicLevel: sentry.LevelFatal,
}

// sentryEventHook captures error-level logrus entries as Sentry issues via CaptureException, the
// path recommended by sentry-go after NewEventHookFromClient was deprecated (removed in 0.48.0).
// It preserves the previous behavior: error/fatal/panic logs still create Sentry issues (not just
// structured logs, which is what NewLogHookFromClient would produce), carrying the entry's level
// and fields.
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
	hub := sentry.CurrentHub().Clone()
	hub.WithScope(func(scope *sentry.Scope) {
		if level, ok := logrusToSentryLevel[entry.Level]; ok {
			scope.SetLevel(level)
		}
		if len(entry.Data) > 0 {
			fields := make(sentry.Context, len(entry.Data))
			for key, value := range entry.Data {
				fields[key] = value
			}
			scope.SetContext("logrus", fields)
		}

		if err, ok := entry.Data[log.ErrorKey].(error); ok && err != nil {
			hub.CaptureException(err)
			return
		}
		hub.CaptureMessage(entry.Message)
	})
	return nil
}
