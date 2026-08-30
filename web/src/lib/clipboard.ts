export interface ClipboardEnvironment {
  clipboard?: Pick<Clipboard, "writeText">;
  document: Document;
}

export async function copyText(
  value: string,
  environment: ClipboardEnvironment = {
    clipboard:
      typeof navigator === "undefined" ? undefined : navigator.clipboard,
    document,
  },
): Promise<boolean> {
  if (environment.clipboard) {
    try {
      await environment.clipboard.writeText(value);
      return true;
    } catch {
      // HTTP Tailnet pages commonly expose Clipboard API but reject writes.
    }
  }
  return fallbackCopy(value, environment.document);
}

function fallbackCopy(value: string, targetDocument: Document): boolean {
  const active = targetDocument.activeElement;
  const textarea = targetDocument.createElement("textarea");
  textarea.value = value;
  textarea.readOnly = true;
  textarea.setAttribute("aria-hidden", "true");
  textarea.style.position = "fixed";
  textarea.style.inset = "0 auto auto -9999px";
  textarea.style.opacity = "0";
  targetDocument.body.append(textarea);
  textarea.select();
  textarea.setSelectionRange(0, value.length);
  let copied = false;
  try {
    copied = targetDocument.execCommand("copy");
  } catch {
    copied = false;
  }
  textarea.remove();
  if (typeof HTMLElement !== "undefined" && active instanceof HTMLElement) {
    active.focus({ preventScroll: true });
  }
  return copied;
}
