export type ThemeMode = 'dark' | 'light'

export const THEME_STORAGE_KEY = 'icloud-relay-theme'

export function isThemeMode(value: string | null): value is ThemeMode {
  return value === 'dark' || value === 'light'
}

export function getInitialTheme(): ThemeMode {
  try {
    const stored = window.localStorage.getItem(THEME_STORAGE_KEY)
    return isThemeMode(stored) ? stored : 'dark'
  } catch {
    return 'dark'
  }
}

export function applyTheme(theme: ThemeMode) {
  document.documentElement.dataset.theme = theme
  document.documentElement.style.colorScheme = theme
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', theme === 'light' ? '#f2f6f4' : '#080b0d')
}

export function persistTheme(theme: ThemeMode) {
  try {
    window.localStorage.setItem(THEME_STORAGE_KEY, theme)
  } catch {
    // Theme still applies for the current session when storage is unavailable.
  }
  applyTheme(theme)
}
