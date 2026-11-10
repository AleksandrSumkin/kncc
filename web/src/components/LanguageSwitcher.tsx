import React, { useState, useRef, useEffect } from 'react';
import { Languages, Check } from 'lucide-react';
import { useTranslation } from '../context/LanguageContext';

export const LanguageSwitcher: React.FC = () => {
  const { language, setLanguage, supportedLanguages, t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const currentLang = supportedLanguages.find((l) => l.code === language) || supportedLanguages[0];

  return (
    <div className="relative inline-block text-left" ref={dropdownRef}>
      <button
        type="button"
        onClick={() => setIsOpen(!isOpen)}
        title={t('navbar.switchLanguage')}
        aria-label={t('navbar.switchLanguage')}
        className={`inline-flex items-center space-x-1.5 px-2.5 py-1.5 rounded-lg text-xs font-semibold border transition-colors focus:outline-none ${
          isOpen
            ? 'bg-gray-100 dark:bg-slate-800 text-gray-900 dark:text-white border-gray-300 dark:border-slate-600'
            : 'text-gray-600 dark:text-slate-300 border-gray-200 dark:border-slate-700 hover:text-gray-900 dark:hover:text-white hover:bg-gray-50 dark:hover:bg-slate-800'
        }`}
      >
        <Languages className="w-4 h-4 text-gray-500 dark:text-slate-400" />
        <span className="uppercase tracking-wider">{currentLang.shortLabel}</span>
      </button>

      {isOpen && (
        <div className="origin-top-right absolute right-0 mt-2 w-36 rounded-lg shadow-lg bg-white dark:bg-slate-900 ring-1 ring-black ring-opacity-5 border border-gray-200 dark:border-slate-800 py-1 z-50 focus:outline-none">
          {supportedLanguages.map((lang) => {
            const isSelected = lang.code === language;
            return (
              <button
                key={lang.code}
                type="button"
                onClick={() => {
                  setLanguage(lang.code);
                  setIsOpen(false);
                }}
                className={`w-full text-left px-3.5 py-2 text-xs flex items-center justify-between transition-colors ${
                  isSelected
                    ? 'font-bold text-gray-900 dark:text-white bg-gray-50 dark:bg-slate-800/80'
                    : 'text-gray-700 dark:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-800'
                }`}
              >
                <div className="flex items-center space-x-2">
                  <span className="font-mono uppercase text-gray-400 dark:text-slate-500 text-[10px]">
                    {lang.shortLabel}
                  </span>
                  <span>{lang.label}</span>
                </div>
                {isSelected && <Check className="w-3.5 h-3.5 text-red-600 dark:text-red-400 stroke-[2.5]" />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
};
