package beefis

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type closingBuffer struct {
	strings.Builder
	closed bool
}

func (buffer *closingBuffer) Close() error {
	buffer.closed = true
	return nil
}

func TestAskWritesOneEventLine(test *testing.T) {
	for _, testCase := range []struct {
		name     string
		question Question
		want     string
	}{
		{
			"single line",
			Question{RequestIdentifier: "q1", Repository: "o/r", Number: 7, Head: "abc", Path: "a.go", Side: "RIGHT", Line: 3, Question: "what is this"},
			`{"name":"ask","data":{"requestIdentifier":"q1","repository":"o/r","number":7,"head":"abc","path":"a.go","side":"RIGHT","line":3,"question":"what is this"}}` + "\n",
		},
		{
			"range with excerpt",
			Question{RequestIdentifier: "q2", Repository: "o/r", Number: 7, Head: "abc", Path: "a.go", Side: "LEFT", Line: 5, StartLine: 2, Excerpt: "-x\n-y", Question: "why\nremoved"},
			`{"name":"ask","data":{"requestIdentifier":"q2","repository":"o/r","number":7,"head":"abc","path":"a.go","side":"LEFT","line":5,"startLine":2,"excerpt":"-x\n-y","question":"why\nremoved"}}` + "\n",
		},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			buffer := &closingBuffer{}
			unit := New(buffer, strings.NewReader(""))
			if err := unit.Ask(testCase.question); err != nil {
				test.Fatal(err)
			}
			if diff := cmp.Diff(testCase.want, buffer.String()); diff != "" {
				test.Errorf("written (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReceive(test *testing.T) {
	for _, testCase := range []struct {
		name    string
		input   string
		want    []Answer
		wantErr string
	}{
		{"nothing", "", nil, ""},
		{
			"answers in order",
			`{"name":"answer","data":{"requestIdentifier":"q1","text":"a method call"}}` + "\n" +
				`{"name":"answer","data":{"requestIdentifier":"q2","text":"","error":"no claude here"}}` + "\n",
			[]Answer{{RequestIdentifier: "q1", Text: "a method call"}, {RequestIdentifier: "q2", Error: "no claude here"}},
			"",
		},
		{
			"other commands and blank lines skipped",
			"\n" + `{"name":"other","data":{}}` + "\n" + `{"name":"answer","data":{"requestIdentifier":"q1","text":"x"}}`,
			[]Answer{{RequestIdentifier: "q1", Text: "x"}},
			"",
		},
		{"not JSON", "nope\n", nil, "read beefis command"},
		{"bad answer", `{"name":"answer","data":{"text":7}}` + "\n", nil, "read beefis answer"},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			unit := New(&closingBuffer{}, strings.NewReader(testCase.input))
			var got []Answer
			for {
				answer, err := unit.Receive()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					if testCase.wantErr == "" || !strings.Contains(err.Error(), testCase.wantErr) {
						test.Fatalf("error %v, want one containing %q", err, testCase.wantErr)
					}
					return
				}
				got = append(got, answer)
			}
			if testCase.wantErr != "" {
				test.Fatalf("no error, want one containing %q", testCase.wantErr)
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				test.Errorf("answers (-want +got):\n%s", diff)
			}
		})
	}
}

func TestManifestIsValidJSON(test *testing.T) {
	var decoded struct {
		Tool   string `json:"tool"`
		Events []struct {
			Name string `json:"name"`
		} `json:"events"`
		Commands []struct {
			Name string `json:"name"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(manifest), &decoded); err != nil {
		test.Fatal(err)
	}
	if decoded.Tool != "strig" || len(decoded.Events) != 1 || decoded.Events[0].Name != askEvent ||
		len(decoded.Commands) != 1 || decoded.Commands[0].Name != answerCommand {
		test.Errorf("manifest = %+v", decoded)
	}
}

func TestCloseClosesTheWriter(test *testing.T) {
	buffer := &closingBuffer{}
	if err := New(buffer, strings.NewReader("")).Close(); err != nil {
		test.Fatal(err)
	}
	if !buffer.closed {
		test.Error("writer not closed")
	}
}

func TestInPane(test *testing.T) {
	environment := map[string]string{SessionVariable: "x"}
	if !InPane(func(name string) string { return environment[name] }) {
		test.Error("InPane = false with the session variable set")
	}
	if InPane(func(string) string { return "" }) {
		test.Error("InPane = true without it")
	}
}
