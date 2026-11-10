import React, { createContext, useContext, useEffect, useState, useCallback } from 'react';
import { SupportedLanguage, LanguageInfo, SUPPORTED_LANGUAGES, getTranslation } from '../i18n';
import { useTheme } from './ThemeContext';

interface LanguageContextType {
  language: SupportedLanguage;
  setLanguage: (lang: SupportedLanguage) => void;
  toggleLanguage: () => void;
  t: (path: string, params?: Record<string, string | number>) => string;
  supportedLanguages: LanguageInfo[];
}

const LanguageContext = createContext<LanguageContextType>({
  language: 'en',
  setLanguage: () => {},
  toggleLanguage: () => {},
  t: (path: string) => path,
  supportedLanguages: SUPPORTED_LANGUAGES,
});

const LANGUAGE_STORAGE_KEY = 'kncc_language';

export const LanguageProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { config } = useTheme();

  const [hasExplicitUserChoice, setHasExplicitUserChoice] = useState<boolean>(() => {
    try {
      return localStorage.getItem(LANGUAGE_STORAGE_KEY) !== null;
    } catch {
      return false;
    }
  });

  const [language, setLanguageState] = useState<SupportedLanguage>(() => {
    try {
      const saved = localStorage.getItem(LANGUAGE_STORAGE_KEY) as SupportedLanguage;
      if (saved === 'en' || saved === 'ru') {
        return saved;
      }
    } catch {}

    // Check browser language
    if (typeof navigator !== 'undefined' && navigator.language) {
      if (navigator.language.toLowerCase().startsWith('ru')) {
        return 'ru';
      }
    }
    return 'en';
  });

  // When config loads and user has not explicitly chosen a language in localStorage,
  // adopt server's default_language if provided!
  useEffect(() => {
    if (!hasExplicitUserChoice && config?.default_language) {
      const norm = config.default_language.toLowerCase().trim();
      if (norm === 'ru' || norm === 'en') {
        setLanguageState(norm as SupportedLanguage);
      }
    }
  }, [config?.default_language, hasExplicitUserChoice]);

  // Update HTML lang attribute
  useEffect(() => {
    if (typeof document !== 'undefined') {
      document.documentElement.lang = language;
    }
  }, [language]);

  const setLanguage = useCallback((lang: SupportedLanguage) => {
    setLanguageState(lang);
    setHasExplicitUserChoice(true);
    try {
      localStorage.setItem(LANGUAGE_STORAGE_KEY, lang);
    } catch {}
  }, []);

  const toggleLanguage = useCallback(() => {
    const nextLang: SupportedLanguage = language === 'ru' ? 'en' : 'ru';
    setLanguage(nextLang);
  }, [language, setLanguage]);

  const t = useCallback(
    (path: string, params?: Record<string, string | number>) => {
      return getTranslation(language, path, params);
    },
    [language]
  );

  return (
    <LanguageContext.Provider
      value={{
        language,
        setLanguage,
        toggleLanguage,
        t,
        supportedLanguages: SUPPORTED_LANGUAGES,
      }}
    >
      {children}
    </LanguageContext.Provider>
  );
};

export const useTranslation = () => useContext(LanguageContext);
