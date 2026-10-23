package workspace

import (
	"fmt"
	"strings"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// methodArgs maps the :merge argument to a method; "" is the default.
var methodArguments = map[string]github.MergeMethod{
	"merge":  github.MergeMethodMerge,
	"squash": github.MergeMethodSquash,
	"rebase": github.MergeMethodRebase,
}

// methodNames are how the methods read in messages.
var methodNames = map[github.MergeMethod]string{
	github.MergeMethodMerge:  "merge commit",
	github.MergeMethodSquash: "squash",
	github.MergeMethodRebase: "rebase",
}

// methodPlurals complete "... are not allowed in this repo".
var methodPlurals = map[github.MergeMethod]string{
	github.MergeMethodMerge:  "merge commits",
	github.MergeMethodSquash: "squash merges",
	github.MergeMethodRebase: "rebase merges",
}

// parseMergeArgs returns the requested method, or "" for the default.
func parseMergeArguments(arguments string) (github.MergeMethod, error) {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return "", nil
	}
	if method, ok := methodArguments[arguments]; ok {
		return method, nil
	}
	return "", fmt.Errorf("unknown merge method %q (use merge, squash or rebase)", terminaltext.Clean(arguments))
}

// resolveMethod picks the method to merge with: the requested one if the repo
// allows it, otherwise (when none was requested) the first allowed one.
func resolveMethod(requested github.MergeMethod, status github.MergeStatus) (github.MergeMethod, error) {
	if requested != "" {
		if !status.Allows(requested) {
			return "", fmt.Errorf("%s are not allowed in this repo", methodPlurals[requested])
		}
		return requested, nil
	}
	for _, method := range github.MergeMethods {
		if status.Allows(method) {
			return method, nil
		}
	}
	return "", fmt.Errorf("no merge method is allowed in this repo")
}

// mergeQuestion is the confirmation text; it always names the method and
// says what happens to the head branch.
func mergeQuestion(number int, method github.MergeMethod, status github.MergeStatus) string {
	branch := "Branch will be kept."
	if status.DeleteBranchOnMerge {
		branch = "Branch will be deleted."
	}
	return fmt.Sprintf("Merge #%d into %s (%s)? %s", number, terminaltext.Clean(status.BaseBranch), methodNames[method], branch)
}

// mergedText is the success message.
func mergedText(number int, method github.MergeMethod) string {
	return fmt.Sprintf("Merged #%d (%s)", number, methodNames[method])
}
