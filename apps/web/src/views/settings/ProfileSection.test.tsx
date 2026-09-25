import { cleanup, fireEvent, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { confirmDialog, lastToast, showSettings } from "./test-support";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => {
  vi.useFakeTimers();
});
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
});

const nameField = () => screen.getByLabelText<HTMLInputElement>("Name");
const saveButton = () => screen.getByRole("button", { name: "Save profile" });

describe("Profile section", () => {
  it("shows the saved profile, with save off until something changes", () => {
    showSettings("profile");
    expect(screen.getByRole("heading", { name: "Profile", level: 2 })).toBeInTheDocument();
    expect(nameField()).toHaveValue(M.S.profile.name);
    expect(screen.getByLabelText("Email", { exact: false })).toHaveAttribute("type", "email");
    expect(screen.getByLabelText("Time zone", { exact: false })).toHaveValue(M.S.profile.tz);
    expect(saveButton()).toBeDisabled();
    expect(saveButton()).toHaveAttribute("type", "submit");
    expect(screen.getByLabelText("Your avatar")).toHaveTextContent("AO");
    expect(screen.getByText("ada@kolaborate.co")).toBeInTheDocument();
    expect(screen.getByText("marshal-laptop.tail3f2a.ts.net")).toBeInTheDocument();
  });

  it("follows the name being typed in the initials, and saves the trimmed edits", () => {
    showSettings("profile");
    fireEvent.input(nameField(), { target: { value: "  grace brewster hopper " } });
    fireEvent.input(screen.getByLabelText("Email", { exact: false }), { target: { value: " grace@navy.mil " } });
    fireEvent.change(screen.getByLabelText("Time zone", { exact: false }), { target: { value: "Asia/Singapore" } });
    expect(screen.getByLabelText("Your avatar")).toHaveTextContent("GB");
    expect(saveButton()).toBeEnabled();
    fireEvent.click(saveButton());
    expect(M.S.profile).toMatchObject({
      name: "grace brewster hopper",
      email: "grace@navy.mil",
      tz: "Asia/Singapore",
    });
    expect(lastToast()).toBe("Profile saved");
    expect(saveButton()).toBeDisabled();
  });

  it("asks for a name and keeps save off while the name is empty", () => {
    showSettings("profile");
    fireEvent.input(nameField(), { target: { value: "  " } });
    expect(screen.getByText("Enter a name. It shows on cards you comment on.")).toHaveClass(
      "text-status-danger-text",
    );
    expect(saveButton()).toBeDisabled();
    fireEvent.input(screen.getByLabelText("Email", { exact: false }), { target: { value: "a@b.c" } });
    expect(saveButton()).toBeDisabled();
  });

  it("toasts for Upload image", () => {
    showSettings("profile");
    fireEvent.click(screen.getByRole("button", { name: "Upload image" }));
    expect(lastToast()).toBe("Choose an image to use as your avatar");
  });

  it("lists paired devices, and asks before removing one", () => {
    showSettings("profile");
    expect(screen.getByText("Pixel 8")).toBeInTheDocument();
    expect(screen.getByText("Last seen 40 min ago")).toBeInTheDocument();
    expect(screen.getByText("Last seen 2 days ago")).toBeInTheDocument();
    fireEvent.click(screen.getAllByRole("button", { name: "Remove device" })[0] as HTMLElement);
    expect(M.S.dialog).toMatchObject({
      title: "Remove device",
      message: "Pixel 8 will lose access to Marshal and must pair again to reconnect.",
      action: "Remove device",
      destructive: true,
    });
    expect(M.S.profile.devices).toHaveLength(2);
    confirmDialog();
    expect(M.S.profile.devices.map((d) => d.name)).toEqual(["iPad Air"]);
    expect(lastToast()).toBe("Device removed");
    expect(screen.queryByText("Pixel 8")).toBeNull();
  });

  it("says so when there are no paired devices", () => {
    showSettings("profile");
    M.S.profile.devices = [];
    expect(screen.getByText("No paired devices.")).toBeInTheDocument();
  });

  it("shows a pairing code after Pair a device", () => {
    showSettings("profile");
    expect(screen.queryByText("7QX-2LD")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Pair a device" }));
    expect(screen.getByText("Enter this code on the device")).toBeInTheDocument();
    expect(screen.getByText("7QX-2LD").tagName).toBe("CODE");
  });

  it("submitting the form does not save when nothing changed", () => {
    showSettings("profile");
    const before = M.S.toasts.length;
    fireEvent.submit(
      within(document.body)
        .getByRole("button", { name: "Save profile" })
        .closest("form") as HTMLFormElement,
    );
    expect(M.S.toasts).toHaveLength(before);
  });
});
