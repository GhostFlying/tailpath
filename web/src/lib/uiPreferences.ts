export const showRecentPreferenceKey = "tailpath.ui.v1.showRecent";
export const showControlTrafficPreferenceKey =
  "tailpath.ui.v1.showControlTraffic";

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

export function readShowControlTrafficPreference(
  storage: Storage | undefined,
): boolean {
  if (!storage) return false;
  try {
    return storage.getItem(showControlTrafficPreferenceKey) === "true";
  } catch {
    return false;
  }
}

export function writeShowControlTrafficPreference(
  storage: Storage | undefined,
  showControlTraffic: boolean,
): void {
  if (!storage) return;
  try {
    storage.setItem(
      showControlTrafficPreferenceKey,
      String(showControlTraffic),
    );
  } catch {
    // The UI remains usable when storage is unavailable or blocked.
  }
}
