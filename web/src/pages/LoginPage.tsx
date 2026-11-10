import React, { useState, useEffect } from 'react';
import { api } from '../api';
import { useAuth } from '../context/AuthContext';
import { useTheme } from '../context/ThemeContext';
import { useTranslation } from '../context/LanguageContext';
import { LanguageSwitcher } from '../components/LanguageSwitcher';
import { Activity, Lock, User, AlertCircle, ArrowRight, Sun, Moon } from 'lucide-react';

export const LoginPage: React.FC = () => {
  const { login } = useAuth();
  const { config, theme, toggleTheme } = useTheme();
  const { t } = useTranslation();

  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [userSelectedProvider, setUserSelectedProvider] = useState(false);
  const [provider, setProvider] = useState<'local' | 'ldap'>(() => {
    if (config?.auth_providers) {
      return config.auth_providers.includes('ldap') ? 'ldap' : 'local';
    }
    return 'ldap';
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [logoError, setLogoError] = useState(false);

  React.useEffect(() => {
    setLogoError(false);
    const params = new URLSearchParams(window.location.search);
    const err = params.get('error');
    if (err) {
      setError(decodeURIComponent(err));
      params.delete('error');
      const cleanSearch = params.toString() ? `?${params.toString()}` : '';
      window.history.replaceState({}, document.title, window.location.pathname + cleanSearch + window.location.hash);
    }
  }, [config?.logo_url]);

  const authProviders = config?.auth_providers || ['local'];
  const hasMultiplePassProviders = authProviders.includes('local') && authProviders.includes('ldap');
  const hasOIDC = authProviders.includes('oidc');

  // Synchronize default provider with server configuration, prioritizing LDAP unless user explicitly changed it
  useEffect(() => {
    if (!userSelectedProvider && config?.auth_providers) {
      if (config.auth_providers.includes('ldap')) {
        setProvider('ldap');
      } else if (config.auth_providers.includes('local')) {
        setProvider('local');
      }
    }
  }, [config?.auth_providers, userSelectedProvider]);

  const handleProviderSelect = (p: 'local' | 'ldap') => {
    setUserSelectedProvider(true);
    setProvider(p);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setLoading(true);

    try {
      const res = await api.login({
        username,
        password,
        provider,
      });
      login(res.token, res.user);
    } catch (err: any) {
      setError(err.message || 'Login failed');
    } finally {
      setLoading(false);
    }
  };

  const handleOIDCLogin = async () => {
    try {
      const res = await api.getAuthProviders();
      if (res.oidc_auth_url) {
        window.location.href = res.oidc_auth_url;
      }
    } catch (err: any) {
      setError('Failed to initiate SSO login: ' + err.message);
    }
  };

  return (
    <div className="min-h-screen flex flex-col justify-center py-12 sm:px-6 lg:px-8 bg-gray-50 dark:bg-slate-950 transition-colors relative">
      {/* Quick theme and language toggle for guests / unauthenticated users */}
      <div className="absolute top-4 right-4 flex items-center space-x-2">
        <LanguageSwitcher />
        <button
          onClick={toggleTheme}
          title={theme === 'dark' ? t('navbar.lightMode') : t('navbar.darkMode')}
          aria-label={theme === 'dark' ? t('navbar.lightMode') : t('navbar.darkMode')}
          className="p-2 text-gray-500 dark:text-slate-400 hover:text-amber-500 dark:hover:text-amber-400 hover:bg-white dark:hover:bg-slate-900 border border-gray-200 dark:border-slate-800 rounded-lg shadow-sm transition-colors focus:outline-none"
        >
          {theme === 'dark' ? (
            <Sun className="w-5 h-5 text-amber-400" />
          ) : (
            <Moon className="w-5 h-5 text-gray-600" />
          )}
        </button>
      </div>

      <div className="sm:mx-auto sm:w-full sm:max-w-md text-center">
        {logoError ? (
          <div
            className="w-14 h-14 rounded-2xl mx-auto flex items-center justify-center text-white shadow-lg mb-4"
            style={{ backgroundColor: 'var(--color-accent, #dc2626)' }}
          >
            <Activity className="w-8 h-8 stroke-[2.5]" />
          </div>
        ) : (
          <img
            src={config?.logo_url || '/api/system/logo'}
            alt="kncc logo"
            onError={() => setLogoError(true)}
            className="w-16 h-16 mx-auto object-contain rounded-2xl shadow-lg mb-4"
          />
        )}
        <h2 className="text-3xl font-extrabold text-gray-900 dark:text-white tracking-tight">
          kncc
        </h2>
        <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
          {config?.app_name || t('login.subtitle')}
        </p>
      </div>

      <div className="mt-8 sm:mx-auto sm:w-full sm:max-w-md px-4 sm:px-0">
        <div className="bg-white dark:bg-slate-900 py-8 px-6 shadow-md rounded-2xl sm:px-10 border border-gray-100 dark:border-slate-800 transition-colors">
          {error && (
            <div className="mb-6 bg-red-50 dark:bg-red-950/40 border-l-4 border-red-500 p-4 rounded-md flex items-start">
              <AlertCircle className="w-5 h-5 text-red-500 mr-2 flex-shrink-0 mt-0.5" />
              <div className="text-sm text-red-700 dark:text-red-300">{error}</div>
            </div>
          )}

          {hasMultiplePassProviders && (
            <div className="mb-6 flex rounded-lg bg-gray-100 dark:bg-slate-800 p-1">
              <button
                type="button"
                onClick={() => handleProviderSelect('ldap')}
                className={`flex-1 py-1.5 text-xs font-semibold rounded-md transition-all ${
                  provider === 'ldap'
                    ? 'bg-white dark:bg-slate-700 shadow text-gray-900 dark:text-white'
                    : 'text-gray-500 dark:text-slate-400 hover:text-gray-900 dark:hover:text-white'
                }`}
              >
                {t('login.ldapAccount')}
              </button>
              <button
                type="button"
                onClick={() => handleProviderSelect('local')}
                className={`flex-1 py-1.5 text-xs font-semibold rounded-md transition-all ${
                  provider === 'local'
                    ? 'bg-white dark:bg-slate-700 shadow text-gray-900 dark:text-white'
                    : 'text-gray-500 dark:text-slate-400 hover:text-gray-900 dark:hover:text-white'
                }`}
              >
                {t('login.localAccount')}
              </button>
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-5">
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">
                {t('login.username')}
              </label>
              <div className="relative rounded-md shadow-sm">
                <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none text-gray-400 dark:text-slate-500">
                  <User className="w-4 h-4" />
                </div>
                <input
                  type="text"
                  required
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder={provider === 'ldap' ? 'username or domain\\user' : 'admin'}
                  className="block w-full pl-10 pr-3 py-2 bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 rounded-lg text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-red-500 focus:border-red-500 transition-colors"
                />
              </div>
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">
                {t('login.password')}
              </label>
              <div className="relative rounded-md shadow-sm">
                <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none text-gray-400 dark:text-slate-500">
                  <Lock className="w-4 h-4" />
                </div>
                <input
                  type="password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="••••••••"
                  className="block w-full pl-10 pr-3 py-2 bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 rounded-lg text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-red-500 focus:border-red-500 transition-colors"
                />
              </div>
            </div>

            <button
              type="submit"
              disabled={loading}
              className="w-full flex justify-center items-center py-2.5 px-4 border border-transparent rounded-lg shadow-sm text-sm font-semibold text-white transition-all disabled:opacity-50"
              style={{
                backgroundColor: 'var(--color-accent, #dc2626)',
              }}
            >
              {loading ? (
                <div className="w-5 h-5 border-2 border-white border-t-transparent rounded-full animate-spin" />
              ) : (
                <>
                  <span>{t('login.signIn')}</span>
                  <ArrowRight className="w-4 h-4 ml-2" />
                </>
              )}
            </button>
          </form>

          {hasOIDC && (
            <div className="mt-6 pt-6 border-t border-gray-200 dark:border-slate-800">
              <button
                type="button"
                onClick={handleOIDCLogin}
                className="w-full flex justify-center items-center py-2 px-4 border border-gray-300 dark:border-slate-700 rounded-lg shadow-sm text-sm font-medium text-gray-700 dark:text-slate-200 bg-white dark:bg-slate-800 hover:bg-gray-50 dark:hover:bg-slate-700 transition-colors"
              >
                {t('login.oidcSignIn')}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
