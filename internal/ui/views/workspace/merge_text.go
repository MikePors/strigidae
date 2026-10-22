package workspace

import (
	"fmt"
	"strings"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui/termtext"
)

// methodArgs maps the :merge argument to a method; "" is the default.
var methodArgs = map[string]github.MergeMethod{
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
func parseMergeArgs(args string) (github.MergeMethod, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return "", nil
	}
	if m, ok := methodArgs[args]; ok {
		return m, nil
	}
	return "", fmt.Errorf("unknown merge method %q (use merge, squash or rebase)", termtext.Clean(args))
}

// resolveMethod picks the method to merge with: the requested one if the repo
// allows it, otherwise (when none was requested) the first allowed one.
func resolveMethod(requested github.MergeMethod, st github.MergeStatus) (github.MergeMethod, error) {
	if requested != "" {
		if !st.Allows(requested) {
			return "", fmt.Errorf("%s are not allowed in this repo", methodPlurals[requested])
		}
		return requested, nil
	}
	for _, m := range github.MergeMethods {
		if st.Allows(m) {
			return m, nil
		}
	}
	return "", fmt.Errorf("no merge method is allowed in this repo")
}

// mergeQuestion is the confirmation text; it always names the method and
// says what happens to the head branch.
func mergeQuestion(number int, method github.MergeMethod, st github.MergeStatus) string {
	branch := "Branch will be kept."
	if st.DeleteBranchOnMerge {
		branch = "Branch will be deleted."
	}
	return fmt.Sprintf("Merge #%d into %s (%s)? %s", number, termtext.Clean(st.BaseRef), methodNames[method], branch)
}

// mergedText is the success message.
func mergedText(number int, method github.MergeMethod) string {
	return fmt.Sprintf("Merged #%d (%s)", number, methodNames[method])
}
