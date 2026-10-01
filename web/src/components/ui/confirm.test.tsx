// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { confirm, ConfirmDialog } from "./confirm";

afterEach(cleanup);

const open = (options: Partial<Parameters<typeof confirm>[0]> = {}) =>
  act(() =>
    confirm({
      title: "Delete heartbeat",
      message: "Gone for good.",
      confirmLabel: "Delete",
      action: vi.fn(),
      ...options,
    }),
  );

describe("confirm dialog", () => {
  it("runs the action and closes", async () => {
    const action = vi.fn().mockResolvedValue(undefined);
    render(<ConfirmDialog />);
    open({ action });
    expect(screen.getByRole("alertdialog")).toBeTruthy();
    // The safe choice has focus.
    expect(document.activeElement?.textContent).toBe("Cancel");
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Delete" })));
    expect(action).toHaveBeenCalledOnce();
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("needs the name typed before deleting", () => {
    render(<ConfirmDialog />);
    open({ typeToConfirm: "Nightly backup" });
    const button = screen.getByRole("button", { name: "Delete" }) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Type Nightly backup to confirm"), { target: { value: "Nightly backup" } });
    expect(button.disabled).toBe(false);
  });

  it("keeps the dialog open with the error when the action fails", async () => {
    render(<ConfirmDialog />);
    open({ action: () => Promise.reject(new Error("Can't reach the agent.")) });
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Delete" })));
    expect(screen.getByRole("alertdialog")).toBeTruthy();
    expect(screen.getByRole("alert").textContent).toContain("Can't reach the agent.");
  });

  it("closes on Escape without running the action", () => {
    const action = vi.fn();
    render(<ConfirmDialog />);
    open({ action });
    fireEvent.keyDown(screen.getByRole("alertdialog"), { key: "Escape" });
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(action).not.toHaveBeenCalled();
  });
});
