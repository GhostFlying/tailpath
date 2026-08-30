import { afterEach, describe, expect, it, vi } from "vitest";
import { copyText } from "./clipboard";

afterEach(() => vi.restoreAllMocks());

describe("copyText", () => {
  it("uses Clipboard API when available", async () => {
    const writeText = vi.fn(async () => undefined);
    const targetDocument = fakeDocument(true);
    await expect(
      copyText("100.64.0.1", {
        clipboard: { writeText },
        document: targetDocument.value,
      }),
    ).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith("100.64.0.1");
  });

  it("falls back to a temporary textarea after a rejected write", async () => {
    const writeText = vi.fn(async () => Promise.reject(new Error("denied")));
    const targetDocument = fakeDocument(true);

    await expect(
      copyText("fd7a:115c:a1e0::1", {
        clipboard: { writeText },
        document: targetDocument.value,
      }),
    ).resolves.toBe(true);
    expect(targetDocument.execCommand).toHaveBeenCalledWith("copy");
    expect(targetDocument.textarea.remove).toHaveBeenCalledOnce();
  });

  it("reports a failed fallback without leaving a textarea", async () => {
    const targetDocument = fakeDocument(false);
    await expect(
      copyText("value", {
        clipboard: undefined,
        document: targetDocument.value,
      }),
    ).resolves.toBe(false);
    expect(targetDocument.textarea.remove).toHaveBeenCalledOnce();
  });
});

function fakeDocument(copyResult: boolean) {
  const textarea = {
    value: "",
    readOnly: false,
    style: {},
    setAttribute: vi.fn(),
    select: vi.fn(),
    setSelectionRange: vi.fn(),
    remove: vi.fn(),
  };
  const execCommand = vi.fn(() => copyResult);
  const value = {
    activeElement: null,
    createElement: vi.fn(() => textarea),
    body: { append: vi.fn() },
    execCommand,
  } as unknown as Document;
  return { value, textarea, execCommand };
}
