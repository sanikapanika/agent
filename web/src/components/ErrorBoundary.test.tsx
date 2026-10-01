// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { ErrorBoundary } from "./ErrorBoundary";

afterEach(cleanup);

function Broken(): never {
  throw new Error("boom");
}

describe("ErrorBoundary", () => {
  it("shows a way out instead of a blank page", () => {
    vi.spyOn(console, "error").mockImplementation(() => {}); // React logs the error too
    render(
      <ErrorBoundary>
        <Broken />
      </ErrorBoundary>,
    );
    expect(screen.getByRole("alert").textContent).toContain("boom");
    expect(screen.getByRole("button", { name: /Reload/ })).toBeTruthy();
  });
});
