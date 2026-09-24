package utils

import (
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"
	sentrylogrus "github.com/getsentry/sentry-go/logrus"
	log "github.com/sirupsen/logrus"
)

func InitSentry(sentryDsn string, release ...string) error {
	if sentryDsn == "" {
		log.Info("Sentry DSN not configured, skipping initialization")
		return nil
	}

	var sentryRelease string
	if len(release) > 0 {
		sentryRelease = release[0]
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
		Release:          sentryRelease,
		Environment:      GetEnv("ENVIRONMENT", "development"),
		Debug:            false,
		Transport:        transport,
		AttachStacktrace: true,
		DataCollection:   sentryDataCollection(sendDefaultPII),
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

	eventHook := newSentryEventHook([]log.Level{log.ErrorLevel, log.FatalLevel, log.PanicLevel})

	log.AddHook(logHook)
	log.AddHook(eventHook)

	log.RegisterExitHandler(func() {
		sentry.Flush(5 * time.Second)
		logHook.Flush(5 * time.Second)
	})

	log.Infof("Sentry initialized successfully (release: %s)", client.Options().Release)
	return nil
}

func sentryDataCollection(sendDefaultPII bool) *sentry.DataCollection {
	if sendDefaultPII {
		return &sentry.DataCollection{}
	}

	return &sentry.DataCollection{
		UserInfo: sentry.Set(false),
		Cookies:  &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff},
		HTTPHeaders: &sentry.HeaderCollectionConfig{
			Request:  piiDenyList(),
			Response: piiDenyList(),
		},
		HTTPBodies:  []sentry.BodyType{},
		QueryParams: piiDenyList(),
	}
}

// Mirrors sentry-go's unexported SendDefaultPII=false denylist, which also scrubs IP and user-id keys.
func piiDenyList() *sentry.KeyValueCollectionBehavior {
	return &sentry.KeyValueCollectionBehavior{
		Mode:  sentry.CollectionDenyList,
		Terms: []string{"forwarded", "-ip", "remote-", "via", "-user"},
	}
}
