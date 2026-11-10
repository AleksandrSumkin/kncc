import React from 'react';
import { useAuth } from '../context/AuthContext';
import { useTheme } from '../context/ThemeContext';
import { useTranslation } from '../context/LanguageContext';
import { LanguageSwitcher } from './LanguageSwitcher';
import { Activity, PlusCircle, History, Shield, LogOut, User as UserIcon, Sun, Moon } from 'lucide-react';

interface NavbarProps {
  currentView: string;
  onNavigate: (view: string) => void;
}

export const Navbar: React.FC<NavbarProps> = ({ currentView, onNavigate }) => {
  const { user, logout } = useAuth();
  const { config, theme, toggleTheme } = useTheme();
  const { t } = useTranslation();
  const [logoError, setLogoError] = React.useState(false);

  React.useEffect(() => {
    setLogoError(false);
  }, [config?.logo_url]);

  return (
    <header className="bg-white dark:bg-slate-900 border-b border-gray-200 dark:border-slate-800 sticky top-0 z-30 shadow-sm transition-colors">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex justify-between h-16">
          <div className="flex items-center space-x-8">
            <button
              onClick={() => onNavigate('new')}
              className="flex items-center space-x-2.5 text-left focus:outline-none group"
            >
              {logoError ? (
                <div
                  className="w-9 h-9 rounded-lg flex items-center justify-center text-white shadow transition-transform group-hover:scale-105 flex-shrink-0"
                  style={{ backgroundColor: 'var(--color-accent, #dc2626)' }}
                >
                  <Activity className="w-5 h-5 stroke-[2.5]" />
                </div>
              ) : (
                <img
                  src={config?.logo_url || '/api/system/logo'}
                  alt="kncc logo"
                  onError={() => setLogoError(true)}
                  className="w-9 h-9 object-contain rounded-lg shadow-sm transition-transform group-hover:scale-105 flex-shrink-0"
                />
              )}
              <div>
                <span className="font-bold text-lg text-gray-900 dark:text-white tracking-tight block leading-none">
                  kncc
                </span>
                <span className="text-xs text-gray-500 dark:text-slate-400 font-medium hidden sm:inline">
                  {config?.app_name || t('navbar.appNameFallback')}
                </span>
              </div>
            </button>

            {user && (
              <nav className="flex space-x-2 sm:space-x-4">
                <button
                  onClick={() => onNavigate('new')}
                  className={`inline-flex items-center px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                    currentView === 'new'
                      ? 'bg-gray-100 dark:bg-slate-800 text-gray-900 dark:text-white font-semibold'
                      : 'text-gray-600 dark:text-slate-300 hover:text-gray-900 dark:hover:text-white hover:bg-gray-50 dark:hover:bg-slate-800/60'
                  }`}
                >
                  <PlusCircle className="w-4 h-4 mr-1.5" />
                  {t('navbar.newCheck')}
                </button>
                <button
                  onClick={() => onNavigate('history')}
                  className={`inline-flex items-center px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                    currentView === 'history'
                      ? 'bg-gray-100 dark:bg-slate-800 text-gray-900 dark:text-white font-semibold'
                      : 'text-gray-600 dark:text-slate-300 hover:text-gray-900 dark:hover:text-white hover:bg-gray-50 dark:hover:bg-slate-800/60'
                  }`}
                >
                  <History className="w-4 h-4 mr-1.5" />
                  {t('navbar.history')}
                </button>
                {user.role === 'admin' && (
                  <button
                    onClick={() => onNavigate('admin')}
                    className={`inline-flex items-center px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                      currentView === 'admin'
                        ? 'bg-gray-100 dark:bg-slate-800 text-gray-900 dark:text-white font-semibold'
                        : 'text-gray-600 dark:text-slate-300 hover:text-gray-900 dark:hover:text-white hover:bg-gray-50 dark:hover:bg-slate-800/60'
                    }`}
                  >
                    <Shield className="w-4 h-4 mr-1.5" />
                    {t('navbar.admin')}
                  </button>
                )}
              </nav>
            )}
          </div>

          <div className="flex items-center space-x-2 sm:space-x-3">
            {/* Language switcher */}
            <LanguageSwitcher />

            {/* Quick theme toggle on-the-fly */}
            <button
              onClick={toggleTheme}
              title={theme === 'dark' ? t('navbar.lightMode') : t('navbar.darkMode')}
              aria-label={theme === 'dark' ? t('navbar.lightMode') : t('navbar.darkMode')}
              className="p-2 text-gray-500 dark:text-slate-400 hover:text-amber-500 dark:hover:text-amber-400 hover:bg-gray-100 dark:hover:bg-slate-800 rounded-lg transition-colors focus:outline-none"
            >
              {theme === 'dark' ? (
                <Sun className="w-5 h-5 text-amber-400" />
              ) : (
                <Moon className="w-5 h-5 text-gray-600" />
              )}
            </button>

            {user ? (
              <div className="flex items-center space-x-2 sm:space-x-3">
                <div className="flex items-center space-x-2 text-sm text-gray-700 dark:text-slate-200 bg-gray-50 dark:bg-slate-800 px-3 py-1.5 rounded-full border border-gray-200 dark:border-slate-700">
                  <UserIcon className="w-3.5 h-3.5 text-gray-500 dark:text-slate-400" />
                  <span className="font-medium text-gray-900 dark:text-white">{user.username}</span>
                  {user.role === 'admin' && (
                    <span
                      className="px-1.5 py-0.5 text-xs font-bold rounded text-white"
                      style={{ backgroundColor: 'var(--color-accent, #dc2626)' }}
                    >
                      {t('navbar.adminBadge')}
                    </span>
                  )}
                </div>

                <button
                  onClick={logout}
                  title={t('navbar.signOut')}
                  className="p-2 text-gray-500 dark:text-slate-400 hover:text-red-600 dark:hover:text-red-400 hover:bg-gray-100 dark:hover:bg-slate-800 rounded-md transition-colors"
                >
                  <LogOut className="w-4 h-4" />
                </button>
              </div>
            ) : null}
          </div>
        </div>
      </div>
    </header>
  );
};
