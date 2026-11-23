package ai

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validCall() Call {
	return Call{ProviderKey: "logos-key-1234", Feature: "assessment.action_item_suggestions"}
}

func TestValidLabel(t *testing.T) {
	for label, want := range map[string]bool{
		"assessment.action_item_suggestions": true,
		"v1.2-rc_3":                          true,
		"A":                                  true,
		strings.Repeat("a", 128):             true,
		strings.Repeat("a", 129):             false,
		"":                                   false,
		".hidden":                            false,
		"with space":                         false,
		"slash/inside":                       false,
		"umlaut-ä":                           false,
	} {
		assert.Equal(t, want, ValidLabel(label), label)
	}
}

func TestValidProviderKey(t *testing.T) {
	for key, want := range map[string]bool{
		"12345678":               true,
		strings.Repeat("k", 512): true,
		"1234567":                false,
		strings.Repeat("k", 513): false,
		"key with space":         false,
		"key\twith-tab":          false,
		"key-with-ü-umlaut":      false,
	} {
		assert.Equal(t, want, ValidProviderKey(key), key)
	}
}

func TestCallValidate(t *testing.T) {
	tooMany := make([]uuid.UUID, MaxSubjects+1)
	for i := range tooMany {
		tooMany[i] = uuid.New()
	}
	tests := []struct {
		name   string
		modify func(*Call)
		want   error
	}{
		{"valid", func(*Call) {}, nil},
		{"with template and subjects", func(c *Call) {
			c.Template, c.TemplateVersion, c.Subjects = "action-items", "3", tooMany[:MaxSubjects]
		}, nil},
		{"missing provider key", func(c *Call) { c.ProviderKey = "" }, ErrInvalidProviderKey},
		{"missing feature", func(c *Call) { c.Feature = "" }, ErrInvalidFeature},
		{"feature of another service", func(c *Call) { c.Feature = "interview.summary" }, ErrInvalidFeature},
		{"feature without a name", func(c *Call) { c.Feature = "assessment." }, ErrInvalidFeature},
		{"feature without a service", func(c *Call) { c.Feature = "action_item_suggestions" }, ErrInvalidFeature},
		{"invalid feature label", func(c *Call) { c.Feature = "assessment.with space" }, ErrInvalidLabel},
		{"invalid template", func(c *Call) { c.Template = "action items" }, ErrInvalidLabel},
		{"invalid template version", func(c *Call) { c.TemplateVersion = "v/1" }, ErrInvalidLabel},
		{"too many subjects", func(c *Call) { c.Subjects = tooMany }, ErrTooManySubjects},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			call := validCall()
			tc.modify(&call)
			err := call.validate("assessment")
			if tc.want == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tc.want)
		})
	}
}

func TestCallSetHeaders(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	call := validCall()
	call.Template, call.TemplateVersion, call.Subjects = "action-items", "3", []uuid.UUID{first, second}

	header := http.Header{}
	call.setHeaders(header)
	assert.Equal(t, "logos-key-1234", header.Get(ProviderKeyHeader))
	assert.Equal(t, "assessment.action_item_suggestions", header.Get(FeatureHeader))
	assert.Equal(t, "action-items", header.Get(TemplateHeader))
	assert.Equal(t, "3", header.Get(TemplateVersionHeader))
	assert.Equal(t, first.String()+","+second.String(), header.Get(SubjectsHeader))

	// A reused header set drops what the next call leaves out.
	validCall().setHeaders(header)
	for _, name := range []string{TemplateHeader, TemplateVersionHeader, SubjectsHeader} {
		assert.Empty(t, header.Values(name), name)
	}
}

func TestCallID(t *testing.T) {
	id := uuid.New()
	header := http.Header{}
	header.Set(CallIDHeader, id.String())
	got, ok := CallID(header)
	require.True(t, ok)
	assert.Equal(t, id, got)

	_, ok = CallID(http.Header{})
	assert.False(t, ok)
}

func TestEventRequestValidate(t *testing.T) {
	zero, distance, negative := 0, 12, -1
	target := uuid.New()
	for name, tc := range map[string]struct {
		event EventRequest
		valid bool
	}{
		"shown":                    {EventRequest{Type: EventShown}, true},
		"accepted with a target":   {EventRequest{Type: EventAccepted, TargetID: &target}, true},
		"edited with a distance":   {EventRequest{Type: EventEdited, EditDistance: &distance}, true},
		"edited without change":    {EventRequest{Type: EventEdited, EditDistance: &zero}, true},
		"rejected":                 {EventRequest{Type: EventRejected}, true},
		"edited without distance":  {EventRequest{Type: EventEdited}, false},
		"negative distance":        {EventRequest{Type: EventEdited, EditDistance: &negative}, false},
		"content_viewed":           {EventRequest{Type: "content_viewed"}, false},
		"empty type":               {EventRequest{}, false},
		"negative on another type": {EventRequest{Type: EventAccepted, EditDistance: &negative}, false},
	} {
		err := tc.event.Validate()
		if tc.valid {
			assert.NoError(t, err, name)
		} else {
			assert.ErrorIs(t, err, ErrInvalidEvent, name)
		}
	}
}
