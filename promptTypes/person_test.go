package promptTypes

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/prompt-edu/prompt-sdk/utils"
	"github.com/stretchr/testify/require"
)

func TestStudentJSONKeepsContactFieldsFlat(t *testing.T) {
	student := Student{Person: Person{Email: "jane@example.com", UniversityLogin: "ab12cde"}}

	raw, err := json.Marshal(student)
	require.NoError(t, err)

	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))
	require.Equal(t, "jane@example.com", fields["email"])
	require.Equal(t, "ab12cde", fields["universityLogin"])
	require.NotContains(t, fields, "Person")

	var decoded Student
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, "jane@example.com", decoded.Email)
	require.Equal(t, "ab12cde", decoded.UniversityLogin)
}

func TestTeamMembersCarryContactFields(t *testing.T) {
	payload := `{"id":"` + uuid.NewString() + `","name":"Team 1",
		"members":[{"id":"` + uuid.NewString() + `","firstName":"Jane","lastName":"Doe","email":"jane@example.com","universityLogin":"ab12cde"}],
		"tutors":[{"id":"` + uuid.NewString() + `","firstName":"Max","lastName":"Tutor"}]}`

	var team Team
	require.NoError(t, json.Unmarshal([]byte(payload), &team))
	require.Equal(t, "jane@example.com", team.Members[0].Email)
	require.Equal(t, "ab12cde", team.Members[0].UniversityLogin)
	require.Empty(t, team.Tutors[0].Email)
}

func TestPersonValidationAllowsMissingContactFields(t *testing.T) {
	team := Team{Name: "Team 1", Members: []Person{{FirstName: "Jane"}}}
	require.NoError(t, utils.ValidateStruct(team))

	team.Members[0].Email = "not-an-email"
	require.Error(t, utils.ValidateStruct(team))

	team.Members[0].Email = "jane@example.com"
	team.Members[0].UniversityLogin = "not-a-login"
	require.Error(t, utils.ValidateStruct(team))
}
