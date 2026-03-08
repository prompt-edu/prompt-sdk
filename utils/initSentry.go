package utils

import (
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"
	sentrylogrus "github.com/getsentry/sentry-go/logrus"
	log "github.com/sirupsen/logrus"
)

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
		EnableLogs:       true,
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

	eventHook := sentrylogrus.NewEventHookFromClient(
		[]log.Level{log.ErrorLevel, log.FatalLevel, log.PanicLevel},
		client,
	)

	log.AddHook(logHook)
	log.AddHook(eventHook)

	log.RegisterExitHandler(func() {
		eventHook.Flush(5 * time.Second)
		logHook.Flush(5 * time.Second)
	})

	log.Info("Sentry initialized successfully")
	return nil
}
