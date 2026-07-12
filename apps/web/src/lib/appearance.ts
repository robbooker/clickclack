// Appearance preferences: color mode (light / dark / system) and board theme.
//
// Both are personal, device-local prefs stored in localStorage and applied as
// data attributes on <html>; base.css maps them to color-scheme and token
// overrides. An inline script in app.html applies the stored values before
// first paint so a forced mode or non-default board never flashes; this
// module is the single writer afterwards. Keep the storage keys and attribute
// names in sync with that script.

export type ColorMode = "light" | "dark" | "system";
export type BoardTheme = "signal" | "ember" | "moss" | "iris";

export const COLOR_MODE_STORAGE_KEY = "clickclack:color-mode:v1";
export const BOARD_THEME_STORAGE_KEY = "clickclack:board-theme:v1";

export const DEFAULT_COLOR_MODE: ColorMode = "system";
export const DEFAULT_BOARD_THEME: BoardTheme = "signal";

export const COLOR_MODES: { id: ColorMode; label: string }[] = [
  { id: "light", label: "Light" },
  { id: "dark", label: "Dark" },
  { id: "system", label: "System" },
];

export const BOARD_THEMES: { id: BoardTheme; label: string; blurb: string }[] = [
  { id: "signal", label: "Signal", blurb: "Porcelain board, electric cyan" },
  { id: "ember", label: "Ember", blurb: "Warm paper, ember coral" },
  { id: "moss", label: "Moss", blurb: "Sage plate, verdant green" },
  { id: "iris", label: "Iris", blurb: "Violet plate, twilight iris" },
];

function isColorMode(value: string | null): value is ColorMode {
  return value === "light" || value === "dark" || value === "system";
}

function isBoardTheme(value: string | null): value is BoardTheme {
  return BOARD_THEMES.some((board) => board.id === value);
}

export function loadColorMode(): ColorMode {
  try {
    const stored = window.localStorage.getItem(COLOR_MODE_STORAGE_KEY);
    return isColorMode(stored) ? stored : DEFAULT_COLOR_MODE;
  } catch {
    return DEFAULT_COLOR_MODE;
  }
}

export function loadBoardTheme(): BoardTheme {
  try {
    const stored = window.localStorage.getItem(BOARD_THEME_STORAGE_KEY);
    return isBoardTheme(stored) ? stored : DEFAULT_BOARD_THEME;
  } catch {
    return DEFAULT_BOARD_THEME;
  }
}

export function applyColorMode(mode: ColorMode) {
  try {
    if (mode === "system") document.documentElement.removeAttribute("data-color-mode");
    else document.documentElement.setAttribute("data-color-mode", mode);
  } catch {
    // Non-DOM context (SSR/tests); the stored pref still applies on mount.
  }
}

export function applyBoardTheme(board: BoardTheme) {
  try {
    if (board === DEFAULT_BOARD_THEME) document.documentElement.removeAttribute("data-board");
    else document.documentElement.setAttribute("data-board", board);
  } catch {
    // Non-DOM context (SSR/tests); the stored pref still applies on mount.
  }
}

export function setColorMode(mode: ColorMode) {
  applyColorMode(mode);
  try {
    if (mode === DEFAULT_COLOR_MODE) window.localStorage.removeItem(COLOR_MODE_STORAGE_KEY);
    else window.localStorage.setItem(COLOR_MODE_STORAGE_KEY, mode);
  } catch {
    // Ignore unavailable storage; the in-memory pref still applies this session.
  }
}

export function setBoardTheme(board: BoardTheme) {
  applyBoardTheme(board);
  try {
    if (board === DEFAULT_BOARD_THEME) window.localStorage.removeItem(BOARD_THEME_STORAGE_KEY);
    else window.localStorage.setItem(BOARD_THEME_STORAGE_KEY, board);
  } catch {
    // Ignore unavailable storage; the in-memory pref still applies this session.
  }
}

// Re-apply the stored prefs (mount-time belt to the app.html suspenders, and
// the recovery path when the boot script could not run).
export function initAppearance() {
  applyColorMode(loadColorMode());
  applyBoardTheme(loadBoardTheme());
}
