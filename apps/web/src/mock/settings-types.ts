/* Settings entities: roles, provider keys, integrations, schedules, calendar, profile. */

export interface Role {
  name: string;
  starter: boolean;
  overridden: boolean;
  skills: string[];
  mcp: string[];
  limits: { time: number; cost: number; rounds: number };
  backup: string;
  desc: string;
  agent: string;
  model: string;
  think: string;
  perm: string;
  strength: string;
  instr: string;
}
export interface Provider {
  id: string;
  name: string;
  st: "saved" | "empty" | "invalid";
  masked: string;
  models: string;
  error?: string;
  local?: boolean;
}
export interface Integration {
  id: string;
  name: string;
  icon: string;
  st: "connected" | "none" | "error";
  detail: string;
}
export interface Schedule {
  id: string;
  name: string;
  kind: "brief" | "job";
  icon: string;
  trigger: string;
  when: string;
  time: string;
  days: number[];
  action: string;
  project: string;
  enabled: boolean;
  missed: string;
}
export interface CalEvent {
  id: string;
  title: string;
  time: string;
  days?: number[];
  dayOffset?: number;
}
interface Device {
  id: string;
  name: string;
  kind: string;
  last: number;
}
export interface Profile {
  name: string;
  email: string;
  tz: string;
  avatar: string | null;
  tailnet: string;
  node: string;
  devices: Device[];
}
