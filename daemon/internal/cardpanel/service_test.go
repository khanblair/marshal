package cardpanel_test

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/cardpanel"
	"github.com/khanblair/marshal/daemon/internal/localci"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

var testTime = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

// fakeRunner fails every command that holds "fail" and never starts a process.
type fakeRunner struct {
	ran     []string
	failAll bool
}

func (f *fakeRunner) Run(_ context.Context, req localci.RunRequest) (localci.RunResult, error) {
	f.ran = append(f.ran, req.Command)
	return localci.RunResult{Failed: f.failAll || strings.Contains(req.Command, "fail")}, nil
}

type fakeTrees struct{ dir string }

func (f fakeTrees) Worktree(context.Context, string) (string, string, error) { return f.dir, "b", nil }

type fakeAgent struct {
	mu    sync.Mutex
	sent  []string
	fails bool
}

func (f *fakeAgent) Send(_ context.Context, _, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fails {
		return errors.New("no session")
	}
	f.sent = append(f.sent, text)
	return nil
}

func (f *fakeAgent) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

type env struct {
	svc    *cardpanel.Service
	st     *store.Store
	runner *fakeRunner
	agent  *fakeAgent
	files  string
}

const cardID = "card-1"

func newEnv(t *testing.T, language string) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "marshal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := testTime.UnixMilli()
	err = st.Write(ctx, func(q *db.Queries) error {
		if err := q.CreateProject(ctx, db.CreateProjectParams{
			ID: "api", Name: "api", RepoPath: "/tmp/api", DefaultBranch: "main", Language: language,
			PackagesJSON: "[]", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		if err := q.CreateBoard(ctx, db.CreateBoardParams{ID: "board-1", ProjectID: "api", ColumnsJSON: "[]"}); err != nil {
			return err
		}
		if _, err := q.CreateUser(ctx, db.CreateUserParams{ID: "ada", Name: "Ada", CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		return q.CreateCard(ctx, db.CreateCardParams{
			ID: cardID, ProjectID: "api", Number: 1, BoardID: "board-1", Title: "A card",
			State: "working", AgentKind: "claude", PermissionMode: "auto-edits", CreatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	e := &env{st: st, runner: &fakeRunner{}, agent: &fakeAgent{}, files: t.TempDir()}
	e.svc, err = cardpanel.New(cardpanel.Deps{
		Store: st, Worktrees: fakeTrees{dir: t.TempDir()}, Runner: e.runner, Agent: e.agent,
		AttachmentsDir: e.files, Now: func() time.Time { return testTime },
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestAGoCardStartsWithTestLintAndReviewChecks(t *testing.T) {
	e := newEnv(t, "Go")
	got, err := e.svc.Checks(context.Background(), cardID)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, c := range got.Checks {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "Tests pass,Lint clean,Reviewer approval" {
		t.Fatalf("checks = %v", names)
	}
	again, _ := e.svc.Checks(context.Background(), cardID)
	if len(again.Checks) != 3 {
		t.Fatalf("a second read seeded again: %d checks", len(again.Checks))
	}
}

func TestRunningChecksRecordsEachResultWithARunReference(t *testing.T) {
	e := newEnv(t, "Go")
	ctx := context.Background()
	if _, err := e.svc.AddCheck(ctx, cardID, protocol.AddCardCheckRequest{Name: "Bad", Command: "fail now"}); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.RunChecks(ctx, cardID)
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]protocol.CheckStatus{}
	for _, c := range got.Checks {
		status[c.Name] = c.Status
		if c.Kind == "command" && c.RunRef == "" {
			t.Fatalf("%s has no run reference", c.Name)
		}
	}
	if status["Tests pass"] != protocol.CheckStatusPassed || status["Bad"] != protocol.CheckStatusFailed {
		t.Fatalf("statuses = %v", status)
	}
	if status["Reviewer approval"] != protocol.CheckStatusPending {
		t.Fatalf("the review check moved without a review: %v", status)
	}
	if n, _ := e.svc.UnpassedChecks(ctx, cardID); n != 1 {
		t.Fatalf("unpassed = %d, want 1", n)
	}
}

func TestAFailingCheckUnticksTheLineItProved(t *testing.T) {
	e := newEnv(t, "Go")
	ctx := context.Background()
	lists, _ := e.svc.CreateChecklist(ctx, cardID, protocol.CreateChecklistRequest{Name: "Done when"})
	listID := lists.Checklists[0].ID
	lists, _ = e.svc.AddItem(ctx, cardID, listID, protocol.AddChecklistItemRequest{Text: "Tests are green"})
	itemID := lists.Checklists[0].Items[0].ID
	checks, _ := e.svc.RunChecks(ctx, cardID)

	if _, err := e.svc.TickWithCheck(ctx, cardID, listID, itemID, checks.Checks[0].ID); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Checklists(ctx, cardID)
	if it := got.Checklists[0].Items[0]; !it.Done || it.DoneByKind != "agent" {
		t.Fatalf("the proven tick was not kept: %+v", it)
	}
	e.runner.failAll = true
	if _, err := e.svc.RunChecks(ctx, cardID); err != nil {
		t.Fatal(err)
	}
	got, _ = e.svc.Checklists(ctx, cardID)
	if got.Checklists[0].Items[0].Done {
		t.Fatal("the line stayed ticked after its proof failed")
	}
}

func TestAnAgentCannotTickWithACheckThatHasNotPassed(t *testing.T) {
	e := newEnv(t, "Go")
	ctx := context.Background()
	lists, _ := e.svc.CreateChecklist(ctx, cardID, protocol.CreateChecklistRequest{})
	listID := lists.Checklists[0].ID
	lists, _ = e.svc.AddItem(ctx, cardID, listID, protocol.AddChecklistItemRequest{Text: "x"})
	checks, _ := e.svc.Checks(ctx, cardID)
	_, err := e.svc.TickWithCheck(ctx, cardID, listID, lists.Checklists[0].Items[0].ID, checks.Checks[0].ID)
	if err == nil {
		t.Fatal("ticked with a check that never ran")
	}
}

func TestOnlyAPersonTicksAPeopleOnlyList(t *testing.T) {
	e := newEnv(t, "Go")
	ctx := context.Background()
	lists, _ := e.svc.CreateChecklist(ctx, cardID, protocol.CreateChecklistRequest{})
	listID := lists.Checklists[0].ID
	if lists.Checklists[0].Name != "Checklist" {
		t.Fatalf("default name = %q", lists.Checklists[0].Name)
	}
	yes := true
	if _, err := e.svc.UpdateChecklist(ctx, cardID, listID, protocol.UpdateChecklistRequest{PeopleOnly: &yes, Required: &yes}); err != nil {
		t.Fatal(err)
	}
	lists, _ = e.svc.AddItem(ctx, cardID, listID, protocol.AddChecklistItemRequest{Text: "Ship it"})
	itemID := lists.Checklists[0].Items[0].ID

	_, err := e.svc.Tick(ctx, cardID, listID, itemID, true, cardpanel.Actor{Agent: true})
	var perr *protocol.Error
	if !errors.As(err, &perr) || perr.Code != protocol.ErrorCodeForbidden {
		t.Fatalf("the agent's tick answered %v, want forbidden", err)
	}
	if n, _ := e.svc.OpenRequiredItems(ctx, cardID); n != 1 {
		t.Fatalf("open required = %d, want 1", n)
	}
	lists, err = e.svc.Tick(ctx, cardID, listID, itemID, true, cardpanel.Actor{UserID: "ada"})
	if err != nil {
		t.Fatal(err)
	}
	it := lists.Checklists[0].Items[0]
	if !it.Done || it.DoneByKind != "person" || it.DoneByID != "ada" || it.DoneAt == nil {
		t.Fatalf("item = %+v", it)
	}
	if n, _ := e.svc.OpenRequiredItems(ctx, cardID); n != 0 {
		t.Fatalf("open required after the tick = %d", n)
	}
	lists, _ = e.svc.Tick(ctx, cardID, listID, itemID, false, cardpanel.Actor{UserID: "ada"})
	if lists.Checklists[0].Items[0].Done {
		t.Fatal("the line stayed ticked after a reopen")
	}
}

func TestAChecklistOfAnotherCardIsNotFound(t *testing.T) {
	e := newEnv(t, "Go")
	_, err := e.svc.AddItem(context.Background(), cardID, "nope", protocol.AddChecklistItemRequest{Text: "x"})
	var perr *protocol.Error
	if !errors.As(err, &perr) || perr.Code != protocol.ErrorCodeNotFound {
		t.Fatalf("answered %v, want not found", err)
	}
}

func TestOnlyAWholeWordMentionOrAQuestionReachesTheAgent(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{"@agent please look", true},
		{"hey @Agent, look", true},
		{"is this right?", true},
		{"mail me at ann@agentless.dev", false},
		{"ping @ada about it", false},
		{"just a note.", false},
	}
	for _, tc := range cases {
		e := newEnv(t, "Go")
		if _, err := e.svc.Post(context.Background(), cardID, protocol.PostCommentRequest{Body: tc.body}, cardpanel.Actor{UserID: "ada"}); err != nil {
			t.Fatal(err)
		}
		if tc.want {
			waitFor(t, func() bool { return e.agent.count() == 1 })
		} else {
			time.Sleep(20 * time.Millisecond)
			if e.agent.count() != 0 {
				t.Fatalf("%q reached the agent", tc.body)
			}
		}
	}
}

func TestACommentTheAgentTookIsMarkedRead(t *testing.T) {
	e := newEnv(t, "Go")
	ctx := context.Background()
	if _, err := e.svc.Post(ctx, cardID, protocol.PostCommentRequest{Body: "@agent look"}, cardpanel.Actor{UserID: "ada"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		got, _ := e.svc.Comments(ctx, cardID)
		return got.Comments[0].AgentReadAt != nil
	})
}

func TestACommentTheAgentCouldNotTakeStaysUnread(t *testing.T) {
	e := newEnv(t, "Go")
	e.agent.fails = true
	ctx := context.Background()
	if _, err := e.svc.Post(ctx, cardID, protocol.PostCommentRequest{Body: "@agent look"}, cardpanel.Actor{UserID: "ada"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	got, _ := e.svc.Comments(ctx, cardID)
	if got.Comments[0].AgentReadAt != nil {
		t.Fatal("marked read though the agent never got it")
	}
}

func TestFilesAreKeptUnderTheCardAndLinksInTheTextBecomeAttachments(t *testing.T) {
	e := newEnv(t, "Go")
	ctx := context.Background()
	got, err := e.svc.Post(ctx, cardID, protocol.PostCommentRequest{
		Body: "see https://example.com/spec.",
		Attachments: []protocol.NewAttachment{{
			Kind: protocol.AttachmentKindFile, Name: "../../etc/passwd", MimeType: "text/plain",
			Data: base64.StdEncoding.EncodeToString([]byte("hello")),
		}},
	}, cardpanel.Actor{UserID: "ada"})
	if err != nil {
		t.Fatal(err)
	}
	atts := got.Comments[0].Attachments
	if len(atts) != 2 {
		t.Fatalf("attachments = %+v", atts)
	}
	file, err := e.svc.Attachment(ctx, cardID, atts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(file.Path, e.files) || strings.Contains(file.Path, "..") {
		t.Fatalf("the file left its folder: %s", file.Path)
	}
	if b, _ := os.ReadFile(file.Path); string(b) != "hello" {
		t.Fatalf("file holds %q", b)
	}
	var link protocol.Attachment
	for _, a := range atts {
		if a.Kind == protocol.AttachmentKindLink {
			link = a
		}
	}
	if link.URL != "https://example.com/spec" {
		t.Fatalf("link = %+v", link)
	}
	if _, err := e.svc.Attachment(ctx, cardID, link.ID); err == nil {
		t.Fatal("a link was served as a file")
	}
}

func TestAnOversizedFileIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	e := newEnv(t, "Go")
	big := base64.StdEncoding.EncodeToString(make([]byte, protocol.MaxAttachmentBytes+1))
	_, err := e.svc.Post(context.Background(), cardID, protocol.PostCommentRequest{
		Body: "x", Attachments: []protocol.NewAttachment{{Kind: protocol.AttachmentKindFile, Name: "a", Data: big}},
	}, cardpanel.Actor{UserID: "ada"})
	var perr *protocol.Error
	if !errors.As(err, &perr) || perr.Code != protocol.ErrorCodeInvalidArgument {
		t.Fatalf("answered %v", err)
	}
	if got, _ := e.svc.Comments(context.Background(), cardID); len(got.Comments) != 0 {
		t.Fatal("a comment was written")
	}
}

func TestAPersonDeletesOnlyTheirOwnComment(t *testing.T) {
	e := newEnv(t, "Go")
	ctx := context.Background()
	got, _ := e.svc.Post(ctx, cardID, protocol.PostCommentRequest{Body: "mine"}, cardpanel.Actor{UserID: "ada"})
	id := got.Comments[0].ID
	if _, err := e.svc.DeleteComment(ctx, cardID, id, cardpanel.Actor{UserID: "someone"}); err == nil {
		t.Fatal("deleted another person's comment")
	}
	got, err := e.svc.DeleteComment(ctx, cardID, id, cardpanel.Actor{UserID: "ada"})
	if err != nil || len(got.Comments) != 0 {
		t.Fatalf("delete answered %v, %+v", err, got)
	}
}

func TestMembersAreKnownPeopleOnly(t *testing.T) {
	e := newEnv(t, "Go")
	ctx := context.Background()
	if _, err := e.svc.AddMember(ctx, cardID, "ghost"); err == nil {
		t.Fatal("added a person Marshal does not know")
	}
	got, err := e.svc.AddMember(ctx, cardID, "ada")
	if err != nil || len(got.UserIDs) != 1 {
		t.Fatalf("add answered %v, %+v", err, got)
	}
	got, _ = e.svc.AddMember(ctx, cardID, "ada")
	if len(got.UserIDs) != 1 {
		t.Fatal("a second add made a second row")
	}
	got, _ = e.svc.RemoveMember(ctx, cardID, "ada")
	if len(got.UserIDs) != 0 {
		t.Fatal("still a member")
	}
}

func TestAnUnknownCardIsNotFoundEverywhere(t *testing.T) {
	e := newEnv(t, "Go")
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"checks":     func() error { _, err := e.svc.Checks(ctx, "nope"); return err },
		"checklists": func() error { _, err := e.svc.Checklists(ctx, "nope"); return err },
		"comments":   func() error { _, err := e.svc.Comments(ctx, "nope"); return err },
		"members":    func() error { _, err := e.svc.Members(ctx, "nope"); return err },
	} {
		var perr *protocol.Error
		if err := call(); !errors.As(err, &perr) || perr.Code != protocol.ErrorCodeNotFound {
			t.Errorf("%s answered %v", name, err)
		}
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out")
}

func TestTheAgentTicksWithEvidenceAndIsRefusedOnAPeopleOnlyList(t *testing.T) {
	e := newEnv(t, "Go")
	ctx := context.Background()
	lists, _ := e.svc.CreateChecklist(ctx, cardID, protocol.CreateChecklistRequest{})
	listID := lists.Checklists[0].ID
	lists, _ = e.svc.AddItem(ctx, cardID, listID, protocol.AddChecklistItemRequest{Text: "Docs updated"})
	itemID := lists.Checklists[0].Items[0].ID

	if _, err := e.svc.AgentTick(ctx, cardID, itemID, true, "  "); err == nil {
		t.Fatal("ticked with no evidence")
	}
	got, err := e.svc.AgentTick(ctx, cardID, itemID, true, "commit 4f2a9c")
	if err != nil {
		t.Fatal(err)
	}
	if it := got.Checklists[0].Items[0]; !it.Done || it.DoneByKind != "agent" {
		t.Fatalf("item = %+v", it)
	}
	if _, err = e.svc.AgentTick(ctx, cardID, itemID, false, ""); err != nil {
		t.Fatal(err)
	}
	yes := true
	if _, err := e.svc.UpdateChecklist(ctx, cardID, listID, protocol.UpdateChecklistRequest{PeopleOnly: &yes}); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.AgentTick(ctx, cardID, itemID, true, "commit 4f2a9c")
	var perr *protocol.Error
	if !errors.As(err, &perr) || perr.Code != protocol.ErrorCodeForbidden {
		t.Fatalf("answered %v, want forbidden", err)
	}
}

func TestEvidenceThatNamesAPassedCheckIsKeptAsThatCheck(t *testing.T) {
	e := newEnv(t, "Go")
	ctx := context.Background()
	lists, _ := e.svc.CreateChecklist(ctx, cardID, protocol.CreateChecklistRequest{})
	lists, _ = e.svc.AddItem(ctx, cardID, lists.Checklists[0].ID, protocol.AddChecklistItemRequest{Text: "Green"})
	itemID := lists.Checklists[0].Items[0].ID
	if _, err := e.svc.RunChecks(ctx, cardID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AgentTick(ctx, cardID, itemID, true, "tests pass"); err != nil {
		t.Fatal(err)
	}
	e.runner.failAll = true
	if _, err := e.svc.RunChecks(ctx, cardID); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Checklists(ctx, cardID)
	if got.Checklists[0].Items[0].Done {
		t.Fatal("the line stayed ticked after the check it named failed")
	}
}

func TestTheAgentReadsUnreadCommentsOnceAndPostsItsOwn(t *testing.T) {
	e := newEnv(t, "Go")
	ctx := context.Background()
	if _, err := e.svc.Post(ctx, cardID, protocol.PostCommentRequest{Body: "note for later"}, cardpanel.Actor{UserID: "ada"}); err != nil {
		t.Fatal(err)
	}
	first, err := e.svc.UnreadComments(ctx, cardID)
	if err != nil || len(first) != 1 || first[0].Body != "note for later" {
		t.Fatalf("first read = %+v, %v", first, err)
	}
	if again, _ := e.svc.UnreadComments(ctx, cardID); len(again) != 0 {
		t.Fatalf("the same comment was handed over twice: %+v", again)
	}
	posted, err := e.svc.PostAsAgent(ctx, cardID, "Done, see the branch.")
	if err != nil || posted.AuthorKind != protocol.AuthorKindAgent || posted.AuthorID != "" {
		t.Fatalf("posted = %+v, %v", posted, err)
	}
}

func TestTheAgentReadsAKeptTextFileButNotABinaryOne(t *testing.T) {
	e := newEnv(t, "Go")
	ctx := context.Background()
	_, err := e.svc.Post(ctx, cardID, protocol.PostCommentRequest{Body: "files", Attachments: []protocol.NewAttachment{
		{Kind: protocol.AttachmentKindFile, Name: "spec.md", MimeType: "text/markdown", Data: base64.StdEncoding.EncodeToString([]byte("# spec"))},
		{Kind: protocol.AttachmentKindFile, Name: "blob.bin", MimeType: "application/octet-stream", Data: base64.StdEncoding.EncodeToString([]byte{0xff, 0xfe, 0x00})},
	}}, cardpanel.Actor{UserID: "ada"})
	if err != nil {
		t.Fatal(err)
	}
	if text, err := e.svc.ReadAttachment(ctx, cardID, "spec.md"); err != nil || text != "# spec" {
		t.Fatalf("spec.md = %q, %v", text, err)
	}
	if _, err := e.svc.ReadAttachment(ctx, cardID, "blob.bin"); err == nil {
		t.Fatal("a binary file was read as text")
	}
	if _, err := e.svc.ReadAttachment(ctx, cardID, "missing.txt"); err == nil {
		t.Fatal("a file that is not there was found")
	}
}
