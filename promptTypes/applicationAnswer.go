package promptTypes

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// Application answer type constants used to distinguish between different answer formats.
const (
	// TypeMultiSelect indicates that the answer allows multiple selections from predefined options.
	TypeMultiSelect = "multiselect"
	// TypeText indicates that the answer is a free-form text response.
	TypeText = "text"
)

// AnswersMultiSelect represents a multi-select application answer where multiple options can be selected.
// This type is used for application questions that allow students to choose multiple predefined answers.
type AnswersMultiSelect struct {
	// Type specifies the answer format, always "multiselect" for this type.
	Type string `json:"type"`
	// OrderNum indicates the display order of this question in the application form.
	OrderNum int `json:"order_num"`
	// QuestionID identifies the application question this answer belongs to. Reference a
	// question by this id. It is uuid.Nil for answers from a core version that does not
	// export question ids yet.
	QuestionID uuid.UUID `json:"questionID"`
	// Title is the question's title as shown in the application form, for display.
	Title string `json:"title"`
	// Key is the free-text access key a lecturer typed for this question. Prefer
	// QuestionID: core stops exporting the key once every phase references questions by id.
	Key string `json:"key"`
	// Answer contains the list of selected options from the available choices.
	Answer []string `json:"answer"`
}

// AnswersText represents a free-form text application answer.
// This type is used for application questions that require written responses from students.
type AnswersText struct {
	// Type specifies the answer format, always "text" for this type.
	Type string `json:"type"`
	// OrderNum indicates the display order of this question in the application form.
	OrderNum int `json:"order_num"`
	// QuestionID identifies the application question this answer belongs to. Reference a
	// question by this id. It is uuid.Nil for answers from a core version that does not
	// export question ids yet.
	QuestionID uuid.UUID `json:"questionID"`
	// Title is the question's title as shown in the application form, for display.
	Title string `json:"title"`
	// Key is the free-text access key a lecturer typed for this question. Prefer
	// QuestionID: core stops exporting the key once every phase references questions by id.
	Key string `json:"key"`
	// Answer contains the student's written response to the question.
	Answer string `json:"answer"`
}

// rawAnswer is the intermediate struct used for unmarshaling the raw JSON.
// This internal type handles the polymorphic nature of answer data during JSON parsing.
type rawAnswer struct {
	Type       string      `json:"type"`
	OrderNum   int         `json:"order_num"`
	Answer     interface{} `json:"answer"` // can be a string or list of strings
	QuestionID uuid.UUID   `json:"questionID"`
	Title      string      `json:"title"`
	Key        string      `json:"key"`
}

// ReadApplicationAnswersFromMetaData parses application answers from metadata.
// It separates and converts the raw answer data into strongly-typed structures:
//   - Text answers are returned as []AnswersText for type="text"
//   - Multi-select answers are returned as []AnswersMultiSelect for type="multiselect"
//
// This function handles the polymorphic nature of answer data where the same field
// can contain either a string (text) or array of strings (multiselect).
func ReadApplicationAnswersFromMetaData(data interface{}) ([]AnswersText, []AnswersMultiSelect, error) {
	// Step 1: Marshal interface{} into JSON
	rawBytes, err := json.Marshal(data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	// Step 2: Unmarshal into a slice of rawAnswer
	var rawAnswers []rawAnswer
	if err := json.Unmarshal(rawBytes, &rawAnswers); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal into rawAnswers: %w", err)
	}

	// Separate the answers into textAnswers and multiAnswers
	var (
		textAnswers  []AnswersText
		multiAnswers []AnswersMultiSelect
	)

	// Step 3: Convert each rawAnswer to the correct final struct
	for _, ra := range rawAnswers {
		switch ra.Type {
		case TypeMultiSelect:
			// Expect ra.Answer to be an array of strings
			arr, ok := ra.Answer.([]interface{})
			if !ok {
				return nil, nil, fmt.Errorf("multiselect answer was not an array: %v", ra.Answer)
			}
			stringArr := make([]string, 0, len(arr))
			for _, item := range arr {
				s, ok := item.(string)
				if !ok {
					return nil, nil, fmt.Errorf("multiselect array contains non-string element: %v", item)
				}
				stringArr = append(stringArr, s)
			}
			multiAnswers = append(multiAnswers, AnswersMultiSelect{
				Type:       ra.Type,
				OrderNum:   ra.OrderNum,
				QuestionID: ra.QuestionID,
				Title:      ra.Title,
				Key:        ra.Key,
				Answer:     stringArr,
			})

		case TypeText:
			// Expect ra.Answer to be a single string
			s, ok := ra.Answer.(string)
			if !ok {
				return nil, nil, fmt.Errorf("text answer was not a string: %v", ra.Answer)
			}
			textAnswers = append(textAnswers, AnswersText{
				Type:       ra.Type,
				OrderNum:   ra.OrderNum,
				QuestionID: ra.QuestionID,
				Title:      ra.Title,
				Key:        ra.Key,
				Answer:     s,
			})

		default:
			return nil, nil, fmt.Errorf("unknown answer type: %s", ra.Type)
		}
	}

	return textAnswers, multiAnswers, nil
}
