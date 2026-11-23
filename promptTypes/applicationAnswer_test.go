package promptTypes

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// decodeMetaData mirrors how consumers receive the answers: as the untyped
// prevData["applicationAnswers"] value of a resolved participation.
func decodeMetaData(t *testing.T, payload string) interface{} {
	t.Helper()
	var data interface{}
	require.NoError(t, json.Unmarshal([]byte(payload), &data))
	return data
}

func TestReadApplicationAnswersReadsQuestionIDAndTitle(t *testing.T) {
	textID := uuid.New()
	multiID := uuid.New()
	data := decodeMetaData(t, `[
		{"type":"text","order_num":1,"questionID":"`+textID.String()+`","title":"Why this course?","key":"motivation","answer":"Curiosity"},
		{"type":"multiselect","order_num":2,"questionID":"`+multiID.String()+`","title":"English level","key":"language_proficiency_english","answer":["C1/C2"]}
	]`)

	texts, multis, err := ReadApplicationAnswersFromMetaData(data)
	require.NoError(t, err)

	require.Equal(t, []AnswersText{{
		Type: TypeText, OrderNum: 1, QuestionID: textID, Title: "Why this course?", Key: "motivation", Answer: "Curiosity",
	}}, texts)
	require.Equal(t, []AnswersMultiSelect{{
		Type: TypeMultiSelect, OrderNum: 2, QuestionID: multiID, Title: "English level", Key: "language_proficiency_english", Answer: []string{"C1/C2"},
	}}, multis)
}

func TestReadApplicationAnswersAcceptsKeyOnlyAnswers(t *testing.T) {
	// Core versions before prompt-edu/prompt#2348 export the access key only.
	data := decodeMetaData(t, `[
		{"type":"text","order_num":1,"key":"motivation","answer":"Curiosity"},
		{"type":"multiselect","order_num":2,"key":"language_proficiency_german","answer":["Native"]}
	]`)

	texts, multis, err := ReadApplicationAnswersFromMetaData(data)
	require.NoError(t, err)

	require.Len(t, texts, 1)
	require.Equal(t, uuid.Nil, texts[0].QuestionID)
	require.Empty(t, texts[0].Title)
	require.Equal(t, "motivation", texts[0].Key)

	require.Len(t, multis, 1)
	require.Equal(t, uuid.Nil, multis[0].QuestionID)
	require.Equal(t, "language_proficiency_german", multis[0].Key)
	require.Equal(t, []string{"Native"}, multis[0].Answer)
}

func TestReadApplicationAnswersAcceptsAnswersWithoutKey(t *testing.T) {
	// Once every phase references questions by id, core stops exporting the key.
	questionID := uuid.New()
	data := decodeMetaData(t, `[{"type":"text","order_num":1,"questionID":"`+questionID.String()+`","title":"Why this course?","answer":"Curiosity"}]`)

	texts, _, err := ReadApplicationAnswersFromMetaData(data)
	require.NoError(t, err)
	require.Len(t, texts, 1)
	require.Equal(t, questionID, texts[0].QuestionID)
	require.Empty(t, texts[0].Key)
}

func TestReadApplicationAnswersRejectsMalformedAnswers(t *testing.T) {
	cases := map[string]string{
		"invalid question id":   `[{"type":"text","questionID":"not-a-uuid","answer":"x"}]`,
		"text answer as array":  `[{"type":"text","answer":["x"]}]`,
		"multiselect as string": `[{"type":"multiselect","answer":"x"}]`,
		"unknown type":          `[{"type":"fileupload","answer":"x"}]`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := ReadApplicationAnswersFromMetaData(decodeMetaData(t, payload))
			require.Error(t, err)
		})
	}
}

func TestReadApplicationAnswersHandlesMissingAnswers(t *testing.T) {
	// prevData carries no applicationAnswers when the participant answered no exported question.
	texts, multis, err := ReadApplicationAnswersFromMetaData(nil)
	require.NoError(t, err)
	require.Empty(t, texts)
	require.Empty(t, multis)
}
