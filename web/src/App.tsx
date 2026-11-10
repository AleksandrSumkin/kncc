import React, { useState, useEffect } from 'react';
import { useAuth } from './context/AuthContext';
import { useTheme } from './context/ThemeContext';
import { useTranslation } from './context/LanguageContext';
import { Navbar } from './components/Navbar';
import { LoginPage } from './pages/LoginPage';
import { NewCheckPage, PrefillData } from './pages/NewCheckPage';
import { ReportPage } from './pages/ReportPage';
import { HistoryPage } from './pages/HistoryPage';
import { AdminPage } from './pages/AdminPage';

export const App: React.FC = () => {
  const { user, loading } = useAuth();
  const { config } = useTheme();
  const { t } = useTranslation();
  const [currentView, setCurrentView] = useState<'new' | 'report' | 'history' | 'admin'>('new');
  const [activeReportId, setActiveReportId] = useState<string | null>(null);
  const [prefillCheckData, setPrefillCheckData] = useState<PrefillData | null>(() => {
    try {
      const saved = sessionStorage.getItem('kncc_prefill_check');
      return saved ? JSON.parse(saved) : null;
    } catch {
      return null;
    }
  });

  // Sync with browser URL / hash for shareable direct links!
  useEffect(() => {
    const parseHash = () => {
      const hash = window.location.hash.replace('#/', '').replace('#', '');
      if (hash.startsWith('report/')) {
        const id = hash.replace('report/', '');
        if (id) {
          setActiveReportId(id);
          setCurrentView('report');
          return;
        }
      } else if (hash === 'history') {
        setCurrentView('history');
      } else if (hash === 'admin') {
        if (user && user.role !== 'admin') {
          window.location.hash = '/';
          setCurrentView('new');
        } else {
          setCurrentView('admin');
        }
      } else {
        setCurrentView('new');
      }
    };

    parseHash();
    window.addEventListener('hashchange', parseHash);
    return () => window.removeEventListener('hashchange', parseHash);
  }, [user]);

  // Route guard: ensure non-admin users cannot access admin panel under any circumstances
  useEffect(() => {
    if (user && user.role !== 'admin') {
      const hash = window.location.hash.replace('#/', '').replace('#', '');
      if (currentView === 'admin' || hash === 'admin') {
        window.location.hash = '/';
        setCurrentView('new');
      }
    }
  }, [user, currentView]);

  const navigateTo = (view: string, reportId?: string, clearPrefill = false) => {
    if (clearPrefill) {
      try {
        sessionStorage.removeItem('kncc_prefill_check');
      } catch {}
      setPrefillCheckData(null);
    }

    if (view === 'report' && reportId) {
      window.location.hash = `/report/${reportId}`;
      setActiveReportId(reportId);
      setCurrentView('report');
    } else if (view === 'history') {
      window.location.hash = '/history';
      setCurrentView('history');
    } else if (view === 'admin') {
      if (user && user.role !== 'admin') {
        window.location.hash = '/';
        setCurrentView('new');
        return;
      }
      window.location.hash = '/admin';
      setCurrentView('admin');
    } else {
      window.location.hash = '/';
      setCurrentView('new');
    }
  };

  const handleBackToEdit = (data: PrefillData) => {
    try {
      sessionStorage.setItem('kncc_prefill_check', JSON.stringify(data));
    } catch {}
    setPrefillCheckData(data);
    window.location.hash = '/';
    setCurrentView('new');
  };

  const handleClearPrefill = () => {
    try {
      sessionStorage.removeItem('kncc_prefill_check');
    } catch {}
    setPrefillCheckData(null);
  };

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50 dark:bg-slate-950">
        <div className="w-8 h-8 border-4 border-red-600 border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!user) {
    return <LoginPage />;
  }

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-slate-950 text-gray-900 dark:text-slate-100 flex flex-col transition-colors">
      <Navbar currentView={currentView} onNavigate={(v) => navigateTo(v, undefined, v === 'new')} />

      <main className="flex-1">
        {currentView === 'new' && (
          <NewCheckPage
            initialData={prefillCheckData}
            onClearPrefill={handleClearPrefill}
            onCheckCreated={(reportId) => {
              handleClearPrefill();
              navigateTo('report', reportId);
            }}
          />
        )}
        {currentView === 'report' && activeReportId && (
          <ReportPage
            reportId={activeReportId}
            onRerunCreated={(newReportId) => navigateTo('report', newReportId)}
            onBackToEdit={handleBackToEdit}
            onNavigateHome={() => navigateTo('new')}
          />
        )}
        {currentView === 'history' && (
          <HistoryPage
            onSelectReport={(reportId) => navigateTo('report', reportId)}
          />
        )}
        {currentView === 'admin' && user?.role === 'admin' && <AdminPage />}
      </main>

      <footer className="py-4 border-t border-gray-200 dark:border-slate-800 text-center text-xs text-gray-400 dark:text-slate-500 bg-white dark:bg-slate-900 transition-colors">
        kncc{config?.version ? (config.version === 'dev' ? ' (dev)' : ` v${config.version.replace(/^v/, '')}`) : ''} • {t('footer.description')} • {t('footer.openSource')}
      </footer>
    </div>
  );
};
