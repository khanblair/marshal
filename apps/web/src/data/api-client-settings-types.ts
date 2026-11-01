import type {
  AlertSettings,
  CreateRoleRequest,
  LimitList,
  NoticeActionRequest,
  NoticeActionResult,
  NoticeList,
  Role,
  RoleList,
  RoleSpec,
  SaveAlertSettingsRequest,
  SetLimitRequest,
  SleepSettings,
  UpdateRoleRequest,
} from "@marshal/protocol";
import type { CallOptions } from "./api-client-types";

/** The roles, limits, notices, sleep and alert routes of the daemon, kept apart so each file stays short. */
export interface SettingsApi {
  /** Every cost and awake ceiling that is set, global ones first (sections S26b, S19b). */
  listLimits(options?: CallOptions): Promise<LimitList>;
  /** Sets one ceiling. The scope and kind come from the path; the body carries only the number. */
  setLimit(
    scope: string,
    kind: string,
    body: SetLimitRequest,
    options?: CallOptions,
  ): Promise<LimitList>;
  /** Removes one ceiling. A scope that had none of that kind is not an error. */
  deleteLimit(scope: string, kind: string, options?: CallOptions): Promise<LimitList>;
  /**
   * Every role Marshal knows (section S27): Marshal's starter roles first, then the roles a person
   * made. The project is optional and changes only whether a role reads as overridden, because the
   * roles themselves are global.
   */
  listRoles(project?: string, options?: CallOptions): Promise<RoleList>;
  /** Adds a role, or imports one. The body is the export document; a name another role has is a conflict. */
  createRole(body: CreateRoleRequest, project?: string, options?: CallOptions): Promise<RoleList>;
  /** One role by its name. */
  getRole(name: string, project?: string, options?: CallOptions): Promise<Role>;
  /** Renames a role, replaces its body, or both. A field left out is not touched. */
  updateRole(
    name: string,
    body: UpdateRoleRequest,
    project?: string,
    options?: CallOptions,
  ): Promise<RoleList>;
  /** Removes a role a person made. One of Marshal's own is refused; reset it instead. */
  deleteRole(name: string, project?: string, options?: CallOptions): Promise<RoleList>;
  /** Clears one project's own version of a role, leaving the role itself exactly as it is. */
  resetRole(name: string, project: string, options?: CallOptions): Promise<RoleList>;
  /** Gives one project its own version of a role. The body is the spec alone, not an export document. */
  setRoleOverride(
    name: string,
    project: string,
    spec: RoleSpec,
    options?: CallOptions,
  ): Promise<RoleList>;
  /**
   * Every notice that is standing (section S23), the idle-card sleep groups today. It is never null,
   * so an empty answer is an empty array and a client never handles both.
   */
  listNotices(options?: CallOptions): Promise<NoticeList>;
  /**
   * One of the four calls a person makes on a notice (inventory N5): keep one card awake, sleep one
   * card now, keep every card the notice names awake, or sleep them all now. The notice id names the
   * group and the two per-card calls carry the card. The answer is how many cards it changed, so a
   * toast can say a number.
   */
  noticeAction(
    notice: string,
    body: NoticeActionRequest,
    options?: CallOptions,
  ): Promise<NoticeActionResult>;
  /** Takes one notice off the panel. A notice that is already gone is not an error. */
  dismissNotice(notice: string, options?: CallOptions): Promise<void>;
  /** The numbers and choices behind automatic sleep (section S26a). */
  sleepSettings(options?: CallOptions): Promise<SleepSettings>;
  /** Saves them. A value the screen would not offer is refused with the sentence the form shows. */
  saveSleepSettings(body: SleepSettings, options?: CallOptions): Promise<SleepSettings>;
  /** Which channel each kind of alert goes to, and which channels are connected (section S26c). */
  alertSettings(options?: CallOptions): Promise<AlertSettings>;
  /** Changes where some alerts go. An alert left out keeps its channels; an unknown one is refused. */
  saveAlertSettings(body: SaveAlertSettingsRequest, options?: CallOptions): Promise<AlertSettings>;
}
