package workspace

import (
	"errors"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/beefis"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/diffview"
)

// fakeAsker records questions; Receive is never called by these tests (the
// root's listening is simulated by handing messages to Heard).
type fakeAsker struct {
	asked  []beefis.Question
	askErr error
}

func (asker *fakeAsker) Ask(question beefis.Question) error {
	asker.asked = append(asker.asked, question)
	return asker.askErr
}

func (asker *fakeAsker) Receive() (beefis.Answer, error) { return beefis.Answer{}, io.EOF }

type asking struct {
	*inliner
	asker     *fakeAsker
	questions *Questions
}

func newAsking(test *testing.T) *asking {
	inliner := newInliner(test)
	asker := &fakeAsker{}
	questions := NewQuestions(asker)
	inliner.view.questions = questions
	return &asking{inliner: inliner, asker: asker, questions: questions}
}

// deliver does what the root does with a message: the companion hears it
// first, then the view gets it. What the companion runs next is returned,
// not run, because listening again would wait for beefis.
func (asking *asking) deliver(message tea.Msg) tea.Cmd {
	asking.test.Helper()
	heard, next := asking.questions.Heard(message)
	if !heard {
		asking.test.Fatalf("companion did not hear %T", message)
	}
	asking.send(message)
	return next
}

// ask runs :ask with text and delivers the send result.
func (asking *asking) ask(text string) {
	asking.test.Helper()
	command, handled := asking.view.Command("ask", text)
	if !handled {
		asking.test.Fatal(":ask not handled")
	}
	for _, message := range messages(command) {
		if _, ok := message.(askSentMessage); ok {
			asking.run(asking.deliver(message)) // a failed send returns an error to show
			continue
		}
		asking.messages = append(asking.messages, message)
	}
}

func (asking *asking) answer(answer beefis.Answer) {
	asking.test.Helper()
	asking.deliver(answerMessage{questions: asking.questions, answer: answer})
}

func TestAskWithoutBeefisSaysHowToTurnItOn(test *testing.T) {
	inliner := newInliner(test)
	inliner.onDiffLine()
	command, _ := inliner.view.Command("ask", "what is this")
	inliner.run(command)
	if got := inliner.lastInfo(); !strings.Contains(got, "--beefis") {
		test.Errorf("notice = %q, want one naming --beefis", got)
	}
}

func TestAskNeedsAQuestionAndALine(test *testing.T) {
	asking := newAsking(test)
	asking.onDiffLine()
	asking.ask("   ")
	if got := asking.lastInfo(); got != "usage: :ask <question>" {
		test.Errorf("empty question: notice = %q", got)
	}
	asking.diffView().GotoTop() // the file header, not a line
	asking.ask("what is this")
	if got := asking.lastInfo(); got != "move onto a diff line to ask about it" {
		test.Errorf("off a line: notice = %q", got)
	}
	if len(asking.asker.asked) != 0 {
		test.Errorf("asked %d questions, want none", len(asking.asker.asked))
	}
}

func TestAskSendsTheLineAndShowsItWaiting(test *testing.T) {
	asking := newAsking(test)
	asking.onDiffLine()
	asking.ask("what is this")
	if len(asking.asker.asked) != 1 {
		test.Fatalf("asked %d questions, want 1", len(asking.asker.asked))
	}
	question := asking.asker.asked[0]
	excerpt := question.Excerpt
	question.Excerpt = ""
	want := beefis.Question{
		RequestIdentifier: "q1", Repository: "acme/widgets", Number: 7, Head: headSHA,
		Path: "internal/sync/client.go", Side: "RIGHT", Line: 14, Question: "what is this",
	}
	if diff := cmp.Diff(want, question); diff != "" {
		test.Errorf("question (-want +got):\n%s", diff)
	}
	if excerpt == "" {
		test.Error("no excerpt of the diff")
	}
	screen := asking.screen()
	for _, text := range []string{"what is this", "Waiting for an answer", answerAuthor} {
		if !strings.Contains(screen, text) {
			test.Errorf("screen lacks %q:\n%s", text, screen)
		}
	}
}

func TestAnAnswerShowsAtItsLine(test *testing.T) {
	asking := newAsking(test)
	asking.onDiffLine()
	asking.ask("what is this")
	asking.answer(beefis.Answer{RequestIdentifier: "q1", Text: "this is a method call"})
	screen := asking.screen()
	if !strings.Contains(screen, "this is a method call") || strings.Contains(screen, "Waiting for an answer") {
		test.Errorf("answer not shown:\n%s", screen)
	}
	if got := asking.lastInfo(); got != "answer for internal/sync/client.go:14" {
		test.Errorf("notice = %q", got)
	}
}

func TestAnswersAreSafeText(test *testing.T) {
	asking := newAsking(test)
	asking.onDiffLine()
	asking.ask("what is this")
	asking.answer(beefis.Answer{RequestIdentifier: "q1", Text: "evil \x1b]0;pwned\x07 \x1b[2J text \u202e"})
	rendered := asking.render()
	for _, bad := range []string{"\x1b]0;", "\x1b[2J", "\u202e"} {
		if strings.Contains(rendered, bad) {
			test.Errorf("render contains %q", bad)
		}
	}
}

func TestAnAnswerWhileClosedIsThereOnReopen(test *testing.T) {
	asking := newAsking(test)
	asking.onDiffLine()
	asking.ask("what is this")
	// The workspace is closed: only the companion hears the answer.
	heard, _ := asking.questions.Heard(answerMessage{questions: asking.questions, answer: beefis.Answer{RequestIdentifier: "q1", Text: "heard while away"}})
	if !heard {
		test.Fatal("answer not heard")
	}
	reopened := newInliner(test)
	reopened.view.questions = asking.questions
	reopened.run(reopened.view.Init())
	reopened.onDiffLine()
	reopened.press("z", "o")
	if screen := ansi.Strip(reopened.render()); !strings.Contains(screen, "heard while away") {
		test.Errorf("reopened workspace lacks the answer:\n%s", screen)
	}
}

func TestAnswerToAnotherPullRequestOrUnknownIsIgnored(test *testing.T) {
	asking := newAsking(test)
	asking.onDiffLine()
	asking.ask("what is this")
	asking.answer(beefis.Answer{RequestIdentifier: "q99", Text: "nobody asked"})
	if strings.Contains(asking.screen(), "nobody asked") || asking.lastInfo() != "" && strings.HasPrefix(asking.lastInfo(), "answer for") {
		test.Error("an answer to an unknown question was shown")
	}
	other := &Questions{asker: asking.asker}
	if heard, _ := asking.questions.Heard(answerMessage{questions: other}); heard {
		test.Error("heard another companion's answer")
	}
}

func TestAFailedSendShowsOnTheQuestion(test *testing.T) {
	asking := newAsking(test)
	asking.asker.askErr = errors.New("broken pipe")
	asking.onDiffLine()
	asking.ask("what is this")
	if err := asking.lastErr(); err == nil || !strings.Contains(err.Error(), "broken pipe") {
		test.Errorf("error = %v, want the send failure", err)
	}
	if screen := asking.screen(); !strings.Contains(screen, "No answer: could not ask") {
		test.Errorf("question does not show the failure:\n%s", screen)
	}
}

func TestBeefisClosingStopsListening(test *testing.T) {
	asking := newAsking(test)
	next := asking.deliver(answerMessage{questions: asking.questions, err: io.EOF})
	var shown []string
	for _, message := range messages(next) {
		if errorMessage, ok := message.(userinterface.ErrorMessage); ok {
			shown = append(shown, errorMessage.Err.Error())
			continue
		}
		test.Errorf("after EOF the companion runs %T, want only an error", message)
	}
	if len(shown) != 1 {
		test.Errorf("errors = %v, want one", shown)
	}
	asking.onDiffLine()
	asking.ask("anyone there")
	if got := asking.lastInfo(); !strings.Contains(got, "closed the connection") {
		test.Errorf("notice = %q", got)
	}
}

func TestAReadErrorKeepsListening(test *testing.T) {
	asking := newAsking(test)
	_, next := asking.questions.Heard(answerMessage{questions: asking.questions, err: errors.New("bad line")})
	var listened bool
	for _, message := range messages(next) {
		if _, ok := message.(answerMessage); ok {
			listened = true
		}
	}
	if !listened {
		test.Error("did not listen again after a read error")
	}
}

func TestLocalThreadsRefuseGitHubActions(test *testing.T) {
	asking := newAsking(test)
	asking.onDiffLine()
	asking.ask("what is this")
	asking.toThread(localThreadPrefix + "q1")
	for _, action := range []string{"r", "x"} {
		asking.messages = nil
		asking.press(action)
		if got := asking.lastInfo(); !strings.HasPrefix(got, "this answer is local to strig, not on GitHub") {
			test.Errorf("%s: notice = %q", action, got)
		}
	}
	asking.toCommentOf(localThreadPrefix+"q1", 1)
	for _, command := range []string{"react", "edit-comment", "delete-comment"} {
		asking.messages = nil
		run, _ := asking.view.Command(command, "")
		asking.run(run)
		if got := asking.lastInfo(); !strings.HasPrefix(got, "this answer is local to strig, not on GitHub") {
			test.Errorf("%s: notice = %q", command, got)
		}
	}
	if writes := asking.methods(fake.MethodReplyToThread, fake.MethodResolveReviewThread, fake.MethodAddReaction, fake.MethodRemoveReaction, fake.MethodUpdateReviewComment, fake.MethodDeleteReviewComment); len(writes) != 0 {
		test.Errorf("wrote to GitHub: %v", writes)
	}
}

func TestAQuestionOnAnOldHeadIsOutdated(test *testing.T) {
	questions := NewQuestions(&fakeAsker{})
	questions.add(beefis.Question{Number: 7, Head: "old", Path: "a.go", Side: "RIGHT", Line: 4, StartLine: 2}, testRepository, testNow)
	for _, testCase := range []struct {
		name     string
		head     string
		outdated bool
		line     int
	}{
		{"same head", "old", false, 4},
		{"new head", "new", true, 0},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			threads := questions.threads(testRepository, 7, testCase.head)
			if len(threads) != 1 {
				test.Fatalf("%d threads, want 1", len(threads))
			}
			thread := threads[0]
			if thread.IsOutdated != testCase.outdated || thread.Line != testCase.line || thread.OriginalLine != 4 {
				test.Errorf("thread = outdated %v line %d original %d", thread.IsOutdated, thread.Line, thread.OriginalLine)
			}
			if thread.DiffSide != github.SideRight {
				test.Errorf("side = %v", thread.DiffSide)
			}
		})
	}
	if threads := questions.threads(testRepository, 8, "old"); len(threads) != 0 {
		test.Errorf("another pull request has %d threads", len(threads))
	}
}

func TestExcerpt(test *testing.T) {
	inliner := newInliner(test)
	file := inliner.diffView().Context().File
	got := excerpt(file, rangeOf(file.Path(), github.SideRight, 14, 14))
	lines := strings.Split(got, "\n")
	if len(lines) == 0 || len(lines) > 2*excerptContext+1 {
		test.Errorf("excerpt has %d lines:\n%s", len(lines), got)
	}
	if excerpt(file, rangeOf("other.go", github.SideRight, 14, 14)) != "" {
		test.Error("excerpt of another file's range")
	}
	if excerpt(nil, rangeOf("a.go", github.SideRight, 1, 1)) != "" {
		test.Error("excerpt without a file")
	}
}

func rangeOf(path string, side github.DiffSide, start, end int) diffview.Range {
	return diffview.Range{Path: path, Side: side, Start: start, End: end}
}
