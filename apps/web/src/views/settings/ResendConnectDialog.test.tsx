import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type Integration, M } from "~/mock";
import { createEditState } from "./edit-state";
import * as actions from "./integration-actions";
import { ResendConnectDialog } from "./ResendConnectDialog";

vi.mock("./integration-actions", () => ({
  connectResend: vi.fn(async () => true),
  sendResendTest: vi.fn(async () => ({ ok: false, checks: [] })),
}));

beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.useRealTimers();
});

const resend = (st: Integration["st"] = "none"): Integration => ({
  id: "resend",
  name: "Email (Resend)",
  icon: "mail",
  st,
  detail: "",
});

/** Opens the dialog for a connection and returns its edit state, so a test can read what it says. */
function open(integration: Integration) {
  const edit = createEditState();
  edit.open(integration.id);
  render(() => <ResendConnectDialog integration={integration} edit={edit} />);
  return edit;
}

const field = (label: string) => screen.getByLabelText(new RegExp(`^${label}`)) as HTMLInputElement;

describe("the Resend email dialog", () => {
  it("asks for the sender address and the key, and refuses a sender with no @", async () => {
    open(resend());
    fireEvent.input(field("Sender address"), { target: { value: "not-an-address" } });
    fireEvent.input(field("API key"), { target: { value: "re_key" } });
    fireEvent.click(screen.getByRole("button", { name: "Save connection" }));
    expect(actions.connectResend).not.toHaveBeenCalled();
    expect(
      screen.getByText(/Enter the address Marshal should send email from/),
    ).toBeInTheDocument();
  });

  it("saves the sender and the key, and sends nothing on save", async () => {
    open(resend());
    fireEvent.input(field("Sender address"), {
      target: { value: "Marshal <marshal@kolaborate.co>" },
    });
    fireEvent.input(field("API key"), { target: { value: "re_key" } });
    fireEvent.click(screen.getByRole("button", { name: "Save connection" }));
    await waitFor(() =>
      expect(actions.connectResend).toHaveBeenCalledWith({
        from: "Marshal <marshal@kolaborate.co>",
        key: "re_key",
      }),
    );
    expect(actions.sendResendTest).not.toHaveBeenCalled();
  });

  it("offers Send test email only once the connection is saved, and shows what it found", async () => {
    vi.mocked(actions.sendResendTest).mockResolvedValueOnce({
      ok: true,
      checks: [
        {
          name: "Send test email",
          state: "passed",
          message: "Sent to owner@example.com.",
          fix: "",
        },
      ],
      testedAt: "2026-10-31T12:00:00.000Z",
    } as never);
    open(resend());
    expect(screen.queryByRole("button", { name: "Send test email" })).toBeNull();
    cleanup();
    open(resend("connected"));
    fireEvent.click(screen.getByRole("button", { name: "Send test email" }));
    await waitFor(() => expect(screen.getByText("Sent to owner@example.com.")).toBeInTheDocument());
    expect(actions.sendResendTest).toHaveBeenCalledTimes(1);
  });

  it("opens with Resend's own sender filled in when nothing is saved, and saves it as typed", async () => {
    open(resend());
    expect(field("Sender address").value).toBe("Marshal <onboarding@resend.dev>");
    fireEvent.input(field("API key"), { target: { value: "re_key" } });
    fireEvent.click(screen.getByRole("button", { name: "Save connection" }));
    await waitFor(() =>
      expect(actions.connectResend).toHaveBeenCalledWith({
        from: "Marshal <onboarding@resend.dev>",
        key: "re_key",
      }),
    );
  });

  it("uses Resend's own sender when the field is cleared", async () => {
    open(resend());
    fireEvent.input(field("Sender address"), { target: { value: "   " } });
    fireEvent.input(field("API key"), { target: { value: "re_key" } });
    fireEvent.click(screen.getByRole("button", { name: "Save connection" }));
    await waitFor(() =>
      expect(actions.connectResend).toHaveBeenCalledWith({
        from: "Marshal <onboarding@resend.dev>",
        key: "re_key",
      }),
    );
  });

  it("opens with the saved sender filled in, so the key can be rotated without retyping it", () => {
    open({ ...resend("connected"), target: "Marshal <marshal@kolaborate.co>" });
    expect(field("Sender address").value).toBe("Marshal <marshal@kolaborate.co>");
  });

  it("shows why a test email failed instead of doing nothing", async () => {
    vi.mocked(actions.sendResendTest).mockResolvedValueOnce({
      ok: false,
      checks: [
        {
          name: "Send test email",
          state: "failed",
          message: "Resend refused the email: verify a domain",
        },
      ],
    });
    open(resend("connected"));
    fireEvent.click(screen.getByRole("button", { name: "Send test email" }));
    await waitFor(() => expect(screen.getByText(/verify a domain/)).toBeInTheDocument());
  });

  it("is kept out of the prototype's list: the store shows its row only once the daemon reports it", () => {
    expect(M.S.integrations.some((row) => row.id === "resend")).toBe(false);
  });
});
