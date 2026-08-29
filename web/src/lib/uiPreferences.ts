export const showRecentPreferenceKey = "tailpath.ui.v1.showRecent";

export function readShowRecentPreference(
  storage: Storage | undefined,
): boolean {
  if (!storage) return true;
  try {
    const value = storage.getItem(showRecentPreferenceKey);
    return value !== "false";
  } catch {
    return true;
  }
}

export function writeShowRecentPreference(
  storage: Storage | undefined,
  showRecent: boolean,
): void {
  if (!storage) return;
  try {
    storage.setItem(showRecentPreferenceKey, String(showRecent));
  } catch {
    // The UI remains usable when storage is unavailable or blocked.
  }
}
