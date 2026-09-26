// biome-ignore-all lint/style/noMagicNumbers: seed data table of diff line numbers from the prototype
import { type CardKey, cardLabel } from "../card-key";
import type { Card, DiffFile } from "../types";
import { filesFor } from "./files";

const DIFF_41: DiffFile[] = [
  {
    path: "internal/auth/refresh.go",
    add: 38,
    del: 9,
    hunks: [
      {
        h: "@@ -41,18 +41,35 @@ type Client struct",
        lines: [
          [" ", 41, "type Client struct {"],
          [" ", 42, "\thttp   *http.Client"],
          [" ", 43, "\tissuer string"],
          ["+", 44, "\tgroup  singleflight.Group"],
          [" ", 45, "}"],
          [" ", 46, ""],
          ["-", 47, "func (c *Client) Refresh(ctx context.Context) (Token, error) {"],
          ["-", 48, "\treturn c.refreshOnce(ctx)"],
          ["+", 47, "func (c *Client) Refresh(ctx context.Context) (Token, error) {"],
          ["+", 48, '\tv, err, _ := c.group.Do("refresh", func() (any, error) {'],
          ["+", 49, "\t\treturn c.refreshOnce(ctx)"],
          ["+", 50, "\t})"],
          ["+", 51, "\tif err != nil {"],
          ["+", 52, '\t\treturn Token{}, fmt.Errorf("refresh: %w", err)'],
          ["+", 53, "\t}"],
          ["+", 54, "\treturn v.(Token), nil"],
          [" ", 55, "}"],
        ],
      },
    ],
  },
  {
    path: "internal/auth/middleware.go",
    add: 19,
    del: 3,
    hunks: [
      {
        h: "@@ -88,9 +88,25 @@ func (m *Middleware) Wrap(next http.Handler)",
        lines: [
          [" ", 88, "\tresp, err := m.do(r)"],
          ["-", 89, "\tif resp.StatusCode == http.StatusUnauthorized {"],
          ["-", 90, "\t\tm.client.Refresh(r.Context())"],
          ["+", 89, "\tif resp.StatusCode == http.StatusUnauthorized && !retried(r) {"],
          ["+", 90, "\t\tif _, err := m.client.Refresh(r.Context()); err != nil {"],
          ["+", 91, "\t\t\treturn nil, err"],
          ["+", 92, "\t\t}"],
          ["+", 93, "\t\treturn m.do(markRetried(r))"],
          [" ", 94, "\t}"],
        ],
      },
    ],
  },
  {
    path: "internal/auth/refresh_test.go",
    add: 7,
    del: 0,
    hunks: [
      {
        h: "@@ -0,0 +1,7 @@",
        lines: [
          ["+", 1, "func TestConcurrentRefreshSharesOneCall(t *testing.T) {"],
          ["+", 2, "\tsrv := newFakeIssuer(t)"],
          ["+", 3, "\tc := NewClient(srv.URL)"],
          ["+", 4, "\trunParallel(20, func() { c.Refresh(ctx) })"],
          ["+", 5, "\tif srv.calls != 1 {"],
          ["+", 6, '\t\tt.Fatalf("want 1 refresh, got %d", srv.calls)'],
          ["+", 7, "\t}"],
        ],
      },
    ],
  },
  { path: "go.sum", add: 1240, del: 0, large: true, hunks: [] },
];

/**
 * What "Load diff" shows for the mock's large file: the first lines of a generated go.sum. The
 * large file has no hunks of its own, since a real one would be too long to ship as seed data.
 */
export const LARGE_FILE_SAMPLE: DiffFile["hunks"] = [
  {
    h: "@@ -0,0 +1,1240 @@",
    lines: [
      ["+", 1, "cloud.google.com/go v0.115.0 h1:CnFSK6Xo3lDYRoBKEcAtia6VSC837/ZkJuRduSFnr14="],
      ["+", 2, "google.golang.org/grpc v1.66.0 h1:DibZuoBznOxbDQxRINckZcUvnCEvrW9pcWIE2yF9r1c="],
      ["+", 3, "..."],
    ],
  },
];

const DIFFS: Record<CardKey, DiffFile[]> = { "api#41": DIFF_41 };

/** The card's diff: a hand-written one for #41, a small generated one for the others. */
export function diffFor(c: Card): DiffFile[] {
  const known = DIFFS[c.id];
  if (known) return known;
  if (c.state === "backlog") return [];
  const [first = "", second = ""] = filesFor(c);
  return [
    {
      path: first,
      add: 12 + (c.n % 30),
      del: c.n % 9,
      hunks: [
        {
          h: "@@ -12,6 +12,9 @@",
          lines: [
            [" ", 12, `// ${c.title}`],
            ["-", 13, "const legacy = true"],
            ["+", 13, "const legacy = false"],
            ["+", 14, `// Changed by card ${cardLabel(c)}`],
            [" ", 15, ""],
          ],
        },
      ],
    },
    {
      path: second,
      add: 4,
      del: 1,
      hunks: [
        {
          h: "@@ -3,4 +3,7 @@",
          lines: [
            [" ", 3, ""],
            ["+", 4, `// test for ${cardLabel(c)}`],
            [" ", 5, ""],
          ],
        },
      ],
    },
  ];
}
