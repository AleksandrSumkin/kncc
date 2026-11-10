import { en } from './locales/en';
import { ru } from './locales/ru';
import { SupportedLanguage, TranslationSchema } from './types';

export * from './types';

export interface LanguageInfo {
  code: SupportedLanguage;
  label: string;
  shortLabel: string;
}

export const SUPPORTED_LANGUAGES: LanguageInfo[] = [
  { code: 'en', label: 'English', shortLabel: 'EN' },
  { code: 'ru', label: 'Русский', shortLabel: 'RU' },
];

export const translations: Record<SupportedLanguage, TranslationSchema> = {
  en,
  ru,
};

export function getTranslation(
  lang: SupportedLanguage,
  path: string,
  params?: Record<string, string | number>
): string {
  const dict = translations[lang] || translations.en;
  const parts = path.split('.');

  let current: any = dict;
  for (const part of parts) {
    if (current && typeof current === 'object' && part in current) {
      current = current[part];
    } else {
      // Fallback to English dictionary if key is missing in target language
      let fallbackCurrent: any = translations.en;
      for (const fbPart of parts) {
        if (fallbackCurrent && typeof fallbackCurrent === 'object' && fbPart in fallbackCurrent) {
          fallbackCurrent = fallbackCurrent[fbPart];
        } else {
          return path;
        }
      }
      current = fallbackCurrent;
      break;
    }
  }

  if (typeof current !== 'string') {
    return path;
  }

  if (params) {
    let result = current;
    for (const [key, value] of Object.entries(params)) {
      result = result.replace(new RegExp(`\\{${key}\\}`, 'g'), String(value));
    }
    return result;
  }

  return current;
}
