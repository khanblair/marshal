package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/khanblair/marshal/daemon/internal/cardpanel"
	"github.com/khanblair/marshal/daemon/internal/changereview"
	"github.com/khanblair/marshal/daemon/internal/chats"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/mcpattach"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/pullrequest"
	"github.com/khanblair/marshal/daemon/internal/store"
)

// mergeDeps is what the merge queue is built from.
type mergeDeps struct {
	st       *store.Store
	bus      *events.Bus
	settings config.Settings
	log      *slog.Logger
	core     coreDaemonModules
	chats    *chats.Service
	panel    *cardpanel.Service
	roles    changereview.Roles
	localCI  workflowRunner
	// pullReq opens pull requests on GitHub, and is told which projects may publish.
	pullReq *pullrequest.Service
}

// mergeModules is the merge queue and the pieces the project chats share with it.
type mergeModules struct {
	queue     *integrator.Service
	workspace *integrator.GitWorkspace
	// tools answers the Integrator chat's merge tools; it is the same value as the queue's resolver.
	tools integrator.MergeTools
}

// buildMergeQueue makes the merge queue: finished cards merge into the Integrator's own `integrator`
// branch, then land on the project's integration branch and in the owner's folder. A conflict goes
// to one persistent Integrator agent per project, the pinned Integrator chat, through the resolver.
// The queue has no test runner yet: a clean merge lands, and local CI supplies the runner later.
func buildMergeQueue(d mergeDeps) (mergeModules, error) {
	workspace, err := integrator.NewWorkspace(integrator.WorkspaceDeps{
		Projects: d.core.proj, Git: d.core.git, DataDir: d.settings.DataDir,
	})
	if err != nil {
		return mergeModules{}, fmt.Errorf("start the Integrator workspace: %w", err)
	}
	resolver, err := integrator.NewAgentResolver(integrator.AgentDeps{
		Chat:  chats.NewIntegratorChat(d.chats),
		Asker: ownerAsker{cards: d.core.proj, log: d.log},
		Log:   d.log,
	})
	if err != nil {
		return mergeModules{}, fmt.Errorf("start the Integrator agent: %w", err)
	}
	reviewer, err := changereview.New(d.core.sessions, d.roles, d.core.git)
	if err != nil {
		return mergeModules{}, fmt.Errorf("start the change reviewer: %w", err)
	}
	queue, err := integrator.New(integrator.Deps{
		DirectWorkspace: true,
		Cards:           d.core.proj, Projects: d.core.proj, Git: d.core.git, DataDir: d.settings.DataDir, Log: d.log,
		Checklists: d.panel, Ledger: integrator.NewLedger(d.st), Events: d.bus, Workspace: workspace,
		Resolver: resolver, Reviewer: reviewer, Checker: localChecks{ci: d.localCI},
		Notes: reviewNotes{panel: d.panel}, Worker: d.core.sessions,
	})
	if err != nil {
		return mergeModules{}, fmt.Errorf("start the merge queue: %w", err)
	}
	// A card that reaches Ready to merge goes to the queue by itself (unless merging is paused).
	d.core.proj.SetOnReadyToMerge(queue.Enqueue)
	// Agents may push, and Marshal may publish, only for a project that allows it.
	d.core.sessions.SetPushPolicy(queue)
	d.pullReq.SetPolicy(queue)
	// A card whose agent says it is finished goes through review, checks, and the merge by itself.
	d.core.sessions.SetFinishHandler(queue)
	return mergeModules{queue: queue, workspace: workspace, tools: resolver}, nil
}

// ownerAsker is how the Integrator agent asks the owner a question: the card it is merging goes to
// Needs you with the question. A question with no card (the owner's uncommitted changes) stays in
// the Integrator chat, where the agent's own ask_owner call is already shown.
type ownerAsker struct {
	cards *projects.Service
	log   *slog.Logger
}

// Ask puts the question on the card's Needs you.
func (a ownerAsker) Ask(ctx context.Context, projectID, cardID, text string) error {
	if cardID == "" {
		a.log.Info("the Integrator asked the owner a question with no card", "project_id", projectID)
		return nil
	}
	_, err := a.cards.SetNeeds(ctx, cardID, protocol.NeedsReason{
		Kind: protocol.NeedsReasonKindQuestion, Text: "The Integrator asks: " + text,
	})
	return err
}

// followNewProjects gives every project the pinned Integrator chat the moment it is added, until ctx
// ends. Projects that already exist are covered at start by EnsureAllSystemChats.
func followNewProjects(ctx context.Context, bus *events.Bus, svc *chats.Service, log *slog.Logger) {
	sub := bus.Subscribe(events.Topics(string(protocol.HomeTopic)))
	go func() {
		defer sub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub.C():
				if !ok {
					return
				}
				if ev.Type != string(protocol.EventTypeProjectCreated) {
					continue
				}
				data, isProject := ev.Data.(protocol.ProjectEventData)
				if !isProject {
					continue
				}
				if _, err := svc.EnsureSystemChat(ctx, data.Project.ID, protocol.ChatSystemIntegrator); err != nil {
					log.Warn("could not make the Integrator chat for a new project", "project_id", data.Project.ID, "error", err)
				}
			}
		}
	}()
}

// attachChats gives project chats their own tool server and instructions: the Orchestrator plans
// with the board tools, and the Integrator chat runs in the Integrator's workspace with the merge
// tools.
func attachChats(core coreDaemonModules, attacher *mcpattach.Attacher, mm mergeModules) error {
	core.sessions.SetChatWorkspace(mm.workspace)
	chatAttacher, err := mcpattach.NewChatAttacher(attacher, mcpattach.ChatDeps{
		Harness:    core.sessions.HarnessConfigForChat,
		MergeTools: func() integrator.MergeTools { return mm.tools },
	})
	if err != nil {
		return fmt.Errorf("start the chat tools: %w", err)
	}
	core.sessions.SetChatAttacher(chatAttacher)
	return nil
}
