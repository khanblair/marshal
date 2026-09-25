/* Marshal prototype store: fake data, in-memory state, actions, simulated daemon. */
(function () {
  if (window.M) return;
  const MIN = 6e4, H = 36e5, D = 864e5;
  const NOW = Date.now();
  const d0 = new Date(); d0.setHours(0, 0, 0, 0);
  const T0 = d0.getTime();

  /* ---------------- icons ---------------- */
  const CUSTOM = {
    'st-backlog': '<circle cx="12" cy="12" r="8" stroke-dasharray="2.6 2.6"/>',
    'st-planning': '<path d="M5 22V3"/><path d="M5 4h13l-2.6 4.5L18 13H5"/>',
    'st-working': '<path d="M5 22V3"/><path d="M5 4h13l-2.6 4.5L18 13H5z" fill="currentColor"/>',
    'st-done': '<path d="M5 22V3"/><rect x="5" y="4" width="14" height="9" rx=".5"/><path d="M5 4h3.5v4.5H5zM12 4h3.5v4.5H12zM8.5 8.5H12V13H8.5zM15.5 8.5H19V13h-3.5z" fill="currentColor" stroke="none"/>'
  };
  const ALIAS = { 'st-needs': 'hand', 'st-review': 'eye', 'st-ready': 'git-merge', 'st-merging': 'git-merge' };
  const pending = new Set();
  function lucideInner(name) {
    const L = window.lucide;
    if (!L || !L.icons) return null;
    const key = name.split('-').map(s => s ? s[0].toUpperCase() + s.slice(1) : '').join('');
    let ic = L.icons[key];
    if (!ic) return '';
    if (typeof ic[0] === 'string') ic = ic[2] || [];
    return ic.map(([t, a]) => '<' + t + ' ' + Object.entries(a || {}).map(([k, v]) => k + '="' + v + '"').join(' ') + '/>').join('');
  }
  class MIcon extends HTMLElement {
    static get observedAttributes() { return ['name', 'size']; }
    connectedCallback() { this.r(); }
    attributeChangedCallback() { if (this.isConnected) this.r(); }
    r() {
      const n = this.getAttribute('name') || '';
      const s = +(this.getAttribute('size') || 16);
      this.setAttribute('aria-hidden', 'true');
      this.style.cssText = 'display:inline-flex;flex:none;align-items:center;justify-content:center;width:' + s + 'px;height:' + s + 'px;line-height:0';
      const root = this.shadowRoot || this.attachShadow({ mode: 'open' });
      const CSS = '<style>@keyframes m-spin{to{transform:rotate(360deg)}}@media (prefers-reduced-motion: reduce){span{animation:none !important}}svg,span{display:block}</style>';
      if (n === 'spinner') {
        const w = Math.round(s * 0.75);
        root.innerHTML = CSS + '<span style="width:' + w + 'px;height:' + w + 'px;box-sizing:border-box;border-radius:9999px;border:1.5px solid currentColor;border-right-color:transparent;animation:m-spin 800ms linear infinite"></span>';
        return;
      }
      let inner = CUSTOM[n];
      if (inner === undefined) inner = lucideInner(ALIAS[n] || n);
      if (inner === null) { pending.add(this); inner = ''; }
      const sw = s <= 14 ? 1.75 : 1.5;
      root.innerHTML = CSS + '<svg xmlns="http://www.w3.org/2000/svg" width="' + s + '" height="' + s + '" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="' + (sw * 24 / s * 0.85).toFixed(2) + '" stroke-linecap="round" stroke-linejoin="round">' + inner + '</svg>';
    }
  }
  class MDot extends HTMLElement {
    static get observedAttributes() { return ['state', 'size']; }
    connectedCallback() { this.r(); }
    attributeChangedCallback() { if (this.isConnected) this.r(); }
    r() {
      const st = this.getAttribute('state') || 'backlog';
      const s = this.getAttribute('size') || 8;
      this.style.cssText = 'display:inline-block;flex:none;width:' + s + 'px;height:' + s + 'px;border-radius:9999px;background:' + solidOf(st) + ';' + (st === 'working' ? 'animation:m-pulse 1600ms ease-in-out infinite;' : '');
    }
  }
  if (!customElements.get('m-icon')) customElements.define('m-icon', MIcon);
  if (!customElements.get('m-dot')) customElements.define('m-dot', MDot);
  let lt = 0;
  const lp = setInterval(() => {
    if (window.lucide && window.lucide.icons) { pending.forEach(e => e.r()); pending.clear(); clearInterval(lp); }
    if (++lt > 150) clearInterval(lp);
  }, 80);

  /* ---------------- constants ---------------- */
  const STATUS = {
    backlog: { label: 'Backlog', icon: 'st-backlog', tone: null },
    planning: { label: 'Planning', icon: 'st-planning', tone: 'planning' },
    working: { label: 'Working', icon: 'st-working', tone: 'working' },
    needs: { label: 'Needs you', icon: 'st-needs', tone: 'needs-you' },
    review: { label: 'In review', icon: 'st-review', tone: 'review' },
    ready: { label: 'Ready to merge', icon: 'st-ready', tone: 'ready' },
    merging: { label: 'Merging', icon: 'st-ready', tone: 'ready' },
    done: { label: 'Done', icon: 'st-done', tone: 'done' }
  };
  const COLUMNS = ['backlog', 'planning', 'working', 'needs', 'review', 'ready', 'done'];
  const colOf = s => (s === 'merging' ? 'ready' : s);
  function tone(t, k) {
    if (!t) return k === 'solid' ? 'var(--color-border-strong)' : k === 'text' ? 'var(--color-text-secondary)' : 'var(--color-surface-sunken)';
    return 'var(--color-status-' + t + '-' + k + ')';
  }
  function solidOf(st) { const s = STATUS[st]; return tone(s ? s.tone : st === 'danger' ? 'danger' : null, 'solid'); }
  const CI = {
    queued: { label: 'Queued', icon: 'spinner', color: 'var(--color-text-secondary)' },
    running: { label: 'Running', icon: 'spinner', color: 'var(--color-text-secondary)' },
    passed: { label: 'Passed', icon: 'check', color: tone('working', 'text') },
    failed: { label: 'Failed', icon: 'x', color: tone('danger', 'text') },
    cancelled: { label: 'Cancelled', icon: 'slash', color: 'var(--color-text-muted)' }
  };
  const AGENTS = {
    'Claude Code': { models: ['claude-sonnet-4-5', 'claude-opus-4-1', 'claude-haiku-4-5'], icon: 'terminal-square', version: '2.0.14' },
    'Codex': { models: ['gpt-5-codex', 'gpt-5', 'gpt-5-mini'], icon: 'terminal-square', version: '0.42.0' },
    'Gemini CLI': { models: ['gemini-2.5-pro', 'gemini-2.5-flash'], icon: 'terminal-square', version: '0.8.1' },
    'Built-in agent': { models: ['claude-sonnet-4-5', 'gpt-5-mini', 'deepseek-chat', 'gemini-2.5-flash', 'qwen2.5-coder:32b'], icon: 'cpu', version: 'Marshal 0.9' }
  };
  const NO_THINK = ['deepseek-chat', 'qwen2.5-coder:32b'];
  const THINK = ['Low', 'Medium', 'High', 'Extra high'];
  const PERMS = ['Ask', 'Auto-accept edits', 'Plan only', 'Full auto', 'Bypass permissions'];
  const ROLE_NAMES = ['Orchestrator', 'Worker', 'Reviewer', 'Integrator', 'Tester', 'Docs writer', 'Security checker', 'UI checker'];
  const VIEWS = [
    { key: 'chat', label: 'Chats', icon: 'messages-square' },
    { key: 'agents', label: 'Agents', icon: 'bot' },
    { key: 'board', label: 'Board', icon: 'square-kanban' },
    { key: 'list', label: 'List', icon: 'list' },
    { key: 'timeline', label: 'Timeline', icon: 'gantt-chart' },
    { key: 'calendar', label: 'Calendar', icon: 'calendar' }
  ];

  /* ---------------- projects ---------------- */
  const projects = [
    { id: 'api', name: 'api-gateway', lang: 'Go', path: '~/code/api-gateway', ci: 'passed', ciAgo: 38, monthBase: 61.2,
      runs: [{ wf: 'test', st: 'passed', ago: 38 }, { wf: 'lint', st: 'passed', ago: 38 }, { wf: 'release', st: 'cancelled', ago: 310 }] },
    { id: 'web', name: 'web-dashboard', lang: 'TypeScript', path: '~/code/web-dashboard', ci: 'running', ciAgo: 2, monthBase: 48.9,
      runs: [{ wf: 'ci', st: 'running', ago: 2 }, { wf: 'e2e', st: 'passed', ago: 95 }, { wf: 'deploy-preview', st: 'passed', ago: 95 }] },
    { id: 'mobile', name: 'mobile-app', lang: 'Monorepo', path: '~/code/mobile-app', ci: 'failed', ciAgo: 22, monthBase: 88.4,
      packages: ['apps/ios', 'apps/android', 'packages/ui', 'packages/auth', 'packages/api-client'],
      runs: [{ wf: 'android', st: 'failed', ago: 22, pkg: 'apps/android' }, { wf: 'ios', st: 'passed', ago: 22, pkg: 'apps/ios' }, { wf: 'packages', st: 'passed', ago: 22, pkg: 'packages/*' }] }
  ];

  /* ---------------- cards ---------------- */
  function C(id, p, title, state, o) {
    return Object.assign({
      id, p, title, state, role: 'Worker', agent: 'Claude Code', model: 'claude-sonnet-4-5', think: 'Medium', perm: 'Auto-accept edits',
      branch: null, ci: null, cost: 0, doing: '', reason: '', asleep: false, pinned: false, bypass: false, paused: false, pkg: null,
      labels: [], deps: [], members: [], checklists: [], comments: [], s: null, e: null, due: null, ctx: 0.22, upd: NOW - 30 * MIN, pr: null, mergePct: 0, waking: false
    }, o, { upd: o && o.upd != null ? NOW - o.upd * MIN : NOW - 30 * MIN });
  }
  const cards = [
    C(41, 'api', 'Fix token refresh on login', 'working', { model: 'claude-sonnet-4-5', think: 'High', branch: 'marshal/41-fix-token-refresh', ci: 'running', cost: 0.84, doing: 'Running auth tests', labels: ['auth', 'bug'], s: -2, e: 1, ctx: 0.46, upd: 1 }),
    C(43, 'api', 'Add rate limiting per API key', 'needs', { reason: 'Plan ready for review', model: 'claude-opus-4-1', think: 'Extra high', perm: 'Plan only', branch: 'marshal/43-rate-limit-per-key', cost: 0.37, labels: ['feature'], s: 0, e: 4, ctx: 0.18, upd: 34 }),
    C(44, 'api', 'Upgrade grpc-go to 1.66', 'needs', { reason: 'Approval needed: run go get google.golang.org/grpc@v1.66.0', agent: 'Codex', model: 'gpt-5-codex', think: 'Medium', perm: 'Ask', branch: 'marshal/44-grpc-1-66', cost: 0.22, labels: ['deps'], s: -1, e: 1, upd: 12 }),
    C(39, 'api', 'Structured logging with slog', 'review', { agent: 'Gemini CLI', model: 'gemini-2.5-pro', branch: 'marshal/39-slog', ci: 'passed', cost: 1.92, labels: ['refactor'], s: -6, e: -1, upd: 16, pr: 281 }),
    C(40, 'api', 'Retry upstream calls with jitter', 'review', { agent: 'Codex', model: 'gpt-5-codex', think: 'High', perm: 'Full auto', branch: 'marshal/40-retry-jitter', ci: 'running', cost: 1.10, labels: ['reliability'], s: -4, e: 0, upd: 8, pr: 284 }),
    C(36, 'api', 'Remove deprecated v1 routes', 'ready', { think: 'Low', branch: 'marshal/36-drop-v1-routes', ci: 'passed', cost: 0.95, s: -8, e: -2, upd: 17, pr: 276 }),
    C(35, 'api', 'Health check returns build info', 'merging', { role: 'Integrator', model: 'claude-opus-4-1', think: 'High', perm: 'Full auto', branch: 'marshal/35-health-build-info', ci: 'passed', cost: 0.58, mergePct: 55, doing: 'Running affected tests after merge', s: -7, e: 0, upd: 2, pr: 274 }),
    C(33, 'api', 'Cache JWKS keys for 10 minutes', 'done', { agent: 'Built-in agent', model: 'gpt-5-mini', think: 'Low', perm: 'Full auto', branch: 'marshal/33-jwks-cache', ci: 'passed', cost: 0.61, s: -12, e: -9, upd: 2 * 24 * 60, pr: 268 }),
    C(45, 'api', 'OpenAPI spec for admin routes', 'backlog', { role: 'Docs writer', agent: 'Built-in agent', model: 'deepseek-chat', think: null, labels: ['docs'], s: 3, e: 6, deps: [36], due: 7, upd: 3 * 60 }),
    C(46, 'api', 'Split config loader into packages', 'planning', { think: 'High', perm: 'Plan only', branch: 'marshal/46-config-packages', cost: 0.09, doing: 'Reading internal/config and its callers', s: 1, e: 5, deps: [39], upd: 1 }),
    C(42, 'api', 'Load test the /v2/proxy path', 'working', { role: 'Tester', agent: 'Built-in agent', model: 'gpt-5-mini', think: 'Low', perm: 'Full auto', branch: 'marshal/42-load-test-proxy', cost: 0.31, doing: 'Running k6 at 500 requests per second', labels: ['perf'], s: -1, e: 2, upd: 1 }),

    C(118, 'web', 'Dark mode for settings page', 'working', { branch: 'marshal/118-settings-dark-mode', ci: 'running', cost: 0.53, doing: 'Updating theme tokens in settings.tsx', labels: ['ui'], s: -1, e: 2, upd: 1, ctx: 0.31 }),
    C(119, 'web', 'Migrate tables to TanStack Table v8', 'needs', { reason: 'Stuck: same type error 3 times in columns.tsx', agent: 'Codex', model: 'gpt-5-codex', think: 'High', branch: 'marshal/119-tanstack-table', ci: 'failed', cost: 2.36, labels: ['refactor'], s: -5, e: 1, upd: 25, ctx: 0.81 }),
    C(115, 'web', 'Chart tooltips cut off on small screens', 'review', { asleep: true, agent: 'Gemini CLI', model: 'gemini-2.5-pro', think: 'Low', branch: 'marshal/115-chart-tooltips', ci: 'passed', cost: 0.77, labels: ['bug', 'ui'], s: -4, e: -1, upd: 3 * 60, pr: 902 }),
    C(116, 'web', 'Add CSV export to reports', 'ready', { branch: 'marshal/116-csv-export', ci: 'passed', cost: 1.44, labels: ['feature'], s: -6, e: -1, upd: 18, pr: 899 }),
    C(110, 'web', 'Fix flaky login e2e test', 'done', { role: 'Tester', agent: 'Codex', model: 'gpt-5-codex', branch: 'marshal/110-flaky-login-e2e', ci: 'passed', cost: 0.48, labels: ['bug'], s: -10, e: -8, upd: 26 * 60, pr: 891 }),
    C(111, 'web', 'Update onboarding copy', 'done', { role: 'Docs writer', agent: 'Built-in agent', model: 'claude-haiku-4-5', think: 'Low', branch: 'marshal/111-onboarding-copy', ci: 'passed', cost: 0.12, labels: ['docs'], s: -9, e: -8, upd: 40 * 60, pr: 893 }),
    C(120, 'web', 'Keyboard shortcuts help dialog', 'backlog', { labels: ['ui'], s: 4, e: 6, due: 0, upd: 5 * 60 }),
    C(121, 'web', 'Virtualize activity table', 'backlog', { labels: ['perf'], s: 2, e: 5, deps: [119], upd: 6 * 60 }),
    C(117, 'web', 'Session timeout warning banner', 'planning', { asleep: true, role: 'UI checker', model: 'claude-haiku-4-5', think: 'Low', perm: 'Plan only', branch: 'marshal/117-timeout-banner', cost: 0.05, labels: ['ui'], s: 1, e: 3, upd: 2 * 60 }),

    C(209, 'mobile', 'Biometric login on Android', 'working', { bypass: true, pkg: 'apps/android', model: 'claude-opus-4-1', think: 'High', perm: 'Bypass permissions', branch: 'marshal/209-biometric-login', ci: 'running', cost: 3.12, doing: 'Running ./gradlew :app:testDebugUnitTest', labels: ['auth'], s: -3, e: 2, upd: 1, ctx: 0.58 }),
    C(207, 'mobile', 'Shared Button component variants', 'review', { pinned: true, pkg: 'packages/ui', agent: 'Gemini CLI', model: 'gemini-2.5-pro', branch: 'marshal/207-button-variants', ci: 'passed', cost: 0.91, labels: ['ui'], s: -5, e: -1, upd: 44, pr: 1432 }),
    C(210, 'mobile', 'Offline queue for API client', 'needs', { reason: 'Merge conflict: queue.ts also changed by #208', pkg: 'packages/api-client', role: 'Integrator', model: 'claude-opus-4-1', think: 'High', perm: 'Full auto', branch: 'marshal/210-offline-queue', ci: 'passed', cost: 1.63, labels: ['feature'], s: -4, e: 1, deps: [208], upd: 20, pr: 1435 }),
    C(208, 'mobile', 'Typed errors in api-client', 'ready', { asleep: true, pkg: 'packages/api-client', agent: 'Codex', model: 'gpt-5-codex', branch: 'marshal/208-typed-errors', ci: 'passed', cost: 1.21, s: -7, e: -2, upd: 4 * 60, pr: 1429 }),
    C(205, 'mobile', 'Refresh tokens in secure storage', 'done', { pkg: 'packages/auth', branch: 'marshal/205-secure-refresh', ci: 'passed', cost: 1.05, labels: ['auth'], s: -11, e: -7, upd: 3 * 24 * 60, pr: 1420 }),
    C(211, 'mobile', 'Push notification deep links', 'backlog', { pkg: 'apps/ios', labels: ['feature'], s: 5, e: 9, due: 9, upd: 8 * 60 }),
    C(212, 'mobile', 'Upgrade React Native to 0.76', 'backlog', { pkg: 'apps/ios', agent: 'Codex', model: 'gpt-5-codex', labels: ['deps'], s: 6, e: 11, deps: [211], upd: 9 * 60 }),
    C(206, 'mobile', 'iOS splash screen flicker', 'review', { asleep: true, pkg: 'apps/ios', agent: 'Codex', model: 'gpt-5-codex', think: 'Low', branch: 'marshal/206-splash-flicker', ci: 'passed', cost: 0.47, labels: ['bug'], s: -3, e: 0, upd: 5 * 60, pr: 1431 }),
    C(213, 'mobile', 'Fix Android e2e failing on main', 'working', { pkg: 'apps/android', role: 'Tester', agent: 'Codex', model: 'gpt-5-codex', think: 'Medium', perm: 'Full auto', branch: 'marshal/213-android-e2e', ci: 'failed', cost: 0.64, doing: 'Reading the failed step log from the android workflow', labels: ['ci'], s: 0, e: 1, upd: 1 })
  ];
  let nextId = 300;
  const people = [{ id: 'ada', name: 'Ada Okafor' }, { id: 'blair', name: 'Blair Akandwanaho' }, { id: 'godana', name: 'Godana Emiru' }, { id: 'angella', name: 'Angella Nsubuga' }];
  let ck = 1;
  const CL = (title, items, doneN, by) => ({ id: 'cl' + (ck++), title, hideDone: false, items: items.map((t, i) => ({ id: 'it' + (ck++), text: t, done: i < (doneN || 0), by: i < (doneN || 0) ? (by || 'agent') : null, doneAt: NOW - (items.length - i) * 17 * MIN })) });
  const CM = (author, text, agoMin, att) => ({ id: 'co' + (ck++), author, text, ts: NOW - agoMin * MIN, att: att || [], read: true });
  const cs = id => cards.find(c => c.id === id);
  cs(41).members = ['ada', 'blair'];
  cs(41).checklists = [CL('Sub-tasks', ['Find why two refreshes run at once', 'Add a single-flight guard to the refresh client', 'Retry the request once after refresh', 'Run the race detector on the auth package'], 3), CL('Definition of done', ['No logouts when a token expires mid-request', 'Tests cover concurrent refresh', 'Reviewer approved the pull request'], 1, 'blair')];
  cs(41).comments = [CM('blair', 'Repro steps from support: log in, leave the tab for 16 minutes, then open two reports at once. Screenshot of the logout attached.', 180, [{ kind: 'image', name: 'logout-after-refresh.png', size: '184 KB' }]), CM('agent', 'Read this before starting. The two parallel report requests explain the double refresh. Covered by the concurrent refresh test.', 150), CM('ada', 'Related Sentry issue: https://sentry.io/issues/48213 and the auth spec in the wiki.', 60, [{ kind: 'link', name: 'sentry.io/issues/48213', url: 'https://sentry.io/issues/48213' }, { kind: 'file', name: 'auth-refresh-spec.pdf', size: '312 KB' }])];
  cs(43).members = ['ada', 'godana'];
  cs(43).checklists = [CL('Acceptance criteria', ['Each API key has its own limit', 'Limits come from the plan tier', 'Over-limit requests get 429 with Retry-After', 'Limiter adds less than 200 ns per call'], 0)];
  cs(43).comments = [CM('godana', 'Enterprise keys should get 1,000 requests per minute, not 100. Pricing sheet attached.', 40, [{ kind: 'file', name: 'plan-tiers-2026.xlsx', size: '48 KB' }])];
  cs(44).members = ['ada'];
  cs(118).members = ['angella', 'ada'];
  cs(118).checklists = [CL('Sub-tasks', ['Replace hard-coded grays with tokens', 'Check contrast in dark mode', 'Capture before and after screenshots'], 1)];
  cs(118).comments = [CM('angella', 'Design reference for the dark settings page is in Figma. Keep the section dividers.', 25, [{ kind: 'link', name: 'figma.com/file/settings-dark', url: 'https://figma.com/file/settings-dark' }, { kind: 'image', name: 'settings-dark-mock.png', size: '402 KB' }])];
  cs(119).members = ['godana']; cs(119).comments = [CM('godana', 'Type the column helper with createColumnHelper<ReportRow>(). The loose typing caused bugs last time.', 8)];
  cs(209).members = ['blair']; cs(207).members = ['angella']; cs(210).members = ['ada', 'godana']; cs(116).members = ['godana']; cs(39).members = ['blair']; cs(36).members = ['ada'];
  cs(207).checklists = [CL('Variants', ['Primary', 'Secondary', 'Ghost', 'Destructive', 'Icon only'], 5)];
  cs(210).checklists = [CL('Definition of done', ['Requests queue while offline', 'Queue replays in order when back online', 'Errors use ApiClientError'], 2)];

  /* ---------------- chats ---------------- */
  let mid = 1;
  const m = o => Object.assign({ id: 'm' + (mid++) }, o);
  const U = t => m({ k: 'user', text: t });
  const A = t => m({ k: 'agent', text: t });
  const T = (icon, action, result, st, detail) => m({ k: 'tool', icon, action, result, st: st || 'ok', detail: detail || '', open: false });
  const SYS = t => m({ k: 'system', text: t });

  const FILES = { api: ['internal/proxy/handler.go', 'internal/auth/middleware.go', 'cmd/gateway/main.go'], web: ['src/pages/Reports.tsx', 'src/components/Table.tsx', 'src/lib/api.ts'], mobile: ['src/index.ts', 'src/client.ts', 'README.md'] };
  const TEST = { api: 'go test ./...', web: 'pnpm test', mobile: 'pnpm --filter {pkg} test' };
  function filesFor(c) { const f = FILES[c.p]; return c.pkg ? f.map(x => c.pkg + '/' + x) : f; }
  function testFor(c) { return TEST[c.p].replace('{pkg}', c.pkg || ''); }

  function genericChat(c) {
    if (c.state === 'backlog') return [];
    const f = filesFor(c);
    const out = [U('Please ' + c.title[0].toLowerCase() + c.title.slice(1) + '. Keep the change small and add tests.'),
      A('I\'ll read the relevant code first, then make the change.'),
      T('file-search', 'Read ' + f[0], (80 + c.id % 120) + ' lines'),
      T('file-pen', 'Edited ' + f[0], '+' + (12 + c.id % 30) + ' −' + (c.id % 9)),
      T('terminal', 'Ran ' + testFor(c), (20 + c.id % 60) + ' passed')];
    if (c.state === 'planning') return out.slice(0, 3).concat([A('I\'m reading the callers before I write a plan.')]);
    if (['review', 'ready', 'merging', 'done'].includes(c.state)) {
      out.push(T('git-pull-request', 'Opened pull request #' + c.pr, 'Checks started'));
      out.push(A('I opened pull request #' + c.pr + '. The Reviewer is reading it now.'));
    }
    if (['ready', 'merging', 'done'].includes(c.state)) out.push(SYS('Reviewer approved. All checks passed.'));
    if (c.state === 'merging') out.push(SYS('The Integrator picked up this card. Dry-run merge found no conflicts.'));
    if (c.state === 'done') out.push(SYS('Merged into main. Worktree removed.'));
    return out;
  }
  const chat = {};
  cards.forEach(c => { chat[c.id] = genericChat(c); });

  chat[41] = [
    U('Users get logged out when the access token expires during a request. Refresh once and retry the request. Keep the change inside internal/auth.'),
    A('I\'ll start with the token middleware and the refresh client.'),
    T('file-search', 'Read internal/auth/middleware.go', '212 lines'),
    T('file-search', 'Read internal/auth/refresh.go', '148 lines'),
    A('The middleware refreshes on every 401. Two parallel requests start two refreshes, and the second one invalidates the first token. I\'ll add a single-flight guard so concurrent requests share one refresh.'),
    T('file-pen', 'Edited internal/auth/refresh.go', '+38 −9', 'ok', 'func (c *Client) Refresh(ctx context.Context) (Token, error) {\n+\tv, err, _ := c.group.Do("refresh", func() (any, error) {\n+\t\treturn c.refreshOnce(ctx)\n+\t})'),
    T('file-pen', 'Edited internal/auth/middleware.go', '+19 −3'),
    T('file-plus', 'Created internal/auth/refresh_test.go', '+7 −0'),
    m({ k: 'diff', files: 3, add: 64, del: 12 }),
    T('terminal', 'Ran go test ./internal/auth/...', 'Running', 'running')
  ];
  chat[43] = [
    U('Add per-key rate limiting to the proxy. Limits come from the plan tier. Plan first, I want to see it before any code.'),
    A('I read the proxy middleware, the config loader, and the tier definitions. Here is my plan.'),
    T('file-search', 'Read internal/proxy/middleware.go', '301 lines'),
    T('file-search', 'Read internal/config/tiers.go', '64 lines'),
    m({ k: 'plan', st: 'waiting', editing: false,
      steps: ['Add a token bucket limiter keyed by API key in internal/ratelimit', 'Load limits per plan tier from config, with a default of 100 requests per minute', 'Run the limiter in the proxy middleware, after key lookup and before the upstream call', 'Return 429 with a Retry-After header when a key is over its limit', 'Add unit tests and a benchmark for the limiter'],
      files: ['internal/ratelimit/bucket.go', 'internal/ratelimit/bucket_test.go', 'internal/proxy/middleware.go', 'internal/config/tiers.go'],
      risks: ['Limits are per instance until shared state is added, so 3 pods allow 3 times the limit', 'A hot key could lock the shared map. I\'ll shard the map by key hash'],
      checks: ['go test ./...', 'golangci-lint run', 'Limiter benchmark under 200 ns per call'] })
  ];
  chat[44] = [
    U('Upgrade grpc-go to 1.66 and fix anything that breaks.'),
    T('file-search', 'Read go.mod', '48 lines'),
    A('grpc-go is on v1.64.1. Upgrading changes go.mod and go.sum, so I need your approval to run the command.'),
    m({ k: 'approval', st: 'waiting', cmd: 'go get google.golang.org/grpc@v1.66.0', why: 'Changes go.mod and go.sum in this worktree. Needs network access to proxy.golang.org.' })
  ];
  chat[119] = [
    U('Move all dashboard tables to TanStack Table v8.'),
    T('file-pen', 'Edited src/components/table/columns.tsx', '+120 −88'),
    T('terminal', 'Ran pnpm tsc --noEmit', 'Failed', 'fail', "src/components/table/columns.tsx:48:7 - error TS2322: Type 'AccessorFn<Row, unknown>' is not assignable to type 'AccessorFn<Row, string>'."),
    T('file-pen', 'Edited src/components/table/columns.tsx', '+4 −4'),
    T('terminal', 'Ran pnpm tsc --noEmit', 'Failed', 'fail', "columns.tsx:48:7 - error TS2322 (same error)"),
    T('file-pen', 'Edited src/components/table/columns.tsx', '+4 −4'),
    T('terminal', 'Ran pnpm tsc --noEmit', 'Failed', 'fail', "columns.tsx:48:7 - error TS2322 (same error)"),
    SYS('The stuck detector paused this card. The same type error appeared 3 times in a row in columns.tsx.'),
    A('I keep hitting the same generic type error on the accessor. Should I type the column helper as createColumnHelper<ReportRow>(), or keep the loose typing and cast at the call site?')
  ];
  chat[210] = genericChat(cards.find(c => c.id === 210)).concat([
    SYS('The Integrator stopped the merge. The dry run found a conflict it can\'t resolve safely.'),
    A('Both #210 and #208 changed packages/api-client/src/queue.ts. #208 renamed QueueError to ApiClientError, and #210 added retries that throw QueueError. Resolving by intent means switching #210 to ApiClientError, but that changes the public error type for apps/ios. How should I proceed?')
  ]);
  chat[209] = [
    U('Add biometric login on Android with a PIN fallback. This is a spike, run without stopping.'),
    SYS('Bypass permissions is on for this card. The agent can run any command in its worktree without asking.'),
    T('terminal', 'Ran ./gradlew :app:dependencies', 'Done'),
    T('file-plus', 'Created apps/android/app/src/main/java/app/auth/BiometricPromptManager.kt', '+142 −0'),
    T('file-pen', 'Edited apps/android/app/build.gradle.kts', '+2 −0'),
    A('The prompt and fallback are wired up. I\'m running the unit tests now.'),
    T('terminal', 'Ran ./gradlew :app:testDebugUnitTest', 'Running', 'running')
  ];
  chat[118] = [
    U('Settings page ignores dark mode. Fix it using the existing theme tokens.'),
    T('file-search', 'Read src/pages/settings.tsx', '288 lines'),
    A('Settings uses hard-coded grays in 14 places. I\'m replacing them with theme tokens.'),
    T('file-pen', 'Edited src/pages/settings.tsx', '+31 −31'),
    T('terminal', 'Ran pnpm test settings', '18 passed')
  ];
  chat[46] = [
    U('internal/config is 1,900 lines. Split it into smaller packages without changing behavior. Plan first.'),
    T('file-search', 'Read internal/config/config.go', '1,904 lines'),
    A('I\'m tracing which packages call config before I write a plan.', true)
  ];

  const pchat = {
    api: [
      U('Plan the rate limiting work. I want per-key limits based on plan tier.'),
      A('I looked at the proxy middleware and the tier config. This fits in one card. I created it in plan first mode, so you\'ll see a plan before any code.'),
      m({ k: 'card', cardId: 43 }),
      U('Also make a card to upgrade grpc-go, we\'re two minor versions behind.'),
      m({ k: 'card', cardId: 44 }),
      A('#44 is waiting on your approval to change go.mod.'),
      m({ k: 'approval', st: 'waiting', cmd: 'go get google.golang.org/grpc@v1.66.0', why: 'Requested by #44 Upgrade grpc-go to 1.66', cardId: 44 })
    ],
    web: [
      U('What is blocked?'),
      m({ k: 'links', text: 'One card is blocked. #119 is stuck on the same type error in columns.tsx and is asking how to type the column helper. #121 waits for #119 to merge.', cards: [119, 121] })
    ],
    mobile: [
      U('Why is main failing?'),
      A('The android workflow failed on main 22 minutes ago in LoginFlowTest. I started #213 with the Tester role to fix it. It is reading the failed step log now.'),
      m({ k: 'card', cardId: 213 })
    ]
  };

  const chats = {};
  const mkChat = (pid, title, target, msgs, agoMin, archived) => { const c = { id: 'ch' + (mid++), pid, title, target, msgs, last: NOW - agoMin * MIN, archived: !!archived }; (chats[pid] = chats[pid] || []).push(c); return c; };
  mkChat('api', 'Rate limiting per key', 'Orchestrator', pchat.api.slice(0, 3), 34);
  mkChat('api', 'Upgrade grpc-go', 'Orchestrator', pchat.api.slice(3), 12);
  mkChat('api', 'Load test results', 'Tester', [U('How did the last load test on /v2/proxy go?'), A('p99 was 182 ms at 500 requests per second. #42 is running the 1,000 requests per second step now.'), m({ k: 'card', cardId: 42 })], 90);
  mkChat('api', 'JWKS caching question', 'Orchestrator', [U('Do we cache JWKS keys?'), A('Yes. #33 added a 10 minute cache and merged 2 days ago.'), m({ k: 'card', cardId: 33 })], 2 * 24 * 60, true);
  mkChat('web', 'What is blocked', 'Orchestrator', pchat.web, 20);
  mkChat('web', 'Settings dark mode', '#118', [U('Use the same tokens as the reports page.'), A('Understood. I am reusing the report tokens for all 14 hard-coded grays.')], 6);
  mkChat('mobile', 'Why main is failing', 'Orchestrator', pchat.mobile, 22);
  mkChat('mobile', 'Button variants review', 'Reviewer', [U('Review #207 when it is ready.'), A('I left 2 comments on pull request #1432. Both are about focus styles on the ghost variant.'), m({ k: 'card', cardId: 207 })], 44);
  const actSeed = {};
  function activityFrom(c) {
    const items = [];
    let t = c.upd - 20 * MIN;
    (chat[c.id] || []).forEach(x => {
      if (x.k === 'tool') items.push({ id: 'a' + (mid++), kind: x.icon === 'terminal' ? (/test|tsc/.test(x.action) ? 'test' : 'command') : (x.icon === 'git-pull-request' ? 'tool' : 'file'), text: x.action, result: x.result, st: x.st, ts: (t += 3 * MIN) });
      if (x.k === 'approval') items.push({ id: 'a' + (mid++), kind: 'approval', text: 'Asked to run ' + x.cmd, result: 'Waiting', st: 'waiting', ts: (t += 2 * MIN) });
      if (x.k === 'system') items.push({ id: 'a' + (mid++), kind: 'tool', text: x.text, result: '', st: 'ok', ts: (t += 1 * MIN) });
    });
    return items.reverse();
  }
  cards.forEach(c => { actSeed[c.id] = activityFrom(c); });

  /* ---------------- diffs & checks ---------------- */
  const DIFFS = {
    41: [
      { path: 'internal/auth/refresh.go', add: 38, del: 9, hunks: [{ h: '@@ -41,18 +41,35 @@ type Client struct', lines: [
        [' ', 41, 'type Client struct {'], [' ', 42, '\thttp   *http.Client'], [' ', 43, '\tissuer string'], ['+', 44, '\tgroup  singleflight.Group'], [' ', 45, '}'], [' ', 46, ''],
        ['-', 47, 'func (c *Client) Refresh(ctx context.Context) (Token, error) {'], ['-', 48, '\treturn c.refreshOnce(ctx)'], ['+', 47, 'func (c *Client) Refresh(ctx context.Context) (Token, error) {'],
        ['+', 48, '\tv, err, _ := c.group.Do("refresh", func() (any, error) {'], ['+', 49, '\t\treturn c.refreshOnce(ctx)'], ['+', 50, '\t})'], ['+', 51, '\tif err != nil {'], ['+', 52, '\t\treturn Token{}, fmt.Errorf("refresh: %w", err)'], ['+', 53, '\t}'], ['+', 54, '\treturn v.(Token), nil'], [' ', 55, '}']] }] },
      { path: 'internal/auth/middleware.go', add: 19, del: 3, hunks: [{ h: '@@ -88,9 +88,25 @@ func (m *Middleware) Wrap(next http.Handler)', lines: [
        [' ', 88, '\tresp, err := m.do(r)'], ['-', 89, '\tif resp.StatusCode == http.StatusUnauthorized {'], ['-', 90, '\t\tm.client.Refresh(r.Context())'], ['+', 89, '\tif resp.StatusCode == http.StatusUnauthorized && !retried(r) {'],
        ['+', 90, '\t\tif _, err := m.client.Refresh(r.Context()); err != nil {'], ['+', 91, '\t\t\treturn nil, err'], ['+', 92, '\t\t}'], ['+', 93, '\t\treturn m.do(markRetried(r))'], [' ', 94, '\t}']] }] },
      { path: 'internal/auth/refresh_test.go', add: 7, del: 0, hunks: [{ h: '@@ -0,0 +1,7 @@', lines: [['+', 1, 'func TestConcurrentRefreshSharesOneCall(t *testing.T) {'], ['+', 2, '\tsrv := newFakeIssuer(t)'], ['+', 3, '\tc := NewClient(srv.URL)'], ['+', 4, '\trunParallel(20, func() { c.Refresh(ctx) })'], ['+', 5, '\tif srv.calls != 1 {'], ['+', 6, '\t\tt.Fatalf("want 1 refresh, got %d", srv.calls)'], ['+', 7, '\t}']] }] },
      { path: 'go.sum', add: 1240, del: 0, large: true, hunks: [] }
    ]
  };
  function diffFor(c) {
    if (DIFFS[c.id]) return DIFFS[c.id];
    if (c.state === 'backlog') return [];
    const f = filesFor(c);
    return [
      { path: f[0], add: 12 + c.id % 30, del: c.id % 9, hunks: [{ h: '@@ -12,6 +12,9 @@', lines: [[' ', 12, '// ' + c.title], ['-', 13, 'const legacy = true'], ['+', 13, 'const legacy = false'], ['+', 14, '// Changed by card #' + c.id], [' ', 15, '']] }] },
      { path: f[1], add: 4, del: 1, hunks: [{ h: '@@ -3,4 +3,7 @@', lines: [[' ', 3, ''], ['+', 4, '// test for #' + c.id], [' ', 5, '']] }] }
    ];
  }
  function checksFor(c) {
    const cmd = testFor(c);
    const lint = { api: 'golangci-lint run', web: 'pnpm lint', mobile: 'pnpm lint' }[c.p];
    const st = c.state === 'backlog' ? 'pending' : ['review', 'ready', 'merging', 'done'].includes(c.state) ? 'passed' : c.ci === 'failed' ? 'failed' : c.state === 'working' ? 'running' : 'pending';
    const list = [
      { id: 'k1', name: 'Tests pass', cmd, st: st === 'running' ? 'running' : st },
      { id: 'k2', name: 'Lint clean', cmd: lint, st: st === 'running' ? 'passed' : st },
      { id: 'k3', name: 'Reviewer approval', cmd: '', st: ['ready', 'merging', 'done'].includes(c.state) ? 'passed' : 'pending' }
    ];
    if (c.p === 'web' && c.labels.includes('ui')) list.push({ id: 'k4', name: 'Screenshot matches', cmd: 'marshal screenshot /settings', st: st === 'passed' ? 'passed' : 'pending' });
    if (c.id === 119) list[0].st = 'failed';
    return list;
  }
  const checks = {};
  cards.forEach(c => { checks[c.id] = checksFor(c); });

  /* ---------------- settings data ---------------- */
  const ROLE_DEFAULTS = {
    Orchestrator: { desc: 'Plans work and splits goals into cards', agent: 'Claude Code', model: 'claude-opus-4-1', think: 'High', perm: 'Plan only', strength: 'Strong or medium', instr: 'You plan work for this project. Explore the codebase and memory, propose a plan, and create cards with the right roles and dependencies after the user approves.' },
    Worker: { desc: 'Does the coding on a card', agent: 'Claude Code', model: 'claude-sonnet-4-5', think: 'Medium', perm: 'Auto-accept edits', strength: 'Your choice', instr: 'You do the coding on one card. Stay inside your worktree, claim the files you change, and keep commits small.' },
    Reviewer: { desc: 'Reviews every pull request before you do', agent: 'Claude Code', model: 'claude-opus-4-1', think: 'High', perm: 'Plan only', strength: 'Strong', instr: 'Review the diff against the card\'s task and acceptance checks. Leave specific comments. Approve only when every check passes.' },
    Integrator: { desc: 'Merges finished work into the target branch', agent: 'Claude Code', model: 'claude-opus-4-1', think: 'High', perm: 'Full auto', strength: 'Strong', instr: 'Merge one card at a time. Dry-run with git merge-tree first. Resolve conflicts by intent. Stop and explain when you are not confident.' },
    Tester: { desc: 'Writes and runs tests', agent: 'Codex', model: 'gpt-5-codex', think: 'Medium', perm: 'Full auto', strength: 'Medium', instr: 'Write focused tests for the change and run them. Report flaky tests separately from real failures.' },
    'Docs writer': { desc: 'Writes and updates documentation', agent: 'Built-in agent', model: 'claude-haiku-4-5', think: 'Low', perm: 'Auto-accept edits', strength: 'Medium', instr: 'Update docs to match the code. Use plain, short sentences.' },
    'Security checker': { desc: 'Looks for security problems in changes', agent: 'Claude Code', model: 'claude-opus-4-1', think: 'Extra high', perm: 'Plan only', strength: 'Strong', instr: 'Look for injection, auth bypass, secrets, and unsafe dependencies in the diff.' },
    'UI checker': { desc: 'Checks UI changes with previews and screenshots', agent: 'Claude Code', model: 'claude-sonnet-4-5', think: 'Medium', perm: 'Plan only', strength: 'Medium', instr: 'Open the live preview, capture before and after screenshots, and compare them against the card.' }
  };
  const roles = ROLE_NAMES.map(n => Object.assign({ name: n, starter: true, overridden: n === 'Worker', skills: n === 'UI checker' ? ['screenshot-compare'] : n === 'Tester' ? ['go-testing', 'playwright'] : ['conventional-commits'], mcp: ['marshal', 'github'], limits: { time: 60, cost: 5, rounds: 12 }, backup: 'gpt-5' }, JSON.parse(JSON.stringify(ROLE_DEFAULTS[n]))));

  const providers = [
    { id: 'anthropic', name: 'Anthropic', st: 'saved', masked: 'sk-ant-…4f2a', models: 'Claude models' },
    { id: 'openai', name: 'OpenAI', st: 'saved', masked: 'sk-proj-…91cd', models: 'GPT models' },
    { id: 'gemini', name: 'Google Gemini', st: 'saved', masked: 'AIza…7Qe0', models: 'Gemini models' },
    { id: 'deepseek', name: 'DeepSeek', st: 'empty', masked: '', models: 'DeepSeek models' },
    { id: 'openrouter', name: 'OpenRouter', st: 'invalid', masked: 'sk-or-…0b33', models: 'Any model on OpenRouter', error: 'OpenRouter rejected this key. Create a new key at openrouter.ai/keys and paste it here.' },
    { id: 'ollama', name: 'Ollama', st: 'saved', masked: 'http://localhost:11434', models: 'Local models', local: true }
  ];
  const integrations = [
    { id: 'github', name: 'GitHub', icon: 'github', st: 'connected', detail: 'GitHub App installed on 3 repositories' },
    { id: 'trello', name: 'Trello', icon: 'trello', st: 'connected', detail: 'web-dashboard syncs with the board Dashboard roadmap' },
    { id: 'gcal', name: 'Google Calendar', icon: 'calendar', st: 'connected', detail: 'Reading 2 calendars for briefs and the calendar view' },
    { id: 'gmail', name: 'Gmail', icon: 'mail', st: 'none', detail: 'Turn labeled emails into cards' },
    { id: 'telegram', name: 'Telegram', icon: 'send', st: 'connected', detail: 'Approvals and briefs go to @marshal_ops_bot' },
    { id: 'discord', name: 'Discord', icon: 'message-circle', st: 'error', detail: 'The bot token expired. Reconnect Discord to keep approvals working there.' },
    { id: 'obsidian', name: 'Obsidian', icon: 'book-open', st: 'connected', detail: 'Vault at ~/Notes/Marshal' }
  ];
  const schedules = [
    { id: 's1', name: 'Morning brief', kind: 'brief', icon: 'sunrise', trigger: 'Cron', when: 'Every weekday at 8:00', time: '08:00', days: [1, 2, 3, 4, 5], action: 'Send the brief to the app, Telegram, and Obsidian', project: 'All projects', enabled: true, missed: 'Run once on wake' },
    { id: 's2', name: 'Evening brief', kind: 'brief', icon: 'sunset', trigger: 'Cron', when: 'Every weekday at 18:00', time: '18:00', days: [1, 2, 3, 4, 5], action: 'Send the brief to the app and Obsidian', project: 'All projects', enabled: true, missed: 'Skip' },
    { id: 's3', name: 'Check open issues', kind: 'job', icon: 'clock', trigger: 'Cron', when: 'Every weekday at 9:00', time: '09:00', days: [1, 2, 3, 4, 5], action: 'Send a message to the Orchestrator', project: 'web-dashboard', enabled: true, missed: 'Run once on wake' },
    { id: 's4', name: 'Dependency updates', kind: 'job', icon: 'clock', trigger: 'Cron', when: 'Every Monday at 2:00', time: '02:00', days: [1], action: 'Create a card from the Dependency update template', project: 'api-gateway', enabled: true, missed: 'Run once on wake' },
    { id: 's5', name: 'Fix until e2e passes', kind: 'job', icon: 'repeat', trigger: 'Interval', when: 'Every 30 minutes', time: '', days: [], action: 'Loop on #213, max 6 rounds, 2 hours, $4.00', project: 'mobile-app', enabled: true, missed: 'Skip' }
  ];
  const calEvents = [
    { id: 'e1', title: 'Standup', time: '09:30', days: [1, 2, 3, 4, 5] },
    { id: 'e2', title: 'Design review', time: '14:00', dayOffset: 1 },
    { id: 'e3', title: 'Release planning', time: '11:00', dayOffset: 6 },
    { id: 'e4', title: 'Dentist', time: '16:30', dayOffset: -3 }
  ];

  /* ---------------- state ---------------- */
  const S = {
    ready: true, vw: window.innerWidth, theme: 'system', reduced: false,
    route: { page: 'home', pid: 'api', view: 'board' },
    lastView: { api: 'board', web: 'board', mobile: 'board' },
    people, projects, cards, chat, act: actSeed, chats, checks, chatOpen: {}, chatQuery: {}, archOpen: {},
    openId: null, focusId: null, tab: 'chat', mode: 'chat', switching: false, detailW: 600, detailExpanded: false,
    filters: { api: [], web: [], mobile: [] }, query: { api: '', web: '', mobile: '' },
    swim: { api: 'none', web: 'none', mobile: 'package' }, laneCollapsed: {}, showAllDone: {},
    savedViews: {
      api: [{ name: 'All cards', f: [], swim: 'none' }, { name: 'Needs me', f: [{ k: 'status', v: 'needs' }], swim: 'none' }, { name: 'Claude Code by role', f: [{ k: 'agent', v: 'Claude Code' }], swim: 'role' }],
      web: [{ name: 'All cards', f: [], swim: 'none' }, { name: 'UI work', f: [{ k: 'label', v: 'ui' }], swim: 'none' }, { name: 'By agent', f: [], swim: 'agent' }],
      mobile: [{ name: 'By package', f: [], swim: 'package' }, { name: 'All cards', f: [], swim: 'none' }, { name: 'api-client only', f: [{ k: 'package', v: 'packages/api-client' }], swim: 'none' }]
    },
    savedView: { api: 'All cards', web: 'All cards', mobile: 'By package' },
    notices: [], noticesOpen: false, noticesSeen: 0,
    toasts: [], dialog: null, palette: false, newCard: null, sidebarCollapsed: false, mobileTab: 'home', menu: null,
    limits: { global: { day: 25, month: 400, awake: 18 }, api: { day: 10, month: 150, awake: 8 }, web: { day: 8, month: 120, awake: 6 }, mobile: { day: 8, month: 150, awake: 8 } },
    sleep: { idle: 15, warn: 2, channel: 'In app only', restore: 'Auto-restore on startup' },
    roles, providers, integrations, schedules, calEvents,
    settingsSection: 'general', roleSel: 'Worker',
    listCols: { id: true, title: true, state: true, role: true, agent: true, model: true, branch: true, ci: true, cost: true, upd: true, pkg: false, think: false },
    sort: { agents: { k: 'state', dir: 1 }, list: { k: 'id', dir: -1 } },
    calMode: 'month', calCursor: T0,
    announce: '',
    profile: { name: 'Ada Okafor', email: '', tz: 'Europe/London', avatar: null, tailnet: 'ada@kolaborate.co', node: 'marshal-laptop.tail3f2a.ts.net', devices: [{ id: 'd1', name: 'Pixel 8', kind: 'smartphone', last: NOW - 40 * MIN }, { id: 'd2', name: 'iPad Air', kind: 'tablet', last: NOW - 2 * D }] },
    onboarding: !localStorage.getItem('marshal-proto-onboarded'), obStep: 0, tour: null,
    feed: [], split: [], dashRange: 7, vh: window.innerHeight
  };

  /* ---------------- helpers ---------------- */
  const subs = new Set();
  let raf = 0;
  function emit() { if (raf) return; raf = requestAnimationFrame(() => { raf = 0; subs.forEach(f => { try { f(); } catch (e) { console.error(e); } }); }); }
  function set(p) { Object.assign(S, typeof p === 'function' ? p(S) : p); emit(); }
  const card = id => S.cards.find(c => c.id === +id);
  const proj = id => S.projects.find(p => p.id === id);
  const money = v => '$' + (Math.round(v * 100) / 100).toFixed(2);
  function rel(ts) {
    const d = Date.now() - ts;
    if (d < 45e3) return 'Just now';
    if (d < H) return Math.round(d / MIN) + ' min ago';
    if (d < D) return Math.round(d / H) + ' h ago';
    const n = Math.round(d / D); return n === 1 ? 'Yesterday' : n + ' days ago';
  }
  const full = ts => new Date(ts).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
  function announce(t) { S.announce = t; emit(); }
  function thinkSupported(model) { return !NO_THINK.includes(model); }
  const isAwake = c => !['backlog', 'done'].includes(c.state) && !c.asleep;

  function deco(c) {
    const st = STATUS[c.state]; const t = st.tone; const ci = c.ci ? CI[c.ci] : null;
    const quiet = c.state === 'done' || c.asleep;
    const think = c.think && thinkSupported(c.model) ? c.think : '';
    return {
      id: c.id, key: 'c' + c.id, num: '#' + c.id, title: c.title, p: c.p, projectName: proj(c.p).name,
      state: c.state, col: colOf(c.state), stateLabel: st.label, icon: st.icon,
      edge: t ? tone(t, 'solid') : 'var(--color-border)', iconColor: t ? tone(t, 'solid') : 'var(--color-text-muted)', stColor: t ? tone(t, 'text') : 'var(--color-text-secondary)', subtle: tone(t, 'subtle'),
      titleColor: quiet ? 'var(--color-text-secondary)' : 'var(--color-text-primary)',
      role: c.role, agent: c.agent, model: c.model, think, hasThink: !!think, perm: c.perm, hasModel: c.state !== 'backlog',
      showDoing: (c.state === 'working' || c.state === 'merging' || c.state === 'planning') && !!c.doing && !c.asleep && !c.paused,
      doing: c.doing, paused: c.paused && c.state === 'working',
      showReason: c.state === 'needs', reason: c.reason,
      branch: c.branch, hasBranch: !!c.branch, pkg: c.pkg, hasPkg: !!c.pkg,
      hasCi: !!ci, ciLabel: ci ? ci.label : '', ciIcon: ci ? ci.icon : '', ciColor: ci ? ci.color : '',
      cost: money(c.cost), hasCost: c.cost > 0, costNum: c.cost,
      asleep: c.asleep, waking: c.waking, pinned: c.pinned, bypass: c.bypass,
      merging: c.state === 'merging', mergePct: c.mergePct + '%', mergeNum: c.mergePct,
      sleepLabel: c.waking ? 'Waking' : 'Asleep', hasFooter: !!c.branch || !!ci || c.cost > 0 || c.pinned || c.checklists.length > 0 || c.comments.length > 0 || c.members.length > 0,
      ciTip: ci ? 'CI ' + ci.label.toLowerCase() : '',
      avatars: c.members.map(pid => { const p = people.find(x => x.id === pid); return p ? { key: pid, isAgent: false, isPerson: true, initials: p.name.split(' ').map(x => x[0]).join('').slice(0, 2), title: p.name } : null; }).filter(Boolean).concat(c.state === 'backlog' ? [] : [{ key: 'agent', isAgent: true, isPerson: false, initials: '', title: c.agent + ' agent' }]),
      clDone: c.checklists.reduce((a, l) => a + l.items.filter(i => i.done).length, 0), clTotal: c.checklists.reduce((a, l) => a + l.items.length, 0),
      commentsN: c.comments.length, attachN: c.comments.reduce((a, x) => a + x.att.filter(t => t.kind !== 'link').length, 0),
      aria: '#' + c.id + ' ' + c.title + '. ' + st.label + (c.state === 'needs' ? ': ' + c.reason : '') + (c.asleep ? '. Asleep' : '') + (c.pinned ? '. Pinned' : '') + (c.bypass ? '. Bypass permissions on' : ''),
      awakeLabel: c.waking ? 'Waking' : c.asleep ? 'Asleep' : isAwake(c) ? 'Awake' : 'No session',
      upd: rel(c.upd), updFull: full(c.upd), updTs: c.upd, labels: c.labels,
      selected: S.openId === c.id, focused: S.focusId === c.id, dragging: S.dragId === c.id,
      ring: S.openId === c.id ? '0 0 0 2px var(--color-ink)' : S.focusId === c.id ? '0 0 0 2px var(--color-border-strong)' : 'none',
      opacity: S.dragId === c.id ? '0.4' : '1',
      get hasCl() { return this.clTotal > 0; }, get clLabel() { return this.clDone + '/' + this.clTotal; }, get hasComments() { return this.commentsN > 0; }, get hasAttach() { return this.attachN > 0; }, get hasAvatars() { return this.avatars.length > 0; },
      get clTip() { return this.clDone + ' of ' + this.clTotal + ' checklist items done'; }, get commentsTip() { return this.commentsN + (this.commentsN === 1 ? ' comment' : ' comments'); }, get attachTip() { return this.attachN + (this.attachN === 1 ? ' attachment' : ' attachments'); },
      open: () => { if (M._suppressClick) return; M.openCard(c.id); },
      down: e => M.dragStart(e, c.id),
      key2: e => { if (e.key === 'Enter') M.openCard(c.id); }
    };
  }

  function decoMsgs(list, cardId) {
    return list.map(x => {
      const o = { id: x.id, key: x.id, isUser: x.k === 'user', isAgent: x.k === 'agent', isTool: x.k === 'tool', isPlan: x.k === 'plan', isApproval: x.k === 'approval', isDiff: x.k === 'diff', isSystem: x.k === 'system', isCard: x.k === 'card', isLinks: x.k === 'links', text: x.text || '', streaming: !!x.streaming };
      if (o.isTool) {
        const sp = x.action.indexOf(' ');
        const verb = sp > 0 ? x.action.slice(0, sp) : x.action; const rest = sp > 0 ? x.action.slice(sp + 1) : '';
        Object.assign(o, { verb, target: rest, mono: /[\/@]|\.[a-z]{1,4}\b|^(go|pnpm|git|npm|k6|\.\/gradlew)\b/.test(rest) && !/^"/.test(rest), icon: x.icon, result: x.result,
          stIcon: x.st === 'running' ? 'spinner' : x.st === 'fail' ? 'x' : 'check', resColor: x.st === 'fail' ? tone('danger', 'text') : x.st === 'running' ? 'var(--color-text-secondary)' : tone('working', 'text'),
          open: x.open, hasDetail: !!x.detail, noDetail: !x.detail, detail: x.detail, chev: x.open ? 'chevron-down' : 'chevron-right', toggle: () => { x.open = !x.open; emit(); } });
        o.targetFont = o.mono ? 'var(--font-mono)' : 'var(--font-sans)'; o.targetSize = o.mono ? '12px' : '14px';
      }
      if (o.isDiff) Object.assign(o, { summary: 'Changed ' + x.files + ' files', add: '+' + x.add, del: '−' + x.del, openDiff: () => { M.openCard(cardId, 'diff'); } });
      if (o.isPlan) {
        const W = x.st === 'waiting';
        Object.assign(o, { steps: x.steps.map((t, i) => ({ n: i + 1 + '.', t })), files: x.files, risks: x.risks, checks: x.checks, waiting: W && !x.editing, editing: W && x.editing, notEditing: !(W && x.editing), done: !W,
          statusLabel: W ? (x.edited ? 'Edited, waiting for you' : 'Waiting for you') : x.st === 'approved' ? 'Approved' : 'Rejected',
          statusIcon: W ? 'st-needs' : x.st === 'approved' ? 'check' : 'x', statusColor: W ? tone('needs-you', 'text') : x.st === 'approved' ? tone('working', 'text') : tone('danger', 'text'), statusBg: W ? tone('needs-you', 'subtle') : x.st === 'approved' ? tone('working', 'subtle') : tone('danger', 'subtle'),
          editText: x.steps.join('\n'), approve: () => M.approvePlan(cardId), reject: () => M.rejectPlan(cardId), edit: () => M.editPlan(cardId, true), cancel: () => M.editPlan(cardId, false),
          save: e => { e.preventDefault(); M.savePlan(cardId, e.target.steps.value); } });
      }
      if (o.isApproval) {
        const cid = x.cardId || cardId; const W = x.st === 'waiting';
        Object.assign(o, { cmd: x.cmd, why: x.why, waiting: W, done: !W, resultLabel: x.st === 'approved' ? 'Approved' : 'Denied', resultIcon: x.st === 'approved' ? 'check' : 'x', resultColor: x.st === 'approved' ? tone('working', 'text') : tone('danger', 'text'),
          hasCard: !!x.cardId, cardLabel: x.cardId ? '#' + x.cardId + ' ' + card(x.cardId).title : '', openCard: () => M.openCard(cid),
          approve: () => M.approve(cid), deny: () => M.deny(cid), keys: e => { if (!W) return; if (e.key === 'Enter' && e.target === e.currentTarget) { e.preventDefault(); M.approve(cid); } if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); M.deny(cid); } } });
      }
      if (o.isCard) { const c = card(x.cardId); if (!c) { o.isCard = false; return o; } o.c = deco(c); }
      if (o.isLinks) o.cards = x.cards.map(id => card(id)).filter(Boolean).map(deco);
      return o;
    });
  }

  /* ---------------- core actions ---------------- */
  function toast(msg, action) {
    const id = 't' + (mid++);
    S.toasts = S.toasts.concat([{ id, msg, action, dismiss: () => set({ toasts: S.toasts.filter(t => t.id !== id) }) }]).slice(-3);
    emit();
    if (!action) setTimeout(() => set({ toasts: S.toasts.filter(t => t.id !== id) }), 4000);
    else setTimeout(() => set({ toasts: S.toasts.filter(t => t.id !== id) }), 8000);
  }
  function addAct(id, kind, text, result, st) {
    const list = S.act[id] || (S.act[id] = []);
    list.unshift({ id: 'a' + (mid++), kind, text, result: result || '', st: st || 'ok', ts: Date.now(), fresh: true });
    S.act[id] = list.slice(0, 200);
  }
  function push(id, msg) { (S.chat[id] || (S.chat[id] = [])).push(msg); S.chat[id] = S.chat[id].slice(); emit(); return msg; }
  function setState(c, st, extra) {
    const from = c.state;
    Object.assign(c, { state: st, upd: Date.now() }, extra || {});
    if (st !== 'needs') c.reason = extra && extra.reason || (st === 'needs' ? c.reason : '');
    if (from !== st) announce('#' + c.id + ' moved to ' + STATUS[st].label);
    emit();
  }
  function stream(list, text, done, speed) {
    const msg = m({ k: 'agent', text: '', streaming: true });
    list.push(msg);
    const words = text.split(/(\s+)/);
    let i = 0;
    const iv = setInterval(() => {
      msg.text += words.slice(i, i + 3).join(''); i += 3;
      if (i >= words.length) { msg.streaming = false; clearInterval(iv); done && done(); }
      emit();
    }, speed || 45);
    return msg;
  }
  const streamCard = (id, text, done) => stream(S.chat[id] || (S.chat[id] = []), text, done);
  function runTool(id, icon, action, result, ms, done, kind) {
    const t = push(id, T(icon, action, 'Running', 'running'));
    addAct(id, kind || (icon === 'terminal' ? 'command' : 'file'), action, 'Running', 'running');
    const actItem = S.act[id][0];
    setTimeout(() => { t.st = 'ok'; t.result = result; actItem.st = 'ok'; actItem.result = result; emit(); done && done(); }, ms);
  }
  function seq(steps) { let t = 0; steps.forEach(([d, f]) => { t += d; setTimeout(f, t); }); }

  /* ---------------- scripted daemon ---------------- */
  function script41() {
    const id = 41; const c = card(id);
    const last = S.chat[id][S.chat[id].length - 1];
    const act0 = { id: 'a' + (mid++), kind: 'test', text: 'Ran go test ./internal/auth/...', result: 'Running', st: 'running', ts: Date.now() };
    S.act[id].unshift(act0);
    seq([
      [3500, () => { last.st = 'ok'; last.result = '14 passed'; act0.st = 'ok'; act0.result = '14 passed'; c.doing = 'Writing a summary of the test run'; emit(); }],
      [600, () => streamCard(id, 'All 14 auth tests pass, including the new test for concurrent refresh. This change touches shared state, so I\'ll run the full suite with the race detector next.')],
      [4200, () => { const it = c.checklists[0] && c.checklists[0].items[3]; c.doing = 'Running go test -race ./...'; setTimeout(() => { if (it && !it.done) { it.done = true; it.by = 'agent'; it.doneAt = Date.now(); addAct(id, 'tool', c.agent + ' completed ' + it.text, '', 'ok'); emit(); } }, 6200); runTool(id, 'terminal', 'Ran go test -race ./...', '231 passed, no races', 6000, null, 'test'); }],
      [6800, () => streamCard(id, 'No races found. I\'ll commit, push the branch, and open a pull request.')],
      [2600, () => { c.doing = 'Pushing marshal/41-fix-token-refresh'; runTool(id, 'git-commit-horizontal', 'Committed "Share one token refresh between concurrent requests"', '1 commit', 900); }],
      [1200, () => runTool(id, 'upload', 'Ran git push origin marshal/41-fix-token-refresh', 'Pushed', 1400, null, 'command')],
      [1800, () => runTool(id, 'git-pull-request', 'Opened pull request #287', 'Checks started', 800, () => {
        c.pr = 287; c.ci = 'running'; c.doing = ''; setState(c, 'review');
        streamCard(id, 'Pull request #287 is open. The Reviewer is reading it now, and CI is running.');
      }, 'tool')],
      [9000, () => { c.ci = 'passed'; addAct(id, 'test', 'CI passed on marshal/41-fix-token-refresh', 'test, lint', 'ok'); feed('ci', 'CI passed on marshal/41-fix-token-refresh', 'api', { cardId: 41 }); S.checks[id].forEach(k => { if (k.id !== 'k3') k.st = 'passed'; }); emit(); }]
    ]);
  }
  function script46() {
    const id = 46; const c = card(id);
    seq([
      [5000, () => { c.doing = 'Mapping 38 callers of config.Load'; runTool(id, 'search', 'Searched codebase map for config.Load', '38 callers in 11 packages', 1500, null, 'tool'); }],
      [4500, () => streamCard(id, 'Here is my plan. Nothing changes for callers until the last step.', () => {
        push(id, m({ k: 'plan', st: 'waiting', editing: false,
          steps: ['Move env parsing into internal/config/env with no API change', 'Move file loading into internal/config/file', 'Keep config.Load as a thin wrapper over both', 'Update the 38 callers only where they reach into private fields', 'Delete the old helpers once tests pass'],
          files: ['internal/config/config.go', 'internal/config/env/env.go', 'internal/config/file/file.go', '11 caller packages'],
          risks: ['#39 also edits internal/config/logging.go. I claimed that file and will rebase after #39 merges'],
          checks: ['go test ./...', 'go vet ./...', 'No change to config.Load signature'] }));
        c.doing = ''; setState(c, 'needs', { reason: 'Plan ready for review' });
        addAct(id, 'tool', 'Posted a plan for review', 'Waiting', 'waiting'); feed('plan', 'Plan ready for review on #46', 'api', { cardId: 46 });
        notice({ kind: 'plan', cardId: id, text: 'Plan ready for review on #46', sub: 'Split config loader into packages' });
      })]
    ]);
  }
  function scriptMerge35() {
    const c = card(35);
    const iv = setInterval(() => {
      c.mergePct = Math.min(100, c.mergePct + 9); emit();
      if (c.mergePct >= 100) {
        clearInterval(iv);
        c.doing = ''; setState(c, 'done');
        addAct(35, 'tool', 'Merged into main', 'Affected tests passed', 'ok');
        push(35, SYS('Merged into main. Affected tests passed. Worktree removed.'));
        feed('merge', '#35 Health check returns build info merged into main', 'api', { cardId: 35 });
      }
    }, 2200);
  }
  let ciFailRunning = false;
  function ciFailure(id, fast) {
    const c = card(id || 40); if (!c || ciFailRunning) return;
    ciFailRunning = true;
    const k = fast ? 0.35 : 1;
    if (c.state !== 'review') { c.pr = c.pr || 290; setState(c, 'review'); }
    c.ci = 'running'; emit();
    const test = c.p === 'api' ? 'TestRetryBackoff' : c.p === 'web' ? 'Reports export test' : 'LoginFlowTest';
    seq([
      [(fast ? 3000 : 20000) * 1, () => { c.ci = 'failed'; addAct(c.id, 'test', 'CI failed: test job on ' + c.branch, test + ' failed', 'fail'); push(c.id, SYS('CI failed on the test job. Marshal is rerunning it once in case the test is flaky.')); }],
      [3000 * k, () => { c.ci = 'running'; addAct(c.id, 'test', 'Reran the test job', 'Running', 'running'); emit(); }],
      [5000 * k, () => { c.ci = 'failed'; S.act[c.id][0].st = 'fail'; S.act[c.id][0].result = 'Failed again'; push(c.id, SYS('The rerun failed too. Sent the trimmed log of the failed step to the agent.')); c.doing = 'Fixing ' + test + ' after CI failure'; setState(c, 'working'); }],
      [1200 * k, () => streamCard(c.id, 'The test expects the third retry within 400 ms, but jitter can push it to 450 ms. I\'ll cap the jitter at 25 percent of the base delay.')],
      [4000 * k, () => runTool(c.id, 'file-pen', 'Edited internal/proxy/retry.go', '+3 −1', 900)],
      [1500 * k, () => runTool(c.id, 'upload', 'Ran git push origin ' + c.branch, 'Pushed', 900, () => { c.ci = 'running'; c.doing = ''; setState(c, 'review'); }, 'command')],
      [6000 * k, () => {
        c.ci = 'failed';
        addAct(c.id, 'test', 'CI failed: test job on ' + c.branch, test + ' failed, attempt 3', 'fail');
        push(c.id, SYS('CI failed 3 times. The fix loop reached its round limit, so the card needs you.'));
        setState(c, 'needs', { reason: 'CI failed 3 times: ' + test });
        S.checks[c.id][0].st = 'failed'; feed('ci', 'CI failed 3 times on #' + c.id + ' ' + c.title, c.p, { cardId: c.id });
        notice({ kind: 'ci', cardId: c.id, text: 'CI failed on #' + c.id, sub: c.title + '. ' + test + ' failed 3 times.' });
        ciFailRunning = false;
      }]
    ]);
  }

  const DOING = {
    42: ['Running k6 at 500 requests per second', 'Reading p99 latency from the k6 summary', 'Raising load to 1,000 requests per second'],
    118: ['Updating theme tokens in settings.tsx', 'Running pnpm test settings', 'Capturing screenshots in light and dark'],
    209: ['Running ./gradlew :app:testDebugUnitTest', 'Editing BiometricPromptManager.kt', 'Running ./gradlew lint'],
    213: ['Reading the failed step log from the android workflow', 'Editing LoginFlowTest.kt', 'Running ./gradlew connectedCheck']
  };
  let tickN = 0;
  function tick() {
    tickN++;
    if (tickN % 40 === 0) feed('schedule', 'Fix until e2e passes ran round ' + (tickN / 40 + 2) + ' on #213', 'mobile', { cardId: 213 });
    S.cards.forEach(c => {
      if (c.state === 'working' && !c.paused && !c.asleep) {
        c.cost += 0.004 + (c.id % 5) * 0.002;
        const d = DOING[c.id];
        if (d && tickN % 4 === c.id % 4) { c.doing = d[(d.indexOf(c.doing) + 1) % d.length]; c.upd = Date.now(); addAct(c.id, /Running/.test(c.doing) ? 'command' : 'file', c.doing, '', 'ok'); }
      }
    });
    const sl = S.notices.find(n => n.kind === 'sleep');
    if (sl && Date.now() >= sl.deadline) {
      sl.cards.forEach(id => { const c = card(id); if (c && !c.pinned && !['working', 'needs'].includes(c.state)) c.asleep = true; });
      S.notices = S.notices.filter(n => n !== sl);
      announce(sl.cards.length + ' idle cards went to sleep');
    }
    emit();
  }

  function notice(n) { S.notices = [Object.assign({ id: 'n' + (mid++), ts: Date.now() }, n)].concat(S.notices); emit(); }
  S.notices = [
    { id: 'n1', kind: 'sleep', cards: [39, 36, 116], deadline: NOW + 125e3, ts: NOW },
    { id: 'n2', kind: 'ci-main', pid: 'mobile', cardId: 213, text: 'Main is failing in mobile-app', sub: 'The android workflow failed in LoginFlowTest', ts: NOW - 22 * MIN },
    { id: 'n3', kind: 'cost', pid: 'mobile', text: 'mobile-app is near today\'s cost limit', sub: 'Running cards pause when it reaches $8.00', ts: NOW - 9 * MIN }
  ];

  function feed(kind, text, pid, link) { S.feed.unshift(Object.assign({ id: 'f' + (mid++), kind, text, pid, ts: Date.now() }, link || {})); S.feed = S.feed.slice(0, 80); }
  [['brief', 'Morning brief is ready', null, { job: 's1' }, 6 * 60], ['merge', '#205 Refresh tokens in secure storage merged into main', 'mobile', { cardId: 205 }, 3 * 24 * 60], ['merge', '#110 Fix flaky login e2e test merged into main', 'web', { cardId: 110 }, 26 * 60], ['schedule', 'Check open issues ran and sent 2 issues to the Orchestrator', 'web', { job: 's3' }, 5 * 60], ['approval', 'You approved pnpm add @tanstack/react-table on #119', 'web', { cardId: 119 }, 70], ['plan', 'Plan ready for review on #43', 'api', { cardId: 43 }, 34], ['ci', 'CI failed on main in mobile-app: android workflow', 'mobile', { cardId: 213 }, 22], ['ci', 'CI passed on marshal/39-slog', 'api', { cardId: 39 }, 16]].forEach(([k, t, p, l, ago]) => { feed(k, t, p, l); S.feed[0].ts = NOW - ago * MIN; });

  /* ---------------- public API ---------------- */
  const M = window.M = {
    S, STATUS, COLUMNS, CI, AGENTS, THINK, PERMS, ROLE_NAMES, VIEWS, NO_THINK, colOf, tone, money, rel, full, card, proj, deco, set, emit, toast, T0, D, H, MIN,
    thinkSupported, isAwake, diffFor, filesFor, decoMsgs,
    selRef: v => el => { if (el) requestAnimationFrame(() => { if (el.value !== String(v)) el.value = String(v); }); },
    on(f) { subs.add(f); return () => subs.delete(f); },
    get mobile() { return S.vw < 640; },
    cardsOf: pid => S.cards.filter(c => c.p === pid),
    colCards: (list, col) => list.filter(c => colOf(c.state) === col).sort((a, b) => col === 'needs' ? a.upd - b.upd : col === 'done' ? b.upd - a.upd : (b.pinned - a.pinned) || (b.id - a.id)),
    nav: null,
    needs: pid => S.cards.filter(c => c.state === 'needs' && (!pid || c.p === pid)).sort((a, b) => a.upd - b.upd),
    awake: pid => S.cards.filter(c => isAwake(c) && (!pid || c.p === pid)),
    working: pid => S.cards.filter(c => c.state === 'working' && (!pid || c.p === pid)),
    costs(pid) {
      const cs = S.cards.filter(c => !pid || c.p === pid);
      const today = cs.filter(c => c.state !== 'done').reduce((a, c) => a + c.cost, 0) * 0.92 + (pid ? 0.42 : 1.26);
      const month = (pid ? proj(pid).monthBase : S.projects.reduce((a, p) => a + p.monthBase, 0)) + today;
      const L = S.limits[pid || 'global'];
      return { today, month, day: L.day, monthL: L.month, awakeL: L.awake };
    },
    costTone(v, l) { const r = v / l; return r >= 1 ? 'over' : r >= 0.8 ? 'near' : 'normal'; },

    go(page, pid, view) {
      const r = Object.assign({}, S.route, { page });
      if (pid) r.pid = pid;
      if (page === 'project') { r.view = view || S.lastView[r.pid] || 'board'; S.lastView[r.pid] = r.view; }
      S.route = r; S.menu = null; S.noticesOpen = false;
      if (page === 'settings') { S.openId = null; S.detailExpanded = false; }
      if (M.mobile) { S.openId = null; S.mobileTab = page === 'home' ? 'home' : r.view === 'chat' ? 'chat' : 'board'; }
      emit();
    },
    setView(view) { if (S.route.page !== 'project') S.route = Object.assign({}, S.route, { page: 'project' }); S.route = Object.assign({}, S.route, { view }); S.lastView[S.route.pid] = view; S.menu = null; emit(); },
    openCard(id, tab) {
      const c = card(id); if (!c) return;
      S.openId = c.id; S.focusId = c.id; S.tab = tab || 'chat'; S.mode = 'chat'; S.switching = false; S.menu = null; S.noticesOpen = false; S.palette = false;
      if (S.route.page === 'project' && S.route.pid !== c.p) { S.route = Object.assign({}, S.route, { pid: c.p, view: S.lastView[c.p] }); }
      emit();
    },
    closeCard() { set({ openId: null, detailExpanded: false }); },
    setTab: t => set({ tab: t }),
    setMode(mode) {
      if (mode === S.mode || S.switching) return;
      set({ switching: mode });
      setTimeout(() => set({ mode, switching: false }), 1400);
    },

    refuse(c, from, to) {
      if (from === 'done') return 'Done cards are merged. Fork the card to keep working on it.';
      if (to === 'done') return 'Cards move to Done by themselves after they merge.';
      if (to === 'needs') return 'Cards move to Needs you by themselves when an agent is waiting on you.';
      if (to === 'review' && (!c.branch || ['backlog', 'planning'].includes(from))) return 'In review needs an open pull request. The agent opens one when the work is ready.';
      if (to === 'ready' && from !== 'review') return 'Ready to merge needs an approved review and passing checks.';
      if (to === 'ready' && c.ci !== 'passed') return 'Checks haven\'t passed on this card yet, so it can\'t be ready to merge.';
      if (c.state === 'merging') return 'The Integrator is merging this card. Wait for the merge to finish.';
      return null;
    },
    moveCard(id, to, manual) {
      const c = card(id); if (!c) return;
      const from = colOf(c.state); if (from === to) return;
      const prev = { state: c.state, reason: c.reason };
      const why = M.refuse(c, from, to);
      c.state = to; emit();
      if (why) { setTimeout(() => { Object.assign(c, prev); emit(); toast(why); }, 420); return; }
      c.upd = Date.now(); c.asleep = false;
      if (from === 'backlog' && (to === 'working' || to === 'planning')) { M._startSession(c, to); toast('Card started'); }
      else if (to === 'backlog') { c.paused = false; c.doing = ''; c.asleep = true; toast('Card moved to backlog. Session is asleep.'); }
      else if (to === 'working' && from === 'review') { c.doing = 'Addressing review comments'; toast('Sent back to working'); }
      else if (to === 'working' && from === 'planning') { c.doing = 'Starting without a plan'; toast('Plan skipped'); }
      else if (to === 'review') { c.pr = c.pr || 300 + c.id; c.ci = 'running'; c.doing = ''; toast('Pull request opened'); }
      else if (to === 'ready') toast('Ready to merge');
      announce('#' + c.id + ' moved to ' + STATUS[to].label);
      emit();
    },
    _startSession(c, to) {
      c.state = to || (c.perm === 'Plan only' ? 'planning' : 'working');
      c.branch = c.branch || 'marshal/' + c.id + '-' + c.title.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 28);
      c.asleep = false; c.paused = false; c.doing = c.state === 'planning' ? 'Reading the code before writing a plan' : 'Setting up the worktree';
      push(c.id, SYS('Session started with ' + c.agent + ' in a new worktree on ' + c.branch + '.'));
      addAct(c.id, 'tool', 'Created worktree on ' + c.branch, '', 'ok');
      setTimeout(() => runTool(c.id, 'file-search', 'Read ' + filesFor(c)[0], '120 lines', 1500, () => { c.doing = c.state === 'working' ? 'Editing ' + filesFor(c)[0] : c.doing; emit(); }), 1200);
    },
    start(id) {
      const c = card(id);
      if (c.state === 'backlog') { M._startSession(c); emit(); toast('Card started'); return; }
      if (c.paused) { c.paused = false; emit(); toast('Card resumed'); return; }
      if (c.asleep) M.wake(id);
    },
    pause(id) { const c = card(id); if (c.state !== 'working') { toast('Only working cards can be paused.'); return; } c.paused = true; addAct(id, 'tool', 'Paused by you', '', 'ok'); emit(); toast('Card paused'); },
    sleep(id) {
      const c = card(id); if (!c) return;
      if (c.state === 'working' && !c.paused) { toast('Working cards don\'t sleep. Pause the card first.'); return; }
      if (c.state === 'needs') { toast('This card is waiting on you, so it stays awake.'); return; }
      if (!isAwake(c)) { toast('This card has no awake session.'); return; }
      c.asleep = true; M._dropFromSleep(id); emit(); toast('Card asleep'); announce('#' + id + ' is asleep');
    },
    wake(id) {
      const c = card(id); if (!c || !c.asleep) return;
      c.waking = true; emit();
      setTimeout(() => { c.waking = false; c.asleep = false; addAct(id, 'tool', 'Session resumed', '', 'ok'); emit(); toast('Session resumed'); }, 1600);
    },
    pin(id) { const c = card(id); c.pinned = !c.pinned; if (c.pinned) M._dropFromSleep(id); emit(); toast(c.pinned ? 'Card pinned. It won\'t sleep.' : 'Card unpinned'); },
    keepAwake(id) { M._dropFromSleep(id); emit(); toast('Kept awake for 15 more minutes'); },
    _dropFromSleep(id) {
      const n = S.notices.find(x => x.kind === 'sleep'); if (!n) return;
      n.cards = n.cards.filter(x => x !== id);
      if (!n.cards.length) S.notices = S.notices.filter(x => x !== n);
    },
    sleepAll() { const n = S.notices.find(x => x.kind === 'sleep'); if (!n) return; n.cards.forEach(id => { card(id).asleep = true; }); S.notices = S.notices.filter(x => x !== n); emit(); toast(n.cards.length + ' cards asleep'); },
    keepAllAwake() { const n = S.notices.find(x => x.kind === 'sleep'); if (!n) return; S.notices = S.notices.filter(x => x !== n); emit(); toast('Kept ' + n.cards.length + ' cards awake'); },
    dismissNotice(id) { S.notices = S.notices.filter(n => n.id !== id); emit(); },
    fork(id) {
      const c = card(id);
      const f = Object.assign({}, c, { id: nextId++, title: c.title + ' (fork)', state: c.state === 'done' ? 'working' : c.state === 'backlog' ? 'backlog' : 'working', branch: c.branch ? c.branch + '-fork' : null, pr: null, ci: null, cost: 0, pinned: false, asleep: false, upd: Date.now(), doing: 'Starting from the latest checkpoint', reason: '', mergePct: 0 });
      S.cards.push(f); S.chat[f.id] = [SYS('Forked from #' + c.id + ' at its latest checkpoint. This card has its own worktree and a copy of the session context.')]; S.act[f.id] = []; S.checks[f.id] = checksFor(f);
      emit(); toast('Card forked', { label: 'Open', run: () => M.openCard(f.id) });
    },
    deleteCard(id) {
      const c = card(id);
      M.confirm({ title: 'Delete card', message: c.branch ? 'This deletes #' + id + ', its session, and its worktree with unmerged work on ' + c.branch + '.' : 'This deletes #' + id + ' and its notes.', action: 'Delete card', destructive: true,
        run: () => { S.cards = S.cards.filter(x => x.id !== id); if (S.openId === id) S.openId = null; emit(); toast('Card deleted'); } });
    },
    rename(id, title) { const c = card(id); if (title && title.trim()) { c.title = title.trim(); emit(); } },
    setSetting(id, key, val) {
      const c = card(id);
      if (key === 'perm' && val === 'Bypass permissions' && !c.bypass) { M.requestBypass(id); return; }
      if (key === 'perm') c.bypass = false;
      if (key === 'agent') { c.model = AGENTS[val].models[0]; }
      c[key] = val;
      if ((key === 'model' || key === 'agent') && !thinkSupported(c.model)) c.think = null;
      if ((key === 'model' || key === 'agent') && thinkSupported(c.model) && !c.think) c.think = 'Medium';
      const label = { agent: 'Agent', role: 'Role', model: 'Model', think: 'Thinking mode', perm: 'Permission mode' }[key];
      if (c.state !== 'backlog') push(id, SYS(label + ' set to ' + val + '. It takes effect on the next turn.'));
      addAct(id, 'tool', label + ' set to ' + val, '', 'ok');
      emit();
    },
    requestBypass(id) {
      M.confirm({ title: 'Turn on bypass permissions', message: 'The agent on #' + id + ' will run every command and edit without asking. It stays inside this card\'s worktree and every action is still audited.', action: 'Turn on bypass', destructive: true,
        ack: 'I understand the agent can run any command in the worktree without asking.',
        run: () => { const c = card(id); c.perm = 'Bypass permissions'; c.bypass = true; push(id, SYS('Bypass permissions is on for this card. The agent can run any command in its worktree without asking.')); addAct(id, 'approval', 'Bypass permissions turned on by you', '', 'ok'); emit(); toast('Bypass turned on'); } });
    },
    turnOffBypass(id) { const c = card(id); c.bypass = false; c.perm = 'Full auto'; push(id, SYS('Bypass permissions is off. Permission mode set to Full auto.')); emit(); toast('Bypass turned off'); },
    confirm(o) { set({ dialog: Object.assign({ acked: false }, o) }); },
    closeDialog() { set({ dialog: null }); },

    pendingApproval(id) { return (S.chat[id] || []).find(x => (x.k === 'approval' || x.k === 'plan') && x.st === 'waiting'); },
    approve(id) {
      const c = card(id); const a = (S.chat[id] || []).find(x => x.k === 'approval' && x.st === 'waiting');
      if (!a) { const p = (S.chat[id] || []).find(x => x.k === 'plan' && x.st === 'waiting'); if (p) return M.approvePlan(id); return; }
      a.st = 'approved'; M._syncProjectApproval(id, 'approved');
      addAct(id, 'approval', 'You approved: ' + a.cmd, 'Approved', 'ok'); feed('approval', 'You approved ' + a.cmd + ' on #' + id, c.p, { cardId: id });
      c.doing = 'Running ' + a.cmd; setState(c, 'working');
      toast('Approved');
      runTool(id, 'terminal', 'Ran ' + a.cmd, 'grpc v1.64.1 => v1.66.0', 2400, () => {
        streamCard(id, 'Upgraded to v1.66.0. grpc.Dial is deprecated in this version, so I\'ll switch the two call sites to grpc.NewClient and run the tests.', () => {
          seq([[800, () => runTool(id, 'file-pen', 'Edited internal/upstream/conn.go', '+4 −4', 1200)], [1600, () => { c.doing = 'Running go test ./...'; runTool(id, 'terminal', 'Ran go test ./...', '229 passed', 3500, () => { c.doing = 'Committing the upgrade'; emit(); }, 'test'); }]]);
        });
      }, 'command');
    },
    deny(id) {
      const c = card(id); const a = (S.chat[id] || []).find(x => x.k === 'approval' && x.st === 'waiting'); if (!a) return;
      a.st = 'denied'; M._syncProjectApproval(id, 'denied');
      addAct(id, 'approval', 'You denied: ' + a.cmd, 'Denied', 'fail'); feed('approval', 'You denied ' + a.cmd + ' on #' + id, c.p, { cardId: id });
      toast('Denied');
      streamCard(id, 'Understood, I won\'t run it. I can pin grpc-go to v1.65.1 instead, which needs no go.sum changes beyond one line. Want me to try that?', () => setState(c, 'needs', { reason: 'Question: pin to v1.65.1 instead?' }));
    },
    _syncProjectApproval(id, st) { Object.values(S.chats).forEach(cs => cs.forEach(ch => ch.msgs.forEach(x => { if (x.k === 'approval' && x.cardId === id && x.st === 'waiting') x.st = st; }))); },
    approvePlan(id) {
      const c = card(id); const p = S.chat[id].find(x => x.k === 'plan' && x.st === 'waiting'); if (!p) return;
      p.st = 'approved'; p.editing = false;
      addAct(id, 'approval', 'You approved the plan', 'Approved', 'ok'); feed('plan', 'You approved the plan on #' + id, c.p, { cardId: id });
      c.perm = c.perm === 'Plan only' ? 'Auto-accept edits' : c.perm;
      c.doing = p.steps[0]; setState(c, 'working'); toast('Plan approved');
      streamCard(id, 'Thanks. Starting with step 1: ' + p.steps[0].charAt(0).toLowerCase() + p.steps[0].slice(1) + '.', () => {
        runTool(id, 'file-plus', 'Created ' + (p.files[0] || 'new file'), '+84 −0', 2200, () => { c.doing = p.steps[1] || c.doing; emit(); });
      });
    },
    rejectPlan(id) {
      const c = card(id); const p = S.chat[id].find(x => x.k === 'plan' && x.st === 'waiting'); if (!p) return;
      p.st = 'rejected'; p.editing = false;
      addAct(id, 'approval', 'You rejected the plan', 'Rejected', 'fail');
      c.doing = 'Reworking the plan'; setState(c, 'planning'); toast('Plan rejected');
      streamCard(id, 'Understood. I\'ll rework the plan. Tell me what should change, or I\'ll propose a smaller first step.');
    },
    editPlan(id, on) { const p = S.chat[id].find(x => x.k === 'plan' && x.st === 'waiting'); if (p) { p.editing = on; emit(); } },
    savePlan(id, text) {
      const p = S.chat[id].find(x => x.k === 'plan' && x.st === 'waiting'); if (!p) return;
      p.steps = text.split('\n').map(s => s.trim()).filter(Boolean); p.editing = false; p.edited = true;
      addAct(id, 'tool', 'You edited the plan', p.steps.length + ' steps', 'ok'); emit(); toast('Plan saved');
    },
    toggleTool(id, msgId) { const x = (S.chat[id] || []).find(z => z.id === msgId); if (x) { x.open = !x.open; emit(); } },

    send(id, text) {
      const c = card(id); if (!text.trim()) return;
      push(id, U(text));
      const reply = () => {
        const wasNeeds = c.state === 'needs';
        c.doing = 'Reading your message'; if (c.state !== 'done') setState(c, 'working');
        streamCard(id, wasNeeds ? 'Thanks, that answers it. I\'ll continue with that approach and run the tests again.' : 'Got it. I\'ll fold that into the current change and tell you when the tests pass.', () => { c.doing = 'Editing ' + filesFor(c)[0]; emit(); });
      };
      if (c.asleep || c.state === 'backlog') {
        if (c.state === 'backlog') { M._startSession(c); emit(); return; }
        c.waking = true; push(id, SYS('Waking the session. This takes a few seconds.'));
        setTimeout(() => { c.waking = false; c.asleep = false; emit(); reply(); }, 1800);
      } else setTimeout(reply, 500);
    },

    chatSend(pid, cid, text) {
      if (!text.trim()) return;
      const ch = M.chatById(pid, cid); const list = ch.msgs;
      if (ch.fresh) { const w = text.replace(/[?.!]+$/, '').split(/\s+/).slice(0, 6).join(' '); ch.title = w.charAt(0).toUpperCase() + w.slice(1); ch.fresh = false; }
      ch.last = Date.now();
      list.push(U(text)); emit();
      const q = text.toLowerCase();
      const who = ch.target;
      setTimeout(() => {
        if (/block|stuck|waiting/.test(q)) {
          const n = M.needs(pid);
          list.push(m({ k: 'links', text: n.length ? n.length + (n.length === 1 ? ' card is' : ' cards are') + ' waiting on you: ' + n.map(c => '#' + c.id + ' ' + c.reason.charAt(0).toLowerCase() + c.reason.slice(1)).join('. ') + '.' : 'Nothing is blocked right now.', cards: n.map(c => c.id) }));
          emit();
        } else if (/(make|create|add).*(card|task)/.test(q)) {
          const topic = (text.match(/for (the )?(.+)$/i) || [])[2] || 'this work';
          stream(list, 'I split it into three cards. They start in Backlog, and the second depends on the first.', () => {
            const base = [['Design the ' + topic + ' data model', 'Worker'], ['Build the ' + topic + ' API', 'Worker'], ['Test the ' + topic + ' flow', 'Tester']];
            let prev = null;
            base.forEach(([t, r]) => {
              const nc = C(nextId++, pid, t.charAt(0).toUpperCase() + t.slice(1), 'backlog', { role: r, s: 1, e: 4, deps: prev ? [prev] : [], upd: 0 });
              S.cards.push(nc); S.chat[nc.id] = []; S.act[nc.id] = []; S.checks[nc.id] = checksFor(nc); prev = nc.id;
              list.push(m({ k: 'card', cardId: nc.id }));
            });
            emit();
          });
        } else if (/merge/.test(q)) {
          const r = S.cards.find(c => c.p === pid && c.state === 'ready');
          if (r) { r.state = 'merging'; r.mergePct = 10; r.doing = 'Dry-run merge with git merge-tree'; emit(); stream(list, '#' + r.id + ' entered the merge queue. The Integrator is running a dry-run merge first.'); list.push(m({ k: 'card', cardId: r.id })); }
          else stream(list, 'No cards are ready to merge in this project.');
        } else {
          const w = M.working(pid).length, n = M.needs(pid).length;
          stream(list, (who === 'Orchestrator' ? '' : who + ' here. ') + w + ' cards are working and ' + n + ' need you. I can make cards for this, or send it to a card. Try "What is blocked?" or "Make cards for the export work".');
        }
      }, 500);
    },

    chatsOf: pid => (S.chats[pid] || []).slice().sort((a, b) => b.last - a.last),
    chatById: (pid, id) => (S.chats[pid] || []).find(c => c.id === id),
    openChat(pid, id) { S.chatOpen[pid] = id; emit(); },
    newChat(pid, target) { const c = { id: 'ch' + (mid++), pid, title: 'New chat', fresh: true, target: target || 'Orchestrator', msgs: [], last: Date.now(), archived: false }; (S.chats[pid] = S.chats[pid] || []).push(c); S.chatOpen[pid] = c.id; emit(); return c; },
    renameChat(pid, id, t) { const c = M.chatById(pid, id); if (!t || !t.trim()) { toast('Chat names can\'t be empty. The old name is kept.'); emit(); return false; } c.title = t.trim(); c.fresh = false; emit(); return true; },
    archiveChat(pid, id, on) { const c = M.chatById(pid, id); if (!c) return; c.archived = on; if (on && S.chatOpen[pid] === id) S.chatOpen[pid] = null; emit(); if (on) toast('Chat archived', { label: 'Undo', run: () => M.archiveChat(pid, id, false) }); else toast('Chat restored'); },
    deleteChat(pid, id) { const c = M.chatById(pid, id); M.confirm({ title: 'Delete chat', message: 'This deletes "' + c.title + '" and its messages. Cards it created stay on the board.', action: 'Delete chat', destructive: true, run: () => { S.chats[pid] = S.chats[pid].filter(x => x !== c); if (S.chatOpen[pid] === id) S.chatOpen[pid] = null; emit(); toast('Chat deleted'); } }); },
    addProject(o) {
      const id = 'p' + (mid++); const mono = !!o.mono;
      S.projects.push({ id, name: o.name, lang: mono ? 'Monorepo' : (o.lang || 'TypeScript'), path: o.path, branch: o.branch || 'main', ci: 'queued', ciAgo: 0, monthBase: 0, packages: mono ? ['apps/web', 'packages/core', 'packages/ui'] : undefined, runs: [{ wf: 'ci', st: 'queued', ago: 0 }] });
      S.filters[id] = []; S.query[id] = ''; S.swim[id] = mono ? 'package' : 'none'; S.savedViews[id] = [{ name: 'All cards', f: [], swim: 'none' }]; S.savedView[id] = 'All cards'; S.lastView[id] = 'board'; S.chats[id] = []; S.limits[id] = { day: 8, month: 120, awake: 6 };
      if (o.sample) [['Add a health check endpoint', 'backlog'], ['Fix the typo in the README', 'working'], ['Write tests for the date helper', 'review']].forEach(([t, st], i) => { const c = C(nextId++, id, t, st, { branch: st !== 'backlog' ? 'marshal/sample-' + (i + 1) : null, ci: st === 'review' ? 'passed' : null, cost: st === 'backlog' ? 0 : 0.12 * (i + 1), doing: st === 'working' ? 'Editing README.md' : '', s: i, e: i + 2, upd: 5, pr: st === 'review' ? 12 : null }); S.cards.push(c); S.chat[c.id] = genericChat(c); S.act[c.id] = activityFrom(c); S.checks[c.id] = checksFor(c); });
      feed('tool', o.name + ' was added to Marshal', id);
      emit(); return id;
    },
    renameProject(id, name) { if (!name || !name.trim()) { toast('Project names can\'t be empty. The old name is kept.'); emit(); return false; } proj(id).name = name.trim(); emit(); return true; },
    removeProject(id) {
      const p = proj(id);
      S.cards = S.cards.filter(c => c.p !== id); delete S.chats[id];
      S.projects = S.projects.filter(x => x.id !== id);
      S.notices.forEach(n => { if (n.cards) n.cards = n.cards.filter(cid => card(cid)); });
      S.notices = S.notices.filter(n => n.pid !== id && !(n.cardId && !card(n.cardId)) && !(n.cards && !n.cards.length));
      if (S.openId && !card(S.openId)) S.openId = null;
      if (S.route.pid === id) S.route = { page: 'home', pid: S.projects[0] ? S.projects[0].id : null, view: 'board' };
      feed('tool', p.name + ' was removed from Marshal. The repository on disk was not touched.', null);
      emit(); toast('Project removed');
    },
    finishOnboarding() { S.onboarding = false; try { localStorage.setItem('marshal-proto-onboarded', '1'); } catch (e) {} S.route = Object.assign({}, S.route, { page: 'home' }); S.openId = null; S.tour = { step: 0 }; emit(); },
    resetFirstLaunch() { try { localStorage.removeItem('marshal-proto-onboarded'); } catch (e) {} S.onboarding = true; S.obStep = 0; S.tour = null; S.openId = null; S.palette = false; S.dialog = null; S.menu = null; S.route = Object.assign({}, S.route, { page: 'home' }); emit(); },
    startTour() { S.openId = null; S.menu = null; S.palette = false; S.route = Object.assign({}, S.route, { page: 'home' }); S.mobileTab = 'home'; S.tour = { step: 0 }; emit(); },
    setViewport(w, h) { if (S.vw !== w || S.vh !== h) { S.vw = w; S.vh = h; emit(); } },
    person: id => people.find(p => p.id === id),
    toggleItem(cid, lid, iid) { const c = card(cid); const l = c.checklists.find(x => x.id === lid); const it = l.items.find(x => x.id === iid); it.done = !it.done; it.by = it.done ? 'ada' : null; it.doneAt = Date.now(); addAct(cid, 'tool', (it.done ? 'You completed ' : 'You reopened ') + it.text, '', 'ok'); emit(); },
    addItem(cid, lid, text) { if (!text || !text.trim()) return; const l = card(cid).checklists.find(x => x.id === lid); l.items.push({ id: 'it' + (ck++), text: text.trim(), done: false, by: null }); emit(); },
    removeItem(cid, lid, iid) { const l = card(cid).checklists.find(x => x.id === lid); l.items = l.items.filter(x => x.id !== iid); emit(); },
    addChecklist(cid, title) { const c = card(cid); c.checklists.push({ id: 'cl' + (ck++), title: (title || '').trim() || 'Checklist', hideDone: false, items: [] }); emit(); toast('Checklist added'); },
    deleteChecklist(cid, lid) { const c = card(cid); const l = c.checklists.find(x => x.id === lid); M.confirm({ title: 'Delete checklist', message: 'This deletes "' + l.title + '" and its ' + l.items.length + ' items.', action: 'Delete checklist', destructive: true, run: () => { c.checklists = c.checklists.filter(x => x !== l); emit(); toast('Checklist deleted'); } }); },
    toggleHideDone(cid, lid) { const l = card(cid).checklists.find(x => x.id === lid); l.hideDone = !l.hideDone; emit(); },
    addComment(cid, text, att) {
      const c = card(cid); if (!text.trim() && !(att && att.length)) return;
      const links = (text.match(/https?:\/\/[^\s)]+/g) || []).map(u => ({ kind: 'link', name: u.replace(/^https?:\/\//, ''), url: u }));
      const cm = { id: 'co' + (ck++), author: 'ada', text: text.trim(), ts: Date.now(), att: (att || []).concat(links), read: false };
      c.comments.push(cm); addAct(cid, 'tool', 'You commented', '', 'ok'); emit(); toast('Comment added');
      if (M.isAwake(c)) setTimeout(() => { cm.read = true; addAct(cid, 'tool', c.agent + ' read your comment', '', 'ok'); emit();
        if (/@agent|@/.test(text) || /\?$/.test(text.trim())) setTimeout(() => { c.comments.push({ id: 'co' + (ck++), author: 'agent', text: 'Got it. I added this to the plan for my next turn' + (cm.att.length ? ' and read the ' + cm.att.length + (cm.att.length === 1 ? ' attachment' : ' attachments') : '') + '.', ts: Date.now(), att: [], read: true }); emit(); }, 1600); }, 1200);
    },
    deleteComment(cid, id) { const c = card(cid); c.comments = c.comments.filter(x => x.id !== id); emit(); toast('Comment deleted'); },
    toggleMember(cid, pid) { const c = card(cid); c.members = c.members.includes(pid) ? c.members.filter(x => x !== pid) : c.members.concat([pid]); addAct(cid, 'tool', (c.members.includes(pid) ? 'Added ' : 'Removed ') + M.person(pid).name, '', 'ok'); emit(); },
    quickAdd(pid, col, title) {
      if (!title || !title.trim()) return;
      const c = C(nextId++, pid, title.trim(), 'backlog', { upd: 0, s: 0, e: 3, perm: col === 'planning' ? 'Plan only' : 'Auto-accept edits', members: ['ada'] });
      S.cards.push(c); S.chat[c.id] = []; S.act[c.id] = []; S.checks[c.id] = checksFor(c);
      if (col === 'planning' || col === 'working') M._startSession(c, col);
      emit(); toast(col === 'backlog' ? 'Card created' : 'Card created and started', { label: 'Open', run: () => M.openCard(c.id) });
    },
    newCard(opts) { set({ newCard: Object.assign({ title: '', body: '', template: 'Blank', role: 'Worker', agent: 'Claude Code', start: false }, opts || {}) }); },
    createCard() {
      const n = S.newCard; if (!n || !n.title.trim()) return;
      const pid = S.route.pid;
      const c = C(nextId++, pid, n.title.trim(), 'backlog', { role: n.role, agent: n.agent, model: AGENTS[n.agent].models[0], upd: 0, s: 0, e: 3, perm: n.template === 'Plan first' ? 'Plan only' : 'Auto-accept edits' });
      S.cards.push(c); S.chat[c.id] = []; S.act[c.id] = []; S.checks[c.id] = checksFor(c);
      if (n.body) S.chat[c.id].push(U(n.body));
      S.newCard = null;
      if (n.start) M._startSession(c);
      emit(); toast('Card created', { label: 'Open', run: () => M.openCard(c.id) });
    },
    dupes(title) {
      const w = title.toLowerCase().split(/\W+/).filter(x => x.length > 3);
      if (w.length < 2) return [];
      return S.cards.filter(c => c.p === S.route.pid && c.state !== 'done').filter(c => { const t = c.title.toLowerCase(); return w.filter(x => t.includes(x)).length >= 2; }).slice(0, 2);
    },

    simulateCiFailure(id) { ciFailure(id || 40, true); toast('Simulating a CI failure on #' + (id || 40)); },
    setTheme(t) { S.theme = t; applyTheme(); emit(); },
    runChecks(id) {
      const ks = S.checks[id]; ks.forEach(k => { if (k.cmd) k.st = 'running'; }); emit();
      ks.forEach((k, i) => k.cmd && setTimeout(() => { k.st = card(id).ci === 'failed' && i === 0 ? 'failed' : 'passed'; addAct(id, 'test', 'Check: ' + k.name, k.st === 'passed' ? 'Passed' : 'Failed', k.st === 'passed' ? 'ok' : 'fail'); emit(); }, 1500 + i * 900));
    },

    filtered(pid) {
      const f = S.filters[pid] || []; const q = (S.query[pid] || '').toLowerCase();
      return M.cardsOf(pid).filter(c => {
        const by = {};
        f.forEach(x => { (by[x.k] = by[x.k] || []).push(x.v); });
        if (by.status && !by.status.includes(colOf(c.state))) return false;
        if (by.role && !by.role.includes(c.role)) return false;
        if (by.agent && !by.agent.includes(c.agent)) return false;
        if (by.model && !by.model.includes(c.model)) return false;
        if (by.label && !by.label.some(l => c.labels.includes(l))) return false;
        if (by.package && !by.package.includes(c.pkg || 'No package')) return false;
        if (q && !(c.title.toLowerCase().includes(q) || ('#' + c.id).includes(q) || (c.branch || '').includes(q))) return false;
        return true;
      });
    },
    addFilter(k, v) { const pid = S.route.pid; const f = S.filters[pid]; if (!f.find(x => x.k === k && x.v === v)) S.filters[pid] = f.concat([{ k, v }]); S.savedView[pid] = null; S.menu = null; emit(); },
    removeFilter(k, v) { const pid = S.route.pid; S.filters[pid] = S.filters[pid].filter(x => !(x.k === k && x.v === v)); S.savedView[pid] = null; emit(); },
    clearFilters() { const pid = S.route.pid; S.filters[pid] = []; S.query[pid] = ''; emit(); },
    applyView(name) { const pid = S.route.pid; const v = S.savedViews[pid].find(x => x.name === name); if (!v) return; S.filters[pid] = v.f.slice(); S.swim[pid] = v.swim; S.savedView[pid] = name; S.menu = null; emit(); },
    saveView(name) { const pid = S.route.pid; if (!name) return; S.savedViews[pid] = S.savedViews[pid].filter(v => v.name !== name).concat([{ name, f: S.filters[pid].slice(), swim: S.swim[pid] }]); S.savedView[pid] = name; S.menu = null; emit(); toast('View saved'); },

    commands() {
      const out = [];
      const pid = S.route.pid;
      out.push({ group: 'Actions', label: 'New card', icon: 'plus', kbd: ['N'], run: () => { if (S.route.page !== 'project') M.go('project', pid); M.newCard(); } });
      VIEWS.forEach((v, i) => out.push({ group: 'Actions', label: 'Switch to ' + v.label.toLowerCase() + ' view', icon: v.icon, kbd: ['⌘', String(i + 1)], run: () => { if (S.route.page !== 'project') M.go('project', pid, v.key); else M.setView(v.key); } }));
      out.push({ group: 'Actions', label: 'Go home', icon: 'home', run: () => M.go('home') });
      out.push({ group: 'Actions', label: 'New project', icon: 'folder-plus', run: () => set({ newProject: { source: 'folder', path: '', url: '', name: '', branch: 'main' } }) });
      out.push({ group: 'Actions', label: 'New chat', icon: 'message-square-plus', run: () => { M.go('project', pid, 'chat'); set({ newChatOpen: true }); } });
      out.push({ group: 'Actions', label: 'Replay tour', icon: 'map', run: () => M.startTour() });
      if (S.openId) {
        const c = card(S.openId);
        out.push({ group: 'Actions', label: (c.pinned ? 'Unpin' : 'Pin') + ' #' + c.id, icon: 'pin', kbd: ['P'], run: () => M.pin(c.id) });
        out.push({ group: 'Actions', label: c.asleep ? 'Resume session on #' + c.id : 'Sleep #' + c.id, icon: 'moon', kbd: ['S'], run: () => c.asleep ? M.wake(c.id) : M.sleep(c.id) });
        out.push({ group: 'Actions', label: 'Fork #' + c.id, icon: 'git-fork', run: () => M.fork(c.id) });
        if (M.pendingApproval(c.id)) out.push({ group: 'Actions', label: 'Approve on #' + c.id, icon: 'check', kbd: ['A'], run: () => M.approve(c.id) });
      }
      const rm = S.cards.find(c => c.state === 'ready' && c.p === pid);
      if (rm) out.push({ group: 'Actions', label: 'Merge #' + rm.id + ' ' + rm.title, icon: 'git-merge', run: () => { rm.state = 'merging'; rm.mergePct = 10; rm.doing = 'Dry-run merge with git merge-tree'; emit(); toast('Added to merge queue'); } });
      out.push({ group: 'Actions', label: 'Simulate CI failure on #40', icon: 'circle-x', run: () => M.simulateCiFailure(40) });
      out.push({ group: 'Actions', label: 'Show notices', icon: 'bell', run: () => set({ noticesOpen: true }) });
      S.projects.forEach(p => out.push({ group: 'Projects', label: p.name, icon: 'folder-git-2', hint: p.lang, run: () => M.go('project', p.id) }));
      S.cards.forEach(c => out.push({ group: 'Cards', label: '#' + c.id + ' ' + c.title, icon: STATUS[c.state].icon, iconColor: tone(STATUS[c.state].tone, 'solid'), hint: proj(c.p).name, run: () => M.openCard(c.id) }));
      [['profile', 'Profile'], ['project', 'Project settings'], ['help', 'Help'], ['general', 'Theme'], ['roles', 'Roles'], ['providers', 'Provider keys'], ['limits', 'Cost and awake limits'], ['schedules', 'Schedules'], ['integrations', 'Integrations'], ['shortcuts', 'Keyboard shortcuts']].forEach(([k, l]) => out.push({ group: 'Settings', label: l, icon: 'settings', run: () => { S.settingsSection = k; M.go('settings'); } }));
      out.push({ group: 'Settings', label: 'Use light theme', icon: 'sun', run: () => M.setTheme('light') });
      out.push({ group: 'Settings', label: 'Use dark theme', icon: 'moon', run: () => M.setTheme('dark') });
      out.push({ group: 'Settings', label: 'Use system theme', icon: 'monitor', run: () => M.setTheme('system') });
      return out;
    },

    dragStart(e, id) {
      if (e.button !== 0 || M.mobile) return;
      if (e.target.closest && e.target.closest('button')) return;
      const el = e.currentTarget; const sx = e.clientX, sy = e.clientY; let ghost = null; const r = el.getBoundingClientRect();
      const c = card(id);
      const move = ev => {
        const dx = ev.clientX - sx, dy = ev.clientY - sy;
        if (!ghost) {
          if (Math.hypot(dx, dy) < 5) return;
          ghost = el.cloneNode(true);
          Object.assign(ghost.style, { position: 'fixed', left: r.left + 'px', top: r.top + 'px', width: r.width + 'px', zIndex: 450, pointerEvents: 'none', boxShadow: 'var(--elevation-drag)', margin: 0, opacity: 1, transition: 'none' });
          document.body.appendChild(ghost); M._suppressClick = true; set({ dragId: id });
        }
        ghost.style.transform = 'translate(' + dx + 'px,' + dy + 'px)';
        const t = document.elementFromPoint(ev.clientX, ev.clientY);
        const ce = t && t.closest && t.closest('[data-col]');
        const cv = ce ? ce.getAttribute('data-col') : null;
        if (cv !== S.dropCol) set({ dropCol: cv });
      };
      const end = () => {
        window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', end); window.removeEventListener('pointercancel', end);
        if (ghost) {
          ghost.remove(); const to = S.dropCol; set({ dragId: null, dropCol: null });
          if (to && to !== colOf(c.state)) M.moveCard(id, to, true);
          setTimeout(() => { M._suppressClick = false; }, 30);
        }
      };
      window.addEventListener('pointermove', move); window.addEventListener('pointerup', end); window.addEventListener('pointercancel', end);
    }
  };

  function applyTheme() {
    const sys = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
    const t = S.theme === 'system' ? (sys ? 'dark' : 'light') : S.theme;
    document.documentElement.setAttribute('data-theme', t);
    S.resolvedTheme = t;
  }
  applyTheme();
  window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => { applyTheme(); emit(); });
  window.addEventListener('resize', () => { if (!M._framed && S.vw !== window.innerWidth) set({ vw: window.innerWidth }); });

  if (!/nosim/.test(location.hash)) {
  const hs = location.hash;
  if (!/n41/.test(hs)) setTimeout(script41, 1500);
  if (!/n46/.test(hs)) setTimeout(script46, 4000);
  if (!/nm/.test(hs)) setTimeout(scriptMerge35, 3000);
  if (!/nci/.test(hs)) setTimeout(() => ciFailure(40, false), 5000);
  if (!/nt/.test(hs)) setInterval(tick, 1000);
  }
  window.dispatchEvent(new Event('marshal-ready'));
})();
