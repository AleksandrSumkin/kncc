import React, { createContext, useContext, useEffect, useState, useCallback } from 'react';
import { api } from '../api';
import { SystemConfig } from '../types';

export type ThemeMode = 'light' | 'dark' | 'system';
export type ResolvedTheme = 'light' | 'dark';

interface ThemeContextType {
  config: SystemConfig | null;
  loading: boolean;
  theme: ResolvedTheme;
  themePreference: ThemeMode;
  setThemePreference: (pref: ThemeMode) => void;
  toggleTheme: () => void;
  reloadConfig: () => Promise<void>;
}

const ThemeContext = createContext<ThemeContextType>({
  config: null,
  loading: true,
  theme: 'light',
  themePreference: 'light',
  setThemePreference: () => {},
  toggleTheme: () => {},
  reloadConfig: async () => {},
});

const THEME_STORAGE_KEY = 'kncc_theme_preference';

export const ThemeProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [config, setConfig] = useState<SystemConfig | null>(null);
  const [loading, setLoading] = useState(true);

  // Preference can be 'light', 'dark', 'system', or null (unspecified -> fallback to config default)
  const [themePreference, setThemePrefState] = useState<ThemeMode>(() => {
    try {
      const saved = localStorage.getItem(THEME_STORAGE_KEY) as ThemeMode;
      if (saved === 'light' || saved === 'dark' || saved === 'system') {
        return saved;
      }
    } catch {}
    return 'system'; // initial default before config loads
  });

  const [hasExplicitUserChoice, setHasExplicitUserChoice] = useState<boolean>(() => {
    try {
      return localStorage.getItem(THEME_STORAGE_KEY) !== null;
    } catch {
      return false;
    }
  });

  const [resolvedTheme, setResolvedTheme] = useState<ResolvedTheme>('light');

  const fetchConfig = useCallback(async () => {
    try {
      const cfg = await api.getSystemConfig();
      setConfig(cfg);
      if (cfg.app_name) {
        document.title = cfg.app_name;
      }
      if (cfg.favicon_url) {
        let link = document.querySelector<HTMLLinkElement>("link[rel~='icon']");
        if (!link) {
          link = document.createElement('link');
          link.rel = 'icon';
          document.head.appendChild(link);
        }
        link.href = cfg.favicon_url;
      }

      const root = document.documentElement;
      if (cfg.primary_color) {
        root.style.setProperty('--color-primary', cfg.primary_color);
      }
      if (cfg.accent_color) {
        root.style.setProperty('--color-accent', cfg.accent_color);
        root.style.setProperty('--color-accent-hover', adjustColorBrightness(cfg.accent_color, -20));
        root.style.setProperty('--color-accent-light', hexToRGBA(cfg.accent_color, 0.08));
      }
      return cfg;
    } catch (err) {
      console.error('Failed to load system config:', err);
      return null;
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  // Compute effective theme based on user preference, system preference, and server default theme
  useEffect(() => {
    const computeTheme = (): ResolvedTheme => {
      // 1. If user explicitly chose a preference
      if (hasExplicitUserChoice) {
        if (themePreference === 'dark') return 'dark';
        if (themePreference === 'light') return 'light';
        // 'system'
        return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
      }

      // 2. If no explicit user choice, use system config default_theme if available
      if (config?.default_theme === 'dark' || config?.default_theme === 'light') {
        return config.default_theme;
      }

      // 3. Fallback to system OS preference
      return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    };

    const active = computeTheme();
    setResolvedTheme(active);

    const root = document.documentElement;
    if (active === 'dark') {
      root.classList.add('dark');
    } else {
      root.classList.remove('dark');
    }
  }, [themePreference, hasExplicitUserChoice, config?.default_theme]);

  // Listen to OS theme changes if user preference is 'system' or default
  useEffect(() => {
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    const handleChange = () => {
      if (!hasExplicitUserChoice || themePreference === 'system') {
        const active = mediaQuery.matches ? 'dark' : 'light';
        setResolvedTheme(active);
        if (active === 'dark') {
          document.documentElement.classList.add('dark');
        } else {
          document.documentElement.classList.remove('dark');
        }
      }
    };

    mediaQuery.addEventListener('change', handleChange);
    return () => mediaQuery.removeEventListener('change', handleChange);
  }, [hasExplicitUserChoice, themePreference]);

  const setThemePreference = useCallback((pref: ThemeMode) => {
    setThemePrefState(pref);
    setHasExplicitUserChoice(true);
    try {
      localStorage.setItem(THEME_STORAGE_KEY, pref);
    } catch {}
  }, []);

  const toggleTheme = useCallback(() => {
    const nextTheme: ResolvedTheme = resolvedTheme === 'dark' ? 'light' : 'dark';
    setThemePrefState(nextTheme);
    setHasExplicitUserChoice(true);
    try {
      localStorage.setItem(THEME_STORAGE_KEY, nextTheme);
    } catch {}
  }, [resolvedTheme]);

  const reloadConfig = useCallback(async () => {
    await fetchConfig();
  }, [fetchConfig]);

  return (
    <ThemeContext.Provider
      value={{
        config,
        loading,
        theme: resolvedTheme,
        themePreference,
        setThemePreference,
        toggleTheme,
        reloadConfig,
      }}
    >
      {children}
    </ThemeContext.Provider>
  );
};

export const useTheme = () => useContext(ThemeContext);

function adjustColorBrightness(hex: string, percent: number): string {
  let num = parseInt(hex.replace('#', ''), 16);
  if (isNaN(num)) return hex;
  let amt = Math.round(2.55 * percent);
  let R = (num >> 16) + amt;
  let G = ((num >> 8) & 0x00ff) + amt;
  let B = (num & 0x0000ff) + amt;
  return `#${(
    0x1000000 +
    (R < 255 ? (R < 1 ? 0 : R) : 255) * 0x10000 +
    (G < 255 ? (G < 1 ? 0 : G) : 255) * 0x100 +
    (B < 255 ? (B < 1 ? 0 : B) : 255)
  )
    .toString(16)
    .slice(1)}`;
}

function hexToRGBA(hex: string, alpha: number): string {
  let c = hex.replace('#', '');
  if (c.length === 3) {
    c = c.split('').map((char) => char + char).join('');
  }
  const num = parseInt(c, 16);
  if (isNaN(num)) return `rgba(220, 38, 38, ${alpha})`;
  return `rgba(${(num >> 16) & 255}, ${(num >> 8) & 255}, ${num & 255}, ${alpha})`;
}
