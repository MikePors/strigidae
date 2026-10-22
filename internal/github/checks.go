package github

// CheckState is the combined state of the checks on a pull request's head
// commit. The zero value means the commit has no checks.
type CheckState string

// Check states.
const (
	ChecksNone    CheckState = ""
	ChecksPassing CheckState = "PASSING"
	ChecksFailing CheckState = "FAILING"
	ChecksRunning CheckState = "RUNNING"
)

// CheckStateFromRollup maps a GraphQL StatusState (SUCCESS, FAILURE, ERROR,
// PENDING, EXPECTED) to a CheckState. An empty or unknown state maps to
// ChecksNone.
func CheckStateFromRollup(state string) CheckState {
	switch state {
	case "SUCCESS":
		return ChecksPassing
	case "FAILURE", "ERROR":
		return ChecksFailing
	case "PENDING", "EXPECTED":
		return ChecksRunning
	}
	return ChecksNone
}
