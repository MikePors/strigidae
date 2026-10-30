package workspace

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/beefis"
	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/diffview"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// Asker sends questions about lines to whatever answers them and hears the
// answers; *beefis.Unit is one. Ask and Receive run in commands, possibly at
// the same time.
type Asker interface {
	Ask(question beefis.Question) error
	Receive() (beefis.Answer, error)
}

// localThreadPrefix starts the identifier of a thread strig made from a
// question. Such threads are shown in the diff like review threads but exist
// only in strig, so actions that write to GitHub refuse them.
const localThreadPrefix = "local:"

// Authors shown on the comments of a question's thread.
const (
	questionAuthor = "you"
	answerAuthor   = "answer (local, not on GitHub)"
)

// excerptContext is how many lines around the asked ones go in the excerpt.
const excerptContext = 3

// Questions remembers the questions asked from workspaces and their answers
// for as long as strig runs, so an answer that arrives while its pull request
// is closed is there when it is opened again. It is the root's
// userinterface.Companion: the root records answers through it whichever
// view is on top. Only the UI goroutine touches it.
type Questions struct {
	asker  Asker
	next   int
	asked  []*askedQuestion
	closed bool
}

// askedQuestion is one question and how answering it stands.
type askedQuestion struct {
	question   beefis.Question
	repository github.Repository
	askedAt    time.Time
	// answered is set when an answer or a failure arrived.
	answered bool
	answer   string
	failure  string
}

// NewQuestions returns an empty record of questions asked through asker.
func NewQuestions(asker Asker) *Questions { return &Questions{asker: asker} }

// WithQuestions lets the workspace ask about lines with :ask and show the
// answers. Without it :ask says how to turn asking on.
func WithQuestions(questions *Questions) Option {
	return func(view *View) { view.questions = questions }
}

// askSentMessage is the result of sending a question.
type askSentMessage struct {
	questions         *Questions
	requestIdentifier string
	err               error
}

// answerMessage is an answer heard, or why listening failed.
type answerMessage struct {
	questions *Questions
	answer    beefis.Answer
	err       error
}

// Listen implements userinterface.Companion: it waits for the next answer.
func (questions *Questions) Listen() tea.Cmd {
	asker := questions.asker
	return func() tea.Msg {
		answer, err := asker.Receive()
		return answerMessage{questions: questions, answer: answer, err: err}
	}
}

// Heard implements userinterface.Companion: it records answers and failed
// sends, and keeps listening until the connection closes.
func (questions *Questions) Heard(message tea.Msg) (heard bool, next tea.Cmd) {
	switch message := message.(type) {
	case askSentMessage:
		if message.questions != questions {
			return false, nil
		}
		if message.err != nil {
			questions.fail(message.requestIdentifier, "could not ask: "+terminaltext.SafeError(message.err).Error())
			return true, userinterface.Error(terminaltext.SafeError(fmt.Errorf("ask: %w", message.err)))
		}
		return true, nil
	case answerMessage:
		if message.questions != questions {
			return false, nil
		}
		if errors.Is(message.err, io.EOF) || errors.Is(message.err, io.ErrClosedPipe) {
			questions.closed = true
			return true, userinterface.Error(errors.New("beefis closed the connection: no more answers"))
		}
		if message.err != nil {
			return true, tea.Batch(userinterface.Error(terminaltext.SafeError(message.err)), questions.Listen())
		}
		questions.record(message.answer)
		return true, questions.Listen()
	}
	return false, nil
}

// add records a new question and returns it with its identifier set.
func (questions *Questions) add(question beefis.Question, repository github.Repository, now time.Time) beefis.Question {
	questions.next++
	question.RequestIdentifier = "q" + strconv.Itoa(questions.next)
	questions.asked = append(questions.asked, &askedQuestion{question: question, repository: repository, askedAt: now})
	return question
}

func (questions *Questions) find(requestIdentifier string) *askedQuestion {
	index := slices.IndexFunc(questions.asked, func(asked *askedQuestion) bool {
		return asked.question.RequestIdentifier == requestIdentifier
	})
	if index < 0 {
		return nil
	}
	return questions.asked[index]
}

// record stores an answer; one to a question strig never asked is dropped.
func (questions *Questions) record(answer beefis.Answer) {
	asked := questions.find(answer.RequestIdentifier)
	if asked == nil {
		return
	}
	asked.answered, asked.answer, asked.failure = true, answer.Text, answer.Error
}

func (questions *Questions) fail(requestIdentifier, failure string) {
	if asked := questions.find(requestIdentifier); asked != nil {
		asked.answered, asked.failure = true, failure
	}
}

// threads returns the questions about one pull request as local threads. A
// question asked on another head is outdated: its lines may have moved.
func (questions *Questions) threads(repository github.Repository, number int, head string) []github.ReviewThread {
	var threads []github.ReviewThread
	for _, asked := range questions.asked {
		if asked.repository == repository && asked.question.Number == number {
			threads = append(threads, asked.thread(head))
		}
	}
	return threads
}

func (asked *askedQuestion) thread(head string) github.ReviewThread {
	question := asked.question
	side := github.DiffSide(question.Side)
	thread := github.ReviewThread{
		Identifier: localThreadPrefix + question.RequestIdentifier,
		Path:       question.Path, Line: question.Line, OriginalLine: question.Line,
		StartLine: question.StartLine, OriginalStartLine: question.StartLine,
		DiffSide: side, StartDiffSide: side,
	}
	if head != question.Head {
		thread.IsOutdated, thread.Line, thread.StartLine = true, 0, 0
	}
	thread.Comments = []github.ReviewComment{
		{Identifier: thread.Identifier + ":question", Author: questionAuthor, Body: question.Question, CreatedAt: asked.askedAt},
		{Identifier: thread.Identifier + ":answer", Author: answerAuthor, Body: asked.body(), CreatedAt: asked.askedAt},
	}
	return thread
}

func (asked *askedQuestion) body() string {
	switch {
	case !asked.answered:
		return "_Waiting for an answer…_"
	case asked.failure != "":
		return "_No answer: " + asked.failure + "_"
	}
	return asked.answer
}

// isLocalThread reports whether a thread is one strig made from a question.
func isLocalThread(thread github.ReviewThread) bool {
	return strings.HasPrefix(thread.Identifier, localThreadPrefix)
}

// showThreads gives the diff pane the review threads and the local ones.
func (view *View) showThreads() {
	threads := view.threads
	if view.questions != nil {
		threads = append(slices.Clone(threads), view.questions.threads(view.repository, view.number, view.diffHead)...)
	}
	view.filePane.SetThreads(threads)
}

// checkQuestion refuses :ask without a question.
func checkQuestion(arguments string) tea.Cmd {
	if strings.TrimSpace(arguments) == "" {
		return userinterface.Info("usage: :ask <question>")
	}
	return nil
}

// ask sends a question about the line or selection under the diff cursor
// and shows it there, waiting for its answer.
func (view *View) ask(text string, lines diffview.Range) tea.Cmd {
	if view.questions == nil {
		return userinterface.Info("asking needs strig started with --beefis in a beefis pane")
	}
	if view.questions.closed {
		return userinterface.Info("beefis closed the connection: restart strig to ask again")
	}
	question := beefis.Question{
		Repository: view.repository.String(), Number: view.number, Head: view.diffHead,
		Path: lines.Path, Side: string(lines.Side), Line: lines.End, Question: text,
		Excerpt: excerpt(view.filePane.Context().File, lines),
	}
	if lines.Start != lines.End {
		question.StartLine = lines.Start
	}
	view.filePane.VisualEnd()
	question = view.questions.add(question, view.repository, view.now())
	view.showThreads()
	view.filePane.SetThreadExpanded(localThreadPrefix+question.RequestIdentifier, true)
	questions, asker := view.questions, view.questions.asker
	return func() tea.Msg {
		return askSentMessage{questions: questions, requestIdentifier: question.RequestIdentifier, err: asker.Ask(question)}
	}
}

// answered shows an answer that arrived for this pull request.
func (view *View) answered(message answerMessage) tea.Cmd {
	if message.questions != view.questions || message.err != nil {
		return nil
	}
	asked := view.questions.find(message.answer.RequestIdentifier)
	if asked == nil || asked.repository != view.repository || asked.question.Number != view.number {
		return nil
	}
	view.showThreads()
	return userinterface.Info("answer for " + terminaltext.Clean(asked.question.Path) + ":" + strconv.Itoa(asked.question.Line))
}

// askFailed shows a question that could not be sent.
func (view *View) askFailed(message askSentMessage) {
	if message.questions == view.questions && message.err != nil {
		view.showThreads()
	}
}

// excerpt is the diff lines of lineRange with a few lines around them, as a
// unified diff fragment, or "" when the file is not the range's.
func excerpt(file *diff.File, lineRange diffview.Range) string {
	if file == nil || file.Path() != lineRange.Path {
		return ""
	}
	first, last := lineRange.Start-excerptContext, lineRange.End+excerptContext
	var builder strings.Builder
	for _, hunk := range file.Hunks {
		for _, line := range hunk.Lines {
			number := line.New
			if lineRange.Side == github.SideLeft {
				number = line.Old
			}
			if number < first || number > last {
				continue
			}
			builder.WriteString(linePrefix(line.Kind) + line.Text + "\n")
		}
	}
	return strings.TrimSuffix(builder.String(), "\n")
}

func linePrefix(kind diff.LineKind) string {
	switch kind {
	case diff.Add:
		return "+"
	case diff.Delete:
		return "-"
	}
	return " "
}
