package mcpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/khanblair/marshal/daemon/internal/integrator"
)

// merge_context, merge_report, and ask_owner: the Integrator chat's tools. The chat's server only
// routes them; internal/integrator's AgentResolver answers them, and it is set after the server is
// built, so each call looks the implementation up. With none set the three say so, in the words a
// model can pass on to the owner, rather than failing in a way it cannot read.

// ErrMergeToolsMissing is what each of the three answers when nothing implements them yet.
var ErrMergeToolsMissing = errors.New("the merge tools are not available")

// mergeTools answers the implementation, or ErrMergeToolsMissing when there is none.
func (s *Server) mergeTools() (integrator.MergeTools, error) {
	if s.deps.MergeTools == nil {
		return nil, ErrMergeToolsMissing
	}
	tools := s.deps.MergeTools()
	if tools == nil {
		return nil, ErrMergeToolsMissing
	}
	return tools, nil
}

// ownTask reads a task the Integrator was sent and checks that it belongs to this chat's project, so
// one project's Integrator cannot read or report another's merge. The merge tools are decided by
// their task id alone, so this is where the chat's identity is held to it.
func (s *Server) ownTask(ctx context.Context, tools integrator.MergeTools, taskID string) (integrator.MergeTask, error) {
	task, err := tools.Context(ctx, taskID)
	if err != nil {
		return integrator.MergeTask{}, err
	}
	if task.ProjectID != s.identity.ProjectID {
		return integrator.MergeTask{}, fmt.Errorf("task %s belongs to another project, and one Integrator chat may not reach across projects", taskID)
	}
	return task, nil
}

// merge_context answers the task the agent was sent, whole.
func (s *Server) mergeContext(ctx context.Context, _ *mcp.CallToolRequest, in integrator.MergeContextArgs) (*mcp.CallToolResult, integrator.MergeTaskView, error) {
	if err := s.allow("merge_context", kindRead); err != nil {
		return nil, integrator.MergeTaskView{}, err
	}
	tools, err := s.mergeTools()
	if err != nil {
		return nil, integrator.MergeTaskView{}, err
	}
	task, err := s.ownTask(ctx, tools, in.TaskID)
	if err != nil {
		return nil, integrator.MergeTaskView{}, err
	}
	return nil, integrator.TaskView(task), nil
}

// mergeReportOut is merge_report's answer.
type mergeReportOut struct {
	// Recorded is true when the verdict reached the merge that is waiting for it.
	Recorded bool `json:"recorded"`
}

// merge_report records the agent's verdict, releasing the merge that waits on the task.
func (s *Server) mergeReport(ctx context.Context, _ *mcp.CallToolRequest, in integrator.MergeReportArgs) (*mcp.CallToolResult, mergeReportOut, error) {
	if err := s.allow("merge_report", kindWrite); err != nil {
		return nil, mergeReportOut{}, err
	}
	tools, err := s.mergeTools()
	if err != nil {
		return nil, mergeReportOut{}, err
	}
	if _, err := s.ownTask(ctx, tools, in.TaskID); err != nil {
		return nil, mergeReportOut{}, err
	}
	if err := tools.Report(ctx, in.TaskID, in.Verdict()); err != nil {
		return nil, mergeReportOut{}, err
	}
	return nil, mergeReportOut{Recorded: true}, nil
}

// askOwnerOut is ask_owner's answer.
type askOwnerOut struct {
	// Asked is true when the question was posted to the owner.
	Asked bool `json:"asked"`
}

// ask_owner posts a question that blocks the agent to the owner.
func (s *Server) askOwner(ctx context.Context, _ *mcp.CallToolRequest, in integrator.AskOwnerArgs) (*mcp.CallToolResult, askOwnerOut, error) {
	if err := s.allow("ask_owner", kindWrite); err != nil {
		return nil, askOwnerOut{}, err
	}
	tools, err := s.mergeTools()
	if err != nil {
		return nil, askOwnerOut{}, err
	}
	if _, err := s.ownTask(ctx, tools, in.TaskID); err != nil {
		return nil, askOwnerOut{}, err
	}
	if err := tools.Ask(ctx, in.TaskID, in.Question); err != nil {
		return nil, askOwnerOut{}, err
	}
	return nil, askOwnerOut{Asked: true}, nil
}
