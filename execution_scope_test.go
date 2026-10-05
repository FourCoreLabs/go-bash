package gobash

import (
	"context"
	"errors"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestNestedExecutionSharesCommandBudget(t *testing.T) {
	b, err := New(BashOptions{ExecutionLimits: &ExecutionLimits{MaxCommandCount: intPtrTest(2)}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(), `bash -c 'echo inner; echo extra'; echo outer`, ExecOptions{})
	var limit *ExecutionLimitError
	if !errors.As(err, &limit) || limit.Limit != "MaxCommandCount" {
		t.Fatalf("result=%+v err=%v", res, err)
	}
}

func TestNestedExecutionSharesLoopBudget(t *testing.T) {
	b, err := New(BashOptions{ExecutionLimits: &ExecutionLimits{MaxLoopIterations: intPtrTest(2)}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.Exec(context.Background(), `for x in 1; do :; done; bash -c 'for x in 1 2; do :; done'`, ExecOptions{})
	var limit *ExecutionLimitError
	if !errors.As(err, &limit) || limit.Limit != "MaxLoopIterations" {
		t.Fatalf("err=%v", err)
	}
}

func TestNestedExecutionSharesOutputBudget(t *testing.T) {
	b, err := New(BashOptions{ExecutionLimits: &ExecutionLimits{MaxOutputSize: intPtrTest(5)}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(), `echo ab; bash -c 'echo cd'`, ExecOptions{})
	var limit *ExecutionLimitError
	if !errors.As(err, &limit) || limit.Limit != "MaxOutputSize" {
		t.Fatalf("result=%+v err=%v", res, err)
	}
}

func TestNestedArgsRemainScoped(t *testing.T) {
	var got string
	cmd := command.Define("remember", func(_ context.Context, args []string, _ *command.Context) command.Result {
		got = ""
		if len(args) > 1 {
			got = args[1]
		}
		return command.Result{}
	})
	b, err := New(BashOptions{CustomCommands: []command.Command{cmd}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.Exec(context.Background(), `remember; bash -c 'remember'`, ExecOptions{Args: []string{"outer"}})
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("nested dispatch consumed/reused Args: got %q", got)
	}
}
