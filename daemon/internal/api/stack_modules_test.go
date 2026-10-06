package api_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/accounts"
	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/auditlog"
	"github.com/khanblair/marshal/daemon/internal/cardhistory"
	"github.com/khanblair/marshal/daemon/internal/cardpanel"
	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/chats"
	"github.com/khanblair/marshal/daemon/internal/ci"
	"github.com/khanblair/marshal/daemon/internal/connectiontest"
	"github.com/khanblair/marshal/daemon/internal/dashboard"
	"github.com/khanblair/marshal/daemon/internal/devices"
	"github.com/khanblair/marshal/daemon/internal/diff"
	"github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/localci"
	"github.com/khanblair/marshal/daemon/internal/memory"
	"github.com/khanblair/marshal/daemon/internal/notify"
	"github.com/khanblair/marshal/daemon/internal/preview"
	"github.com/khanblair/marshal/daemon/internal/providers"
	"github.com/khanblair/marshal/daemon/internal/pullrequest"
	"github.com/khanblair/marshal/daemon/internal/quality"
	"github.com/khanblair/marshal/daemon/internal/review"
	"github.com/khanblair/marshal/daemon/internal/roles"
	"github.com/khanblair/marshal/daemon/internal/schedules"
	"github.com/khanblair/marshal/daemon/internal/search"
	"github.com/khanblair/marshal/daemon/internal/session"
	"github.com/khanblair/marshal/daemon/internal/settings"
)

// startModules builds the session manager and the server over what is already open, and starts
// serving. restart calls it again over the same store and data folder.
func (st *stack) startModules() {
	t := st.t
	t.Helper()
	deps := api.Deps{Store: st.store, Bus: st.bus, Dev: st.dev, Limits: st.cfg.limits,
		Tailnet: st.cfg.tailnet, HostTailscale: st.cfg.hostTailscale, Funnel: st.cfg.funnel}
	if !st.cfg.noIntegrations {
		// The connections Marshal is set up with apart from model providers, built the way
		// cmd/marshald builds it. Its receiver is the stack's webhook route, and its sink is the
		// recorder, so a test can prove a verified delivery reached the daemon. The secret is read
		// from the keychain on every delivery, so the connection is saved below and not configured
		// here.
		if st.webhookSink == nil {
			st.webhookSink = &webhookRecorder{}
		}
		// A stack that is not given a fake GitHub points at a closed local port, so no test reaches the network.
		githubBase := st.cfg.githubBaseURL
		if githubBase == "" {
			githubBase = "http://127.0.0.1:1"
		}
		opts := integrations.Options{
			Logger: st.log, Now: st.now, Tester: st.cfg.integrationsTester,
			TrelloBaseURL: st.cfg.trelloBaseURL,
			GoogleAuthURL: st.cfg.google.auth, GoogleTokenURL: st.cfg.google.token, GCalBaseURL: st.cfg.google.calendar,
			GoogleFilesBaseURL: st.cfg.google.files,
			GoogleRevokeURL:    "http://127.0.0.1:1/revoke", NoBundledGoogleClient: true,
			GoogleClientID: st.cfg.google.clientID, GoogleClientSecret: st.cfg.google.clientSecret,
			GCalRedirectURL:  "http://127.0.0.1:47801/v1/integrations/gcal/callback",
			GitHubAPIBaseURL: githubBase, GitHubAuthBaseURL: githubBase,
		}
		svc, err := integrations.New(st.store, st.keychain, opts)
		if err != nil {
			t.Fatalf("make the integrations service: %v", err)
		}
		svc.SetMonitor(st.webhookSink)
		// A believed Trello delivery is applied to the stack's own board, the way cmd/marshald
		// attaches it, so the Trello route test proves the import end to end rather than logging an
		// ignored delivery.
		if !st.cfg.noProjects {
			svc.SetTrelloCards(stackTrelloCards{proj: st.proj})
		}
		st.integrations = svc
		deps.Integrations = svc
		deps.Webhooks = svc.Receiver()
		if st.cfg.webhookSecret != "" {
			st.connectGitHub(t, st.cfg.webhookSecret)
		}
	}
	if !st.cfg.noHistory {
		// One history store is shared by the session manager that writes it and the service that
		// reads it back, the way cmd/marshald shares one.
		if st.hist == nil {
			hist, err := history.New(st.store, history.WithClock(st.now))
			if err != nil {
				t.Fatalf("make the history store: %v", err)
			}
			st.hist = hist
		}
		histSvc, err := cardhistory.New(cardhistory.Deps{Store: st.store, History: st.hist})
		if err != nil {
			t.Fatalf("make the card history service: %v", err)
		}
		deps.History = histSvc
	}
	if !st.cfg.noDashboard {
		home, err := dashboard.New(dashboard.Deps{Store: st.store}, dashboard.WithLogger(st.log))
		if err != nil {
			t.Fatalf("make the dashboard service: %v", err)
		}
		deps.Dashboard = home
	}
	if !st.cfg.noProjects {
		deps.Projects = st.proj
	}
	// The memory module (B7.2, B7.4, B7.5): the one note each card keeps in the vault, the files a
	// card has claimed, and the search over the notes and the past sessions. Its vault is a folder
	// under the stack's own data folder, which is a temporary directory - a test never writes into
	// the real `<data>/vault`.
	if !st.cfg.noMemory && !st.cfg.noProjects {
		mem, err := memory.New(memory.Deps{Store: st.store, Cards: st.proj, Root: st.vault},
			memory.WithClock(st.now))
		if err != nil {
			t.Fatalf("make the memory module: %v", err)
		}
		deps.Memory = mem
		st.mem = mem
	}
	if !st.cfg.noSchedules {
		// The scheduler (B8.1), built the way cmd/marshald builds it. Its clock is the stack's own,
		// so a schedule a test saves carries the time the stack was fixed at. It is never started
		// here: a test drives the routes, not a cron firing at the wall clock's pace.
		st.schedules = schedules.NewService(st.store, st.log, schedules.WithClock(st.now))
		deps.Schedules = st.schedules
	}
	if !st.cfg.noDevices {
		// The paired-devices service (B9.1, B9.2), built the way cmd/marshald builds it: over the
		// store, on the stack's own clock, so a test can let a code expire without waiting it out.
		st.devices = devices.NewService(st.store,
			devices.WithLogger(st.log), devices.WithClock(st.now))
		deps.Devices = st.devices
	}
	if !st.cfg.noDiff && !st.cfg.noProjects {
		// The diff module reads a card's worktree through the projects module, so a stack without
		// projects has no diff service either.
		cardDiff, err := diff.New(diff.Deps{Projects: st.proj, Git: st.git}, diff.WithLogger(st.log))
		if err != nil {
			t.Fatalf("make the card diff service: %v", err)
		}
		deps.Diff = cardDiff
	}
	if !st.cfg.noSessions {
		// One audit recorder serves the session manager and the CI monitor, as cmd/marshald wires
		// it. It is stateless, so one is shared rather than each module making its own.
		cfg := session.Config{
			DataDir: st.dataDir, Logger: st.log, ResumeMode: st.cfg.resumeMode, History: st.hist,
			Terminals: st.cfg.terminals, Audit: st.audit,
			// The session manager reads the stack's own clock, as the history store, the accounts
			// service, and the server already do, so st.advance moves everyone's idea of "now" at
			// once. That is what lets a test drive the idle timer of Phase 5 without waiting it out.
			Now: st.now,
		}
		// A stack with no history has no plan store either: plans are messages in the history, so
		// the manager is left to make its own from the store it already has rather than handed a
		// nil one through the interface.
		if st.hist != nil {
			cfg.Plans = st.hist
		}
		mgr, err := session.NewManager(st.store, st.bus, st.proj, st.reg, st.git, cfg)
		if err != nil {
			t.Fatalf("make the session manager: %v", err)
		}
		st.mgr = mgr
		st.closeMgr = sync.OnceFunc(func() {
			if err := mgr.Close(); err != nil {
				t.Errorf("close the session manager: %v", err)
			}
		})
		t.Cleanup(st.closeMgr)
		deps.Sessions = mgr
	}
	if !st.cfg.noChats {
		// The chats service is given the session manager when there is one, as cmd/marshald gives
		// it: the manager starts a chat's agent on its first message, and puts it to sleep or ends
		// it when the chat is archived or deleted.
		chatDeps := chats.Deps{Store: st.store, Bus: st.bus}
		if deps.Sessions != nil {
			chatDeps.Sessions = deps.Sessions
		}
		projectChats, err := chats.New(chatDeps, chats.WithLogger(st.log))
		if err != nil {
			t.Fatalf("make the chats service: %v", err)
		}
		deps.Chats = projectChats
		if !st.cfg.noSearch && !st.cfg.noProjects && st.mem != nil {
			// Search reads through the projects, chats, and memory services, so a stack without any
			// of them has no search either.
			finder, err := search.New(search.Deps{
				Projects: st.proj, Chats: projectChats, Sessions: st.mem, Notes: st.mem,
			})
			if err != nil {
				t.Fatalf("make the search service: %v", err)
			}
			deps.Search = finder
		}
	}
	if !st.cfg.noAccounts {
		you, err := accounts.New(accounts.Deps{Store: st.store, Bus: st.bus, Projects: st.proj, DataDir: st.dataDir},
			accounts.WithLogger(st.log), accounts.WithClock(st.now))
		if err != nil {
			t.Fatalf("make the accounts service: %v", err)
		}
		deps.Accounts = you
	}
	if !st.cfg.noAudit {
		auditRead, err := auditlog.New(auditlog.Deps{Store: st.store}, auditlog.WithClock(st.now))
		if err != nil {
			t.Fatalf("make the audit log service: %v", err)
		}
		deps.Auditlog = auditRead
	}
	if !st.cfg.noCatalog {
		deps.Catalog = st.cfg.catalog
		if deps.Catalog == nil {
			deps.Catalog = catalog.NewStub()
		}
	}
	if !st.cfg.noProviders {
		// The keychain is an in-memory one: a test never reads or writes this machine's real
		// keychain. The client factory is a fake, and that is what keeps a provider call out of
		// these tests: saving a key tests it, and so does pressing Test, so a real factory would
		// dial the provider named in the request. The usage recorder is wired as the daemon wires
		// it, over a fixed reader so a row's id is reproducible.
		svc, err := providers.New(st.keychain, providers.Options{
			Logger: st.log, Recorder: providers.NewStoreRecorder(st.store, bytes.NewReader(make([]byte, 64))),
			Now: st.now, Build: fakeProviderBuild(st.cfg.providerFailure),
		})
		if err != nil {
			t.Fatalf("make the provider service: %v", err)
		}
		deps.Providers = svc
	}
	if !st.cfg.noCostLimits {
		deps.CostLimits = providers.NewLimits(st.store)
	}
	if !st.cfg.noConnectionTests {
		deps.ConnectionTests = connectiontest.New(st.store,
			connectiontest.Options{Logger: st.log, Now: st.now})
	}
	if !st.cfg.noRoles {
		// The roles service is built and seeded the way cmd/marshald does it, so the API tests see
		// Marshal's eight starter roles without asking for them.
		roleSvc, err := roles.New(roles.Deps{Store: st.store}, roles.WithLogger(st.log), roles.WithClock(st.now))
		if err != nil {
			t.Fatalf("make the roles service: %v", err)
		}
		if err := roleSvc.EnsureStarters(context.Background()); err != nil {
			t.Fatalf("add the starter roles: %v", err)
		}
		deps.Roles = roleSvc
		// The roles service is the manager's reader of a card's role ceilings, as cmd/marshald
		// wires it, so an API test can drive the harness by naming a role on a card.
		if st.mgr != nil {
			st.mgr.SetRoleLimits(roleSvc)
		}
	}
	if !st.cfg.noCI && !st.cfg.noProjects && !st.cfg.noRoles {
		// The CI monitor is built the way cmd/marshald builds it: over the store, the projects
		// service, the roles service whose ceilings it counts rounds against, and real Git, over a
		// forge client with a token. No forge call is made in the auth test, so the address the
		// client points at never matters here.
		client, err := github.NewTokenClient("test-token", github.WithBaseURL("http://127.0.0.1:0"))
		if err != nil {
			t.Fatalf("make the GitHub client: %v", err)
		}
		ciDeps := ci.Deps{
			Store: st.store, Cards: st.proj, Projects: st.proj, Git: st.git,
			Roles: deps.Roles, Bus: st.bus, Audit: st.audit,
			Options: ci.Options{Logger: st.log, Now: st.now, Forge: client},
		}
		// The worker is set only when there is a session manager: a nil *session.Manager in that
		// interface would be non-nil to the monitor and a call on it would panic.
		if st.mgr != nil {
			ciDeps.Worker = st.mgr
		}
		ciSvc, err := ci.New(ciDeps)
		if err != nil {
			t.Fatalf("make the CI monitor: %v", err)
		}
		deps.CI = ciSvc
		// A verified delivery reaches the CI monitor through the connections service, as
		// cmd/marshald wires it. The recorder keeps its own copy for the webhook tests.
		if st.webhookSink != nil {
			st.webhookSink.setInner(ciSvc)
		}
	}
	if !st.cfg.noLocalCI && !st.cfg.noProjects {
		// The local-CI runner is built the way cmd/marshald builds it: over the projects service,
		// which reads the card's worktree. Its command runner is left at its default, so a test
		// that does not want a child process gives a fake through withLocalCIRunner.
		svc, err := localci.New(localci.Deps{
			Projects: st.proj, Log: st.log, Now: st.now, Runner: st.cfg.localCIRunner,
		})
		if err != nil {
			t.Fatalf("make the local CI service: %v", err)
		}
		deps.LocalCI = svc
	}
	if !st.cfg.noCardPanel && !st.cfg.noProjects {
		// The card panel, built the way cmd/marshald builds it. Its runner is the local-CI one, so a
		// check a test runs never starts a process, and its files live under the stack's data folder.
		panel, err := cardpanel.New(cardpanel.Deps{
			Store: st.store, Bus: st.bus, Worktrees: st.proj, Runner: st.cfg.localCIRunner,
			Agent: st.cfg.panelAgent, AttachmentsDir: filepath.Join(st.dataDir, "attachments"),
			Log: st.log, Now: st.now,
		})
		if err != nil {
			t.Fatalf("make the card panel: %v", err)
		}
		deps.CardPanel = panel
	}
	if !st.cfg.noPreview && !st.cfg.noProjects {
		// The preview module is built the way cmd/marshald builds it: over the projects service,
		// which reads the card's worktree and the project's dev command, and the daemon's data
		// folder, where the shots and the browser profiles live. Every seam is left at its real
		// default, so a test that does not want a dev server or a browser gives fakes through
		// withPreviewOptions.
		svc, err := preview.New(preview.Deps{
			Projects: st.proj, Bus: st.bus, DataDir: st.dataDir, Log: st.log, Now: st.now,
			Options: st.cfg.previewOptions,
		})
		if err != nil {
			t.Fatalf("make the preview module: %v", err)
		}
		deps.Preview = svc
		// The dev servers stop once the server has stopped answering, and before the bus and the
		// store close, as the daemon stops them when it returns.
		t.Cleanup(svc.Close)
	}
	if !st.cfg.noPullRequests && !st.cfg.noProjects {
		// The pull-request service is built the way cmd/marshald builds it, over a client with a
		// token. No call is made in the auth test, so the address the client points at never
		// matters here.
		client, err := github.NewTokenClient("test-token", github.WithBaseURL("http://127.0.0.1:0"))
		if err != nil {
			t.Fatalf("make the GitHub client: %v", err)
		}
		prSvc, err := pullrequest.New(pullrequest.Deps{
			Client: client, Cards: st.proj, Projects: st.proj, Git: st.git, Log: st.log,
		})
		if err != nil {
			t.Fatalf("make the pull-request service: %v", err)
		}
		deps.PullRequests = prSvc
	}
	if !st.cfg.noReview && !st.cfg.noProjects && !st.cfg.noRoles {
		// The review service is built the way cmd/marshald builds it: the ChecksReviewer over a
		// client with a token, reading the Reviewer role the stack just seeded. No forge call is
		// made in the auth test, so the address the client points at never matters here.
		client, err := github.NewTokenClient("test-token", github.WithBaseURL("http://127.0.0.1:0"))
		if err != nil {
			t.Fatalf("make the GitHub client: %v", err)
		}
		checker, err := review.NewChecksReviewer(client)
		if err != nil {
			t.Fatalf("make the checks reviewer: %v", err)
		}
		reviewSvc, err := review.New(review.Deps{
			Cards: st.proj, Projects: st.proj, Roles: deps.Roles, Git: st.git,
			Forge: client, Reviews: checker, Log: st.log,
		})
		if err != nil {
			t.Fatalf("make the review service: %v", err)
		}
		deps.Review = reviewSvc
	}
	if !st.cfg.noIntegration {
		deps.Integration = st.cfg.integration
		if deps.Integration == nil {
			deps.Integration = &fakeMergeQueue{}
		}
	}
	// The opener is always a fake, so no test can start the machine's file manager or editor.
	deps.Opener = st.cfg.opener
	if deps.Opener == nil {
		deps.Opener = &recordingOpener{}
	}
	if !st.cfg.noIntegrator && !st.cfg.noProjects {
		// The merge queue is built the way cmd/marshald builds it, with no test runner.
		queue, err := integrator.New(integrator.Deps{
			Cards: st.proj, Projects: st.proj, Git: st.git, DataDir: st.dataDir, Log: st.log,
		})
		if err != nil {
			t.Fatalf("make the merge queue: %v", err)
		}
		deps.Integrator = queue
	}
	if !st.cfg.noSleepSettings {
		// The sleep settings service is built the way cmd/marshald builds it: over the store, and
		// handed to the session manager as its reader of the idle, warning, and keep-awake times, so
		// the idle timer and the settings screen read the same rows.
		sleepSvc, err := settings.New(st.store)
		if err != nil {
			t.Fatalf("make the settings service: %v", err)
		}
		deps.SleepSettings = sleepSvc
		if !st.cfg.noAlerts {
			deps.Alerts = alertSettingsOver(t, sleepSvc)
		}
		if st.mgr != nil {
			st.mgr.SetSleepSettings(sleepSvc)
		}
	}
	if !st.cfg.noQuality && !st.cfg.noProjects {
		// The quality module is built the way cmd/marshald builds it: over the store, the projects
		// service, real Git, and the session manager as its way of reaching a card's agent. Its
		// linter builder is left at its default, so a test that drives a real check without
		// wanting a child process passes one of its own through the module.
		//
		// The worker is set only when there is a session manager: a nil *session.Manager in that
		// interface would be non-nil to the module and a call on it would panic, so a stack built
		// without sessions is the one stack that really has nobody to ask and answers the
		// "no agent" refusal instead.
		qualityDeps := quality.Deps{
			Store: st.store, Cards: st.proj, Projects: st.proj, Git: st.git,
			Bus: st.bus, Log: st.log,
		}
		if st.mgr != nil {
			qualityDeps.Worker = st.mgr
		}
		qualitySvc, err := quality.New(qualityDeps)
		if err != nil {
			t.Fatalf("make the quality module: %v", err)
		}
		deps.Quality = qualitySvc
		// The module is the projects service's review gate, as cmd/marshald wires it, so an API test
		// can drive the move to review through a card whose changes have a blocking smell.
		st.proj.SetReviewGate(qualitySvc)
	}
	st.server = api.New(st.settings, st.log, st.now, deps)
	server := st.server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener) }()
	st.stopServer = sync.OnceFunc(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
		st.client.CloseIdleConnections()
	})
	t.Cleanup(st.stopServer)
	st.base = "http://" + listener.Addr().String()
	st.client = &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 30 * time.Second}
}

// alertSettingsOver builds the alert settings the way cmd/marshald builds them: a router that sends
// nowhere, over the settings service as its store of choices.
func alertSettingsOver(t *testing.T, store notify.RouteStore) *notify.Alerts {
	t.Helper()
	router, err := notify.New(notify.SenderFunc(func(context.Context, notify.Channel, chatbot.Notice) error { return nil }), notify.Options{})
	if err != nil {
		t.Fatalf("make the notification router: %v", err)
	}
	alerts, err := notify.NewAlerts(router, store, nil, nil)
	if err != nil {
		t.Fatalf("make the alert settings: %v", err)
	}
	return alerts
}
