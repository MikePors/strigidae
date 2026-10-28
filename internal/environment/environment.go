// Package environment builds the environment for programs strig starts.
package environment

import "strings"

// tokenVariables hold GitHub credentials. strig reads them for its own login;
// the programs it starts have no use for them.
var tokenVariables = []string{
	"GH_TOKEN",
	"GITHUB_TOKEN",
	"GH_ENTERPRISE_TOKEN",
	"GITHUB_ENTERPRISE_TOKEN",
}

// Scrub returns a copy of variables (in os.Environ form, "NAME=value") without
// the GitHub token variables. Every program strig starts should get its result
// as exec.Cmd.Env.
func Scrub(variables []string) []string {
	scrubbed := make([]string, 0, len(variables))
	for _, variable := range variables {
		if !isToken(variable) {
			scrubbed = append(scrubbed, variable)
		}
	}
	return scrubbed
}

func isToken(variable string) bool {
	name, _, _ := strings.Cut(variable, "=")
	for _, token := range tokenVariables {
		if name == token {
			return true
		}
	}
	return false
}
