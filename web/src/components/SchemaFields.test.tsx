// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { SchemaField } from "@/lib/api";
import { SchemaFields } from "./SchemaFields";

afterEach(cleanup);

const fields: SchemaField[] = [
  { key: "method", label: "Method", input: "select", options: ["GET", "POST"], default: "GET" },
  { key: "days", label: "Days", input: "number", default: 14 },
  { key: "insecure", label: "Ignore TLS", input: "switch" },
  { key: "token", label: "Bot token", input: "password", required: true },
];

describe("SchemaFields", () => {
  it("renders each input kind with its default", () => {
    render(<SchemaFields idPrefix="t" fields={fields} values={{}} onChange={() => {}} />);
    expect((screen.getByLabelText("Method") as HTMLSelectElement).value).toBe("GET");
    expect((screen.getByLabelText("Days") as HTMLInputElement).value).toBe("14");
    expect((screen.getByLabelText("Bot token") as HTMLInputElement).type).toBe("password");
  });

  it("reports typed values", () => {
    const onChange = vi.fn();
    render(<SchemaFields idPrefix="t" fields={fields} values={{}} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText("Days"), { target: { value: "30" } });
    expect(onChange).toHaveBeenCalledWith("days", 30);
    fireEvent.change(screen.getByLabelText("Days"), { target: { value: "" } });
    expect(onChange).toHaveBeenCalledWith("days", undefined);
    fireEvent.change(screen.getByLabelText("Method"), { target: { value: "POST" } });
    expect(onChange).toHaveBeenCalledWith("method", "POST");
  });

  it("lets people reveal a secret to check what they pasted", () => {
    render(<SchemaFields idPrefix="t" fields={fields} values={{ token: "123:abc" }} onChange={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: "Show" }));
    expect((screen.getByLabelText("Bot token") as HTMLInputElement).type).toBe("text");
  });
});
