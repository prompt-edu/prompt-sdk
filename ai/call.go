// Package ai is the contract between a phase service and the AI server: the headers that describe
// a model call, the oversight events a phase reports on a call, and a client that sends both. The
// AI server reads the same constants and types, so the two sides cannot drift.
package ai

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const (
	// ServiceHeader and ServiceTokenHeader name the calling phase service and carry its key from
	// the AI server's AI_SERVICE_KEYS. Inference and events need both.
	ServiceHeader      = "X-Prompt-Service"
	ServiceTokenHeader = "X-Prompt-Service-Token"
	// ProviderKeyHeader carries the course phase's own Logos key. The AI server forwards it to the
	// provider and stores no keys.
	ProviderKeyHeader = "X-Prompt-Provider-Key"
	// FeatureHeader names the feature as "<phase type slug>.<feature>". The AI server's feature
	// registry derives the content retention from it.
	FeatureHeader         = "X-Prompt-Feature"
	TemplateHeader        = "X-Prompt-Template"
	TemplateVersionHeader = "X-Prompt-Template-Version"
	// SubjectsHeader lists the course participation ids a call is about, comma-separated. They
	// decide whose privacy export and erasure the call's records belong to.
	SubjectsHeader = "X-Prompt-Subjects"
	// CallIDHeader names the recorded call on the AI server's answer. Events refer to it.
	CallIDHeader = "X-Prompt-AI-Call-ID"
)

const (
	MaxSubjects          = 100
	MinProviderKeyLength = 8
	MaxProviderKeyLength = 512
)

var (
	ErrInvalidProviderKey = errors.New("the provider key must be 8 to 512 printable characters without spaces")
	ErrInvalidLabel       = errors.New("a label takes letters, digits, '.', '_' and '-', at most 128")
	ErrInvalidFeature     = errors.New("the feature must be named <service>.<feature>")
	ErrTooManySubjects    = fmt.Errorf("a call names at most %d subjects", MaxSubjects)
)

var labelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// ValidLabel reports whether the AI server accepts label as a feature, template or template version.
func ValidLabel(label string) bool {
	return labelPattern.MatchString(label)
}

// ValidProviderKey reports whether the AI server accepts key as a Logos key.
func ValidProviderKey(key string) bool {
	return len(key) >= MinProviderKeyLength && len(key) <= MaxProviderKeyLength &&
		!strings.ContainsFunc(key, func(r rune) bool { return r < '!' || r > '~' })
}

// Call describes one model call. Template, TemplateVersion and Subjects are optional; a call
// about students should name them as Subjects.
type Call struct {
	// ProviderKey is the course phase's Logos key.
	ProviderKey     string
	Feature         string
	Template        string
	TemplateVersion string
	Subjects        []uuid.UUID
}

// validate checks the call as the AI server will, so a misconfigured call fails before it is sent.
// The AI server only accepts a feature whose prefix is the calling service.
func (c Call) validate(serviceName string) error {
	if !ValidProviderKey(c.ProviderKey) {
		return ErrInvalidProviderKey
	}
	prefix, rest, found := strings.Cut(c.Feature, ".")
	if !found || prefix != serviceName || rest == "" {
		return fmt.Errorf("%w, here %s.<feature>", ErrInvalidFeature, serviceName)
	}
	for _, label := range []string{c.Feature, c.Template, c.TemplateVersion} {
		if label != "" && !ValidLabel(label) {
			return ErrInvalidLabel
		}
	}
	if len(c.Subjects) > MaxSubjects {
		return ErrTooManySubjects
	}
	return nil
}

func (c Call) setHeaders(header http.Header) {
	header.Set(ProviderKeyHeader, c.ProviderKey)
	header.Set(FeatureHeader, c.Feature)
	setOptional(header, TemplateHeader, c.Template)
	setOptional(header, TemplateVersionHeader, c.TemplateVersion)
	subjects := make([]string, len(c.Subjects))
	for i, subject := range c.Subjects {
		subjects[i] = subject.String()
	}
	setOptional(header, SubjectsHeader, strings.Join(subjects, ","))
}

// setOptional deletes an empty header instead of sending it, so a reused header set never keeps a
// value from an earlier call.
func setOptional(header http.Header, name, value string) {
	if value == "" {
		header.Del(name)
		return
	}
	header.Set(name, value)
}

// CallID reads the id of the recorded call from the AI server's answer.
func CallID(header http.Header) (uuid.UUID, bool) {
	id, err := uuid.Parse(header.Get(CallIDHeader))
	return id, err == nil
}
