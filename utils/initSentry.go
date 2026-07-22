package utils

import (
	"errors"
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"
	sentrylogrus "github.com/getsentry/sentry-go/logrus"
	log "github.com/sirupsen/logrus"
)

// sentryEventHook forwards error-level logrus entries to Sentry as issues via CaptureException.
// It replaces sentrylogrus.NewEventHookFromClient (deprecated in sentry-go 0.47, removed in 0.48),
// which is the recommended migration: errors become issues via CaptureException while the log hook
// keeps handling info/warn as structured logs. A plain level swap to the log hook would instead
// downgrade errors from issues to structured logs.
type sentryEventHook struct {
	levels []log.Level
}

func (h *sentryEventHook) Levels() []log.Level { return h.levels }

func (h *sentryEventHook) Fire(entry *log.Entry) error {
	if err, ok := entry.Data[log.ErrorKey].(error); ok && err != nil {
		sentry.CaptureException(err)
		return nil
	}
	sentry.CaptureException(errors.New(entry.Message))
	return nil
}

func InitSentry(sentryDsn string) error {
	if sentryDsn == "" {
		log.Info("Sentry DSN not configured, skipping initialization")
		return nil
	}

	transport := sentry.NewHTTPTransport()
	transport.Timeout = 2 * time.Second
	sendDefaultPII, err := strconv.ParseBool(GetEnv("SENTRY_SEND_DEFAULT_PII", "false"))
	if err != nil {
		log.Warnf("Invalid SENTRY_SEND_DEFAULT_PII value, defaulting to false: %v", err)
		sendDefaultPII = false
	}

	if err := sentry.Init(sentry.ClientOptions{
		Dsn:              sentryDsn,
		Environment:      GetEnv("ENVIRONMENT", "development"),
		Debug:            false,
		Transport:        transport,
		AttachStacktrace: true,
		SendDefaultPII:   sendDefaultPII,
		EnableTracing:    true,
		TracesSampleRate: 1.0,
	}); err != nil {
		log.Errorf("Sentry initialization failed: %v", err)
		return err
	}

	client := sentry.CurrentHub().Client()
	if client == nil {
		log.Error("Sentry client is nil")
		return err
	}

	logHook := sentrylogrus.NewLogHookFromClient(
		[]log.Level{log.InfoLevel, log.WarnLevel},
		client,
	)

	eventHook := &sentryEventHook{
		levels: []log.Level{log.ErrorLevel, log.FatalLevel, log.PanicLevel},
	}

	log.AddHook(logHook)
	log.AddHook(eventHook)

	log.RegisterExitHandler(func() {
		sentry.Flush(5 * time.Second)
		logHook.Flush(5 * time.Second)
	})

	log.Info("Sentry initialized successfully")
	return nil
}
