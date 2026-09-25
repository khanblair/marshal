import { ROLE_NAMES } from "../constants";
import type { Integration, Provider, Role } from "../settings-types";

type RoleDefaults = Pick<
  Role,
  "desc" | "agent" | "model" | "think" | "perm" | "strength" | "instr"
>;

const ROLE_DEFAULTS: Record<string, RoleDefaults> = {
  Orchestrator: {
    desc: "Plans work and splits goals into cards",
    agent: "Claude Code",
    model: "claude-opus-4-1",
    think: "High",
    perm: "Plan only",
    strength: "Strong or medium",
    instr:
      "You plan work for this project. Explore the codebase and memory, propose a plan, and create cards with the right roles and dependencies after the user approves.",
  },
  Worker: {
    desc: "Does the coding on a card",
    agent: "Claude Code",
    model: "claude-sonnet-4-5",
    think: "Medium",
    perm: "Auto-accept edits",
    strength: "Your choice",
    instr:
      "You do the coding on one card. Stay inside your worktree, claim the files you change, and keep commits small.",
  },
  Reviewer: {
    desc: "Reviews every pull request before you do",
    agent: "Claude Code",
    model: "claude-opus-4-1",
    think: "High",
    perm: "Plan only",
    strength: "Strong",
    instr:
      "Review the diff against the card's task and acceptance checks. Leave specific comments. Approve only when every check passes.",
  },
  Integrator: {
    desc: "Merges finished work into the target branch",
    agent: "Claude Code",
    model: "claude-opus-4-1",
    think: "High",
    perm: "Full auto",
    strength: "Strong",
    instr:
      "Merge one card at a time. Dry-run with git merge-tree first. Resolve conflicts by intent. Stop and explain when you are not confident.",
  },
  Tester: {
    desc: "Writes and runs tests",
    agent: "Codex",
    model: "gpt-5-codex",
    think: "Medium",
    perm: "Full auto",
    strength: "Medium",
    instr:
      "Write focused tests for the change and run them. Report flaky tests separately from real failures.",
  },
  "Docs writer": {
    desc: "Writes and updates documentation",
    agent: "Built-in agent",
    model: "claude-haiku-4-5",
    think: "Low",
    perm: "Auto-accept edits",
    strength: "Medium",
    instr: "Update docs to match the code. Use plain, short sentences.",
  },
  "Security checker": {
    desc: "Looks for security problems in changes",
    agent: "Claude Code",
    model: "claude-opus-4-1",
    think: "Extra high",
    perm: "Plan only",
    strength: "Strong",
    instr: "Look for injection, auth bypass, secrets, and unsafe dependencies in the diff.",
  },
  "UI checker": {
    desc: "Checks UI changes with previews and screenshots",
    agent: "Claude Code",
    model: "claude-sonnet-4-5",
    think: "Medium",
    perm: "Plan only",
    strength: "Medium",
    instr:
      "Open the live preview, capture before and after screenshots, and compare them against the card.",
  },
};

function skillsFor(name: string): string[] {
  if (name === "UI checker") return ["screenshot-compare"];
  return name === "Tester" ? ["go-testing", "playwright"] : ["conventional-commits"];
}

export const seedRoles = (): Role[] =>
  ROLE_NAMES.flatMap((name) => {
    const defaults = ROLE_DEFAULTS[name];
    if (!defaults) return [];
    return [
      {
        name,
        starter: true,
        overridden: name === "Worker",
        skills: skillsFor(name),
        mcp: ["marshal", "github"],
        limits: { time: 60, cost: 5, rounds: 12 },
        backup: "gpt-5",
        ...defaults,
      },
    ];
  });

export const seedProviders = (): Provider[] => [
  {
    id: "anthropic",
    name: "Anthropic",
    st: "saved",
    masked: "sk-ant-…4f2a",
    models: "Claude models",
  },
  { id: "openai", name: "OpenAI", st: "saved", masked: "sk-proj-…91cd", models: "GPT models" },
  {
    id: "gemini",
    name: "Google Gemini",
    st: "saved",
    masked: "AIza…7Qe0",
    models: "Gemini models",
  },
  { id: "deepseek", name: "DeepSeek", st: "empty", masked: "", models: "DeepSeek models" },
  {
    id: "openrouter",
    name: "OpenRouter",
    st: "invalid",
    masked: "sk-or-…0b33",
    models: "Any model on OpenRouter",
    error:
      "OpenRouter rejected this key. Create a new key at openrouter.ai/keys and paste it here.",
  },
  {
    id: "ollama",
    name: "Ollama",
    st: "saved",
    masked: "http://localhost:11434",
    models: "Local models",
    local: true,
  },
];

export const seedIntegrations = (): Integration[] => [
  {
    id: "github",
    name: "GitHub",
    icon: "github",
    st: "connected",
    detail: "GitHub App installed on 3 repositories",
  },
  {
    id: "trello",
    name: "Trello",
    icon: "trello",
    st: "connected",
    detail: "web-dashboard syncs with the board Dashboard roadmap",
  },
  {
    id: "gcal",
    name: "Google Calendar",
    icon: "calendar",
    st: "connected",
    detail: "Reading 2 calendars for briefs and the calendar view",
  },
  {
    id: "gmail",
    name: "Gmail",
    icon: "mail",
    st: "none",
    detail: "Turn labeled emails into cards",
  },
  {
    id: "telegram",
    name: "Telegram",
    icon: "send",
    st: "connected",
    detail: "Approvals and briefs go to @marshal_ops_bot",
  },
  {
    id: "discord",
    name: "Discord",
    icon: "message-circle",
    st: "error",
    detail: "The bot token expired. Reconnect Discord to keep approvals working there.",
  },
  {
    id: "obsidian",
    name: "Obsidian",
    icon: "book-open",
    st: "connected",
    detail: "Vault at ~/Notes/Marshal",
  },
];
