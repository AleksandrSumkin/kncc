import React, { useEffect, useState, useMemo } from 'react';
import { api } from '../api';
import { useTheme } from '../context/ThemeContext';
import { User, RBACReport, UserStats, UserStatsResponse } from '../types';
import {
  Shield,
  UserPlus,
  Check,
  AlertCircle,
  Settings,
  Layers,
  Lock,
  RefreshCw,
  Copy,
  Search,
  KeyRound,
  Users,
  CheckCircle2,
  Activity,
  Terminal,
  Sun,
  Moon,
  Sliders,
  Palette,
  BarChart3,
  ArrowUpDown,
  ArrowUp,
  ArrowDown,
  Calendar,
  Clock,
  TrendingUp,
  FileSpreadsheet,
  Upload,
  Image as ImageIcon,
  RotateCcw,
  Globe,
} from 'lucide-react';

export const AdminPage: React.FC = () => {
  const { config, reloadConfig } = useTheme();

  // Tab state
  const [activeTab, setActiveTab] = useState<'users' | 'stats' | 'rbac' | 'settings'>('users');

  // Stats state
  const [statsData, setStatsData] = useState<UserStatsResponse | null>(null);
  const [statsLoading, setStatsLoading] = useState(false);
  const [statsError, setStatsError] = useState<string | null>(null);
  const [statsSearch, setStatsSearch] = useState('');
  const [sortField, setSortField] = useState<keyof UserStats>('total_count');
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('desc');
  const [statsCopiedNotice, setStatsCopiedNotice] = useState(false);

  // Users state
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAddModal, setShowAddModal] = useState(false);

  // Form state
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [role, setRole] = useState<'user' | 'admin'>('user');
  const [email, setEmail] = useState('');
  const [createLoading, setCreateLoading] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [createSuccess, setCreateSuccess] = useState(false);

  // RBAC state
  const [rbacReport, setRbacReport] = useState<RBACReport | null>(null);
  const [rbacLoading, setRbacLoading] = useState(false);
  const [rbacError, setRbacError] = useState<string | null>(null);
  const [rbacSearch, setRbacSearch] = useState('');
  const [rbacVerbFilter, setRbacVerbFilter] = useState<string>('all');
  const [rbacScopeFilter, setRbacScopeFilter] = useState<string>('all');
  const [copiedNotice, setCopiedNotice] = useState<string | null>(null);

  // Settings state
  const [defaultThemeSetting, setDefaultThemeSetting] = useState<'light' | 'dark'>('light');
  const [settingsSaving, setSettingsSaving] = useState(false);
  const [settingsSuccess, setSettingsSuccess] = useState<string | null>(null);
  const [settingsError, setSettingsError] = useState<string | null>(null);

  // Logo settings state
  const [selectedLogoFile, setSelectedLogoFile] = useState<File | null>(null);
  const [logoPreviewUrl, setLogoPreviewUrl] = useState<string | null>(null);
  const [logoUploading, setLogoUploading] = useState(false);
  const [logoResetting, setLogoResetting] = useState(false);
  const [logoSuccess, setLogoSuccess] = useState<string | null>(null);
  const [logoError, setLogoError] = useState<string | null>(null);
  const [logoPreviewTheme, setLogoPreviewTheme] = useState<'light' | 'dark'>('light');
  const [isDraggingLogo, setIsDraggingLogo] = useState(false);
  const [previewError, setPreviewError] = useState(false);

  const handleLogoFileSelect = (file: File) => {
    setLogoError(null);
    setLogoSuccess(null);
    setPreviewError(false);

    // Validate size: max 1MB
    if (file.size > 1024 * 1024) {
      setLogoError('Размер файла превышает 1 МБ. Пожалуйста, оптимизируйте изображение.');
      return;
    }

    const isSvg = file.name.toLowerCase().endsWith('.svg') || file.type === 'image/svg+xml';
    const validTypes = ['image/svg+xml', 'image/png', 'image/jpeg', 'image/webp'];
    if (!validTypes.includes(file.type) && !isSvg) {
      setLogoError('Неподдерживаемый формат. Разрешены только SVG, PNG, JPEG или WebP.');
      return;
    }

    setSelectedLogoFile(file);

    // Read file as Data URL to ensure universal rendering for SVG and images
    const reader = new FileReader();
    reader.onload = (e) => {
      let result = e.target?.result as string;
      if (isSvg && result) {
        // Normalize MIME header in data URL for SVG to guarantee rendering
        const commaIdx = result.indexOf(',');
        if (commaIdx !== -1) {
          result = 'data:image/svg+xml;base64,' + result.substring(commaIdx + 1);
        }
      }
      setLogoPreviewUrl(result);
    };
    reader.onerror = () => {
      setPreviewError(true);
      setLogoError('Не удалось прочитать выбранный файл.');
    };
    reader.readAsDataURL(file);
  };

  const handleUploadLogo = async () => {
    if (!selectedLogoFile) return;
    setLogoUploading(true);
    setLogoError(null);
    setLogoSuccess(null);
    try {
      const res = await api.uploadLogo(selectedLogoFile);
      await reloadConfig();
      setSelectedLogoFile(null);
      setLogoPreviewUrl(null);
      setLogoSuccess(res.message || 'Корпоративный логотип успешно сохранен в ConfigMap kncc-logo и базе данных!');
      setTimeout(() => setLogoSuccess(null), 5000);
    } catch (err: any) {
      setLogoError(err.message || 'Не удалось загрузить логотип');
    } finally {
      setLogoUploading(false);
    }
  };

  const handleResetLogo = async () => {
    if (!window.confirm('Сбросить текущий логотип на первичное судостроительное лого по умолчанию?')) {
      return;
    }
    setLogoResetting(true);
    setLogoError(null);
    setLogoSuccess(null);
    try {
      const res = await api.resetLogo();
      await reloadConfig();
      setSelectedLogoFile(null);
      setLogoPreviewUrl(null);
      setLogoSuccess(res.message || 'Логотип успешно сброшен на первичное судостроительное лого!');
      setTimeout(() => setLogoSuccess(null), 5000);
    } catch (err: any) {
      setLogoError(err.message || 'Не удалось сбросить логотип');
    } finally {
      setLogoResetting(false);
    }
  };

  // Favicon settings state
  const [selectedFaviconFile, setSelectedFaviconFile] = useState<File | null>(null);
  const [faviconPreviewUrl, setFaviconPreviewUrl] = useState<string | null>(null);
  const [faviconUploading, setFaviconUploading] = useState(false);
  const [faviconResetting, setFaviconResetting] = useState(false);
  const [faviconSuccess, setFaviconSuccess] = useState<string | null>(null);
  const [faviconError, setFaviconError] = useState<string | null>(null);
  const [faviconPreviewTheme, setFaviconPreviewTheme] = useState<'light' | 'dark'>('light');
  const [isDraggingFavicon, setIsDraggingFavicon] = useState(false);

  const handleFaviconFileSelect = (file: File) => {
    setFaviconError(null);
    setFaviconSuccess(null);

    // Validate size: max 512 KB
    if (file.size > 512 * 1024) {
      setFaviconError('Размер файла превышает 512 КБ. Пожалуйста, оптимизируйте фавикон.');
      return;
    }

    const ext = file.name.toLowerCase().split('.').pop();
    const isSvg = ext === 'svg' || file.type === 'image/svg+xml';
    const isIco = ext === 'ico' || file.type === 'image/x-icon' || file.type === 'image/vnd.microsoft.icon';
    const validTypes = ['image/svg+xml', 'image/x-icon', 'image/vnd.microsoft.icon', 'image/png', 'image/jpeg', 'image/webp'];

    if (!validTypes.includes(file.type) && !isSvg && !isIco) {
      setFaviconError('Неподдерживаемый формат. Разрешены только ICO, SVG, PNG, JPEG или WebP.');
      return;
    }

    setSelectedFaviconFile(file);

    const reader = new FileReader();
    reader.onload = (e) => {
      let result = e.target?.result as string;
      if (isSvg && result) {
        const commaIdx = result.indexOf(',');
        if (commaIdx !== -1) {
          result = 'data:image/svg+xml;base64,' + result.substring(commaIdx + 1);
        }
      }
      setFaviconPreviewUrl(result);
    };
    reader.onerror = () => {
      setFaviconError('Не удалось прочитать выбранный файл фавикона.');
    };
    reader.readAsDataURL(file);
  };

  const handleUploadFavicon = async () => {
    if (!selectedFaviconFile) return;
    setFaviconUploading(true);
    setFaviconError(null);
    setFaviconSuccess(null);
    try {
      const res = await api.uploadFavicon(selectedFaviconFile);
      await reloadConfig();
      setSelectedFaviconFile(null);
      setFaviconPreviewUrl(null);
      setFaviconSuccess(res.message || 'Фавикон успешно сохранен в ConfigMap kncc-favicon и базе данных!');
      setTimeout(() => setFaviconSuccess(null), 5000);
    } catch (err: any) {
      setFaviconError(err.message || 'Не удалось загрузить фавикон');
    } finally {
      setFaviconUploading(false);
    }
  };

  const handleResetFavicon = async () => {
    if (!window.confirm('Сбросить кастомный фавикон и вернуть автоматическую миниатюру корпоративного логотипа?')) {
      return;
    }
    setFaviconResetting(true);
    setFaviconError(null);
    setFaviconSuccess(null);
    try {
      const res = await api.resetFavicon();
      await reloadConfig();
      setSelectedFaviconFile(null);
      setFaviconPreviewUrl(null);
      setFaviconSuccess(res.message || 'Фавикон сброшен на миниатюру корпоративного логотипа!');
      setTimeout(() => setFaviconSuccess(null), 5000);
    } catch (err: any) {
      setFaviconError(err.message || 'Не удалось сбросить фавикон');
    } finally {
      setFaviconResetting(false);
    }
  };

  useEffect(() => {
    if (config?.default_theme) {
      setDefaultThemeSetting(config.default_theme);
    }
  }, [config?.default_theme]);

  const fetchUsers = async () => {
    try {
      const res = await api.listUsers();
      setUsers(res.users || []);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  const fetchRBAC = async () => {
    setRbacLoading(true);
    setRbacError(null);
    try {
      const rep = await api.getRBACReport();
      setRbacReport(rep);
    } catch (e: any) {
      console.error(e);
      setRbacError(e.message || 'Failed to retrieve RBAC permissions report');
    } finally {
      setRbacLoading(false);
    }
  };

  const fetchStats = async () => {
    setStatsLoading(true);
    setStatsError(null);
    try {
      const res = await api.getUserStats();
      setStatsData(res);
    } catch (e: any) {
      console.error(e);
      setStatsError(e.message || 'Не удалось загрузить статистику использования');
    } finally {
      setStatsLoading(false);
    }
  };

  const handleSort = (field: keyof UserStats) => {
    if (sortField === field) {
      setSortDirection(prev => (prev === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortField(field);
      setSortDirection('desc');
    }
  };

  const filteredAndSortedStats = useMemo(() => {
    if (!statsData?.users) return [];
    const searchLower = statsSearch.toLowerCase().trim();
    let list = statsData.users.filter(u =>
      u.username.toLowerCase().includes(searchLower) ||
      u.user_id.toLowerCase().includes(searchLower) ||
      (u.provider && u.provider.toLowerCase().includes(searchLower))
    );

    list.sort((a, b) => {
      const valA: any = a[sortField];
      const valB: any = b[sortField];

      if (sortField === 'last_active') {
        const timeA = valA ? new Date(valA).getTime() : 0;
        const timeB = valB ? new Date(valB).getTime() : 0;
        return sortDirection === 'asc' ? timeA - timeB : timeB - timeA;
      }

      if (typeof valA === 'string') {
        return sortDirection === 'asc'
          ? (valA || '').localeCompare(valB || '')
          : (valB || '').localeCompare(valA || '');
      }

      const numA = Number(valA ?? 0);
      const numB = Number(valB ?? 0);
      return sortDirection === 'asc' ? numA - numB : numB - numA;
    });

    return list;
  }, [statsData, statsSearch, sortField, sortDirection]);

  const handleCopyStats = () => {
    if (!statsData) return;
    const headerRow = ['Пользователь', 'Роль', 'Провайдер', 'За 24 часа', 'За 7 дней', 'За 30 дней', 'За год', 'Всего запросов', 'Последняя активность'];
    const dataRows = filteredAndSortedStats.map(u => [
      u.username,
      u.role,
      u.provider,
      u.day_count,
      u.week_count,
      u.month_count,
      u.year_count,
      u.total_count,
      u.last_active ? new Date(u.last_active).toLocaleString() : 'Никогда'
    ]);
    const summaryRow = [
      'ИТОГО (все пользователи)',
      '-',
      '-',
      statsData.total.day_count,
      statsData.total.week_count,
      statsData.total.month_count,
      statsData.total.year_count,
      statsData.total.total_count,
      '-'
    ];
    const tsv = [headerRow.join('\t'), ...dataRows.map(r => r.join('\t')), summaryRow.join('\t')].join('\n');
    navigator.clipboard.writeText(tsv);
    setStatsCopiedNotice(true);
    setTimeout(() => setStatsCopiedNotice(false), 2500);
  };

  const formatLastActive = (dateStr?: string) => {
    if (!dateStr) {
      return <span className="text-gray-400 dark:text-slate-500 text-xs italic">Никогда</span>;
    }
    const d = new Date(dateStr);
    const now = new Date();
    const diffSec = Math.max(0, Math.floor((now.getTime() - d.getTime()) / 1000));
    let relative = '';
    if (diffSec < 60) relative = 'Только что';
    else if (diffSec < 3600) relative = `${Math.floor(diffSec / 60)} мин. назад`;
    else if (diffSec < 86400) relative = `${Math.floor(diffSec / 3600)} ч. назад`;
    else if (diffSec < 2592000) relative = `${Math.floor(diffSec / 86400)} дн. назад`;
    else relative = d.toLocaleDateString();

    return (
      <span title={d.toLocaleString()} className="cursor-help">
        {relative}
      </span>
    );
  };

  useEffect(() => {
    fetchUsers();
  }, []);

  const handleCreateUser = async (e: React.FormEvent) => {
    e.preventDefault();
    setCreateError(null);
    setCreateLoading(true);
    setCreateSuccess(false);

    try {
      await api.createUser({
        username,
        password,
        role,
        email: email || undefined,
      });
      setCreateSuccess(true);
      setUsername('');
      setPassword('');
      setEmail('');
      setRole('user');
      fetchUsers();
      setTimeout(() => {
        setShowAddModal(false);
        setCreateSuccess(false);
      }, 1200);
    } catch (err: any) {
      setCreateError(err.message || 'Failed to create user');
    } finally {
      setCreateLoading(false);
    }
  };

  const handleSaveThemeSettings = async () => {
    setSettingsSaving(true);
    setSettingsSuccess(null);
    setSettingsError(null);
    try {
      await api.updateThemeSettings(defaultThemeSetting);
      await reloadConfig();
      setSettingsSuccess('Тема по умолчанию успешно сохранена в настройках приложения!');
      setTimeout(() => setSettingsSuccess(null), 4000);
    } catch (err: any) {
      setSettingsError(err.message || 'Не удалось сохранить настройки темы');
    } finally {
      setSettingsSaving(false);
    }
  };

  // RBAC Filter helper
  const filteredRules = (rbacReport?.rules || []).filter((rule) => {
    // 1. Search term
    if (rbacSearch.trim()) {
      const q = rbacSearch.toLowerCase();
      const matchGroup = rule.api_group?.toLowerCase().includes(q);
      const matchResource = rule.resource?.toLowerCase().includes(q);
      const matchVerbs = rule.verbs.some((v) => v.toLowerCase().includes(q));
      const matchNs = rule.namespace?.toLowerCase().includes(q);
      const matchOrigin = rule.originated_from?.toLowerCase().includes(q);
      if (!matchGroup && !matchResource && !matchVerbs && !matchNs && !matchOrigin) {
        return false;
      }
    }

    // 2. Verb filter
    if (rbacVerbFilter !== 'all') {
      if (rbacVerbFilter === '*') {
        if (!rule.verbs.includes('*')) return false;
      } else {
        if (!rule.verbs.includes(rbacVerbFilter) && !rule.verbs.includes('*')) {
          return false;
        }
      }
    }

    // 3. Scope filter
    if (rbacScopeFilter === 'cluster' && rule.namespace !== '*') {
      return false;
    }
    if (rbacScopeFilter === 'namespaced' && rule.namespace === '*') {
      return false;
    }

    return true;
  });

  const handleCopyJSON = () => {
    if (!rbacReport) return;
    navigator.clipboard.writeText(JSON.stringify(rbacReport, null, 2));
    setCopiedNotice('Copied JSON report to clipboard!');
    setTimeout(() => setCopiedNotice(null), 3000);
  };

  const handleCopyTable = () => {
    if (!rbacReport || !rbacReport.rules.length) return;
    const header = `TYPE\tSUBJECT\tVERBS\tNAMESPACE\tAPI GROUP\tKIND\tNAMES\tNONRESOURCEURI\tORIGINATED FROM`;
    const rows = filteredRules.map(
      (r) =>
        `${r.type}\t${r.subject}\t${r.verbs.join(',')}\t${r.namespace}\t${r.api_group}\t${r.resource}\t${(r.resource_names || []).join(',') || '*'}\t${(r.non_resource_urls || []).join(',')}\t${r.originated_from || ''}`
    );
    navigator.clipboard.writeText([header, ...rows].join('\n'));
    setCopiedNotice('Copied policy rules table (TSV) to clipboard!');
    setTimeout(() => setCopiedNotice(null), 3000);
  };

  const renderVerbBadge = (verb: string) => {
    let colorClass = 'bg-gray-100 text-gray-700 border-gray-200 dark:bg-slate-800 dark:text-slate-300 dark:border-slate-700';
    if (verb === 'get' || verb === 'list' || verb === 'watch') {
      colorClass = 'bg-emerald-50 text-emerald-700 border-emerald-200 dark:bg-emerald-950/40 dark:text-emerald-300 dark:border-emerald-800';
    } else if (verb === 'create') {
      colorClass = 'bg-blue-50 text-blue-700 border-blue-200 dark:bg-blue-950/40 dark:text-blue-300 dark:border-blue-800';
    } else if (verb === 'delete' || verb === 'deletecollection') {
      colorClass = 'bg-rose-50 text-rose-700 border-rose-200 dark:bg-rose-950/40 dark:text-rose-300 dark:border-rose-800';
    } else if (verb === '*') {
      colorClass = 'bg-amber-50 text-amber-800 border-amber-300 dark:bg-amber-950/40 dark:text-amber-300 dark:border-amber-800 font-bold';
    }
    return (
      <span
        key={verb}
        className={`inline-block px-1.5 py-0.5 rounded text-[10px] font-mono border mr-1 mb-1 ${colorClass}`}
      >
        {verb}
      </span>
    );
  };

  return (
    <div className="max-w-6xl mx-auto py-8 px-4 sm:px-6 lg:px-8">
      {/* Top Header */}
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 mb-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white tracking-tight flex items-center">
            <Shield className="w-6 h-6 mr-2 text-red-600" />
            Administration & Cluster RBAC
          </h1>
          <p className="text-xs text-gray-500 dark:text-slate-400 mt-1">
            Manage users, inspect Kubernetes ServiceAccount permissions, and configure application settings.
          </p>
        </div>

        {activeTab === 'users' && (
          <button
            onClick={() => setShowAddModal(true)}
            className="inline-flex items-center px-4 py-2 border border-transparent rounded-lg text-xs font-semibold text-white shadow-sm transition-all"
            style={{ backgroundColor: 'var(--color-accent, #dc2626)' }}
          >
            <UserPlus className="w-3.5 h-3.5 mr-1.5" />
            Add Local User
          </button>
        )}
      </div>

      {/* Cluster & Security Settings Summary */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-6">
        <div className="bg-white dark:bg-slate-900 p-4 rounded-xl border border-gray-200 dark:border-slate-800 shadow-sm flex items-center justify-between transition-colors">
          <div className="flex items-center">
            <div className="p-2 rounded-lg bg-gray-100 dark:bg-slate-800 mr-3 text-gray-700 dark:text-slate-300">
              <Layers className="w-5 h-5" />
            </div>
            <div>
              <p className="text-xs text-gray-500 dark:text-slate-400 font-medium">Namespace Dropdown</p>
              <p className="text-sm font-bold text-gray-900 dark:text-white">
                {config?.namespace_list_enabled ? 'Enabled' : 'Disabled'}
              </p>
            </div>
          </div>
          <span
            className={`px-2 py-0.5 rounded text-[10px] font-bold uppercase ${
              config?.namespace_list_enabled
                ? 'bg-green-100 dark:bg-green-950/60 text-green-800 dark:text-green-300'
                : 'bg-gray-100 dark:bg-slate-800 text-gray-600 dark:text-slate-400'
            }`}
          >
            {config?.namespace_list_enabled ? 'Active' : 'Manual only'}
          </span>
        </div>

        <div className="bg-white dark:bg-slate-900 p-4 rounded-xl border border-gray-200 dark:border-slate-800 shadow-sm flex items-center justify-between transition-colors">
          <div className="flex items-center">
            <div className="p-2 rounded-lg bg-gray-100 dark:bg-slate-800 mr-3 text-gray-700 dark:text-slate-300">
              <Settings className="w-5 h-5" />
            </div>
            <div>
              <p className="text-xs text-gray-500 dark:text-slate-400 font-medium">Multi-Namespace RBAC</p>
              <p className="text-sm font-bold text-gray-900 dark:text-white">
                {config?.multi_namespace_allowed ? 'Allowed' : 'Restricted'}
              </p>
            </div>
          </div>
          <span
            className={`px-2 py-0.5 rounded text-[10px] font-bold uppercase ${
              config?.multi_namespace_allowed
                ? 'bg-green-100 dark:bg-green-950/60 text-green-800 dark:text-green-300'
                : 'bg-yellow-100 dark:bg-yellow-950/60 text-yellow-800 dark:text-yellow-300'
            }`}
          >
            {config?.multi_namespace_allowed ? 'Cluster-wide' : 'Single NS'}
          </span>
        </div>

        <div className="bg-white dark:bg-slate-900 p-4 rounded-xl border border-gray-200 dark:border-slate-800 shadow-sm flex items-center justify-between transition-colors">
          <div className="flex items-center">
            <div className="p-2 rounded-lg bg-gray-100 dark:bg-slate-800 mr-3 text-gray-700 dark:text-slate-300">
              <Lock className="w-5 h-5" />
            </div>
            <div>
              <p className="text-xs text-gray-500 dark:text-slate-400 font-medium">Default Namespace</p>
              <p className="text-sm font-bold text-gray-900 dark:text-white font-mono">
                {config?.default_namespace || 'kncc'}
              </p>
            </div>
          </div>
          <span className="text-xs text-gray-400 dark:text-slate-500">
            {config?.rate_limit_per_minute || 30} req/m
          </span>
        </div>
      </div>

      {/* Tabs navigation */}
      <div className="flex border-b border-gray-200 dark:border-slate-800 mb-6 space-x-4">
        <button
          onClick={() => setActiveTab('users')}
          className={`pb-3 px-1 text-sm font-semibold flex items-center border-b-2 transition-colors ${
            activeTab === 'users'
              ? 'border-red-600 text-red-600 dark:text-red-500'
              : 'border-transparent text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-200 hover:border-gray-300 dark:hover:border-slate-700'
          }`}
        >
          <Users className="w-4 h-4 mr-2" />
          User Accounts
          <span className="ml-2 bg-gray-100 dark:bg-slate-800 text-gray-700 dark:text-slate-300 text-xs px-2 py-0.5 rounded-full font-mono">
            {users.length}
          </span>
        </button>

        <button
          onClick={() => {
            setActiveTab('stats');
            if (!statsData) fetchStats();
          }}
          className={`pb-3 px-1 text-sm font-semibold flex items-center border-b-2 transition-colors ${
            activeTab === 'stats'
              ? 'border-red-600 text-red-600 dark:text-red-500'
              : 'border-transparent text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-200 hover:border-gray-300 dark:hover:border-slate-700'
          }`}
        >
          <BarChart3 className="w-4 h-4 mr-2" />
          Статистика запросов
          {statsData && (
            <span className="ml-2 bg-red-50 dark:bg-red-950/60 text-red-700 dark:text-red-300 text-xs px-2 py-0.5 rounded-full font-mono">
              {statsData.total.total_count}
            </span>
          )}
        </button>

        <button
          onClick={() => {
            setActiveTab('rbac');
            if (!rbacReport) fetchRBAC();
          }}
          className={`pb-3 px-1 text-sm font-semibold flex items-center border-b-2 transition-colors ${
            activeTab === 'rbac'
              ? 'border-red-600 text-red-600 dark:text-red-500'
              : 'border-transparent text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-200 hover:border-gray-300 dark:hover:border-slate-700'
          }`}
        >
          <KeyRound className="w-4 h-4 mr-2" />
          Kubernetes SA Permissions (RBAC)
          {rbacReport && (
            <span className="ml-2 bg-red-50 dark:bg-red-950/60 text-red-700 dark:text-red-300 text-xs px-2 py-0.5 rounded-full font-mono">
              {rbacReport.rules.length}
            </span>
          )}
        </button>

        <button
          onClick={() => setActiveTab('settings')}
          className={`pb-3 px-1 text-sm font-semibold flex items-center border-b-2 transition-colors ${
            activeTab === 'settings'
              ? 'border-red-600 text-red-600 dark:text-red-500'
              : 'border-transparent text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-200 hover:border-gray-300 dark:hover:border-slate-700'
          }`}
        >
          <Sliders className="w-4 h-4 mr-2" />
          Настройки приложения
          <span className="ml-2 bg-gray-100 dark:bg-slate-800 text-gray-700 dark:text-slate-300 text-xs px-2 py-0.5 rounded-full capitalize font-mono">
            {config?.default_theme || 'light'}
          </span>
        </button>
      </div>

      {/* Copied notification banner */}
      {copiedNotice && (
        <div className="mb-4 bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-800 p-3 rounded-lg text-xs text-emerald-800 dark:text-emerald-300 flex items-center">
          <CheckCircle2 className="w-4 h-4 mr-2 text-emerald-600" />
          <span>{copiedNotice}</span>
        </div>
      )}

      {statsCopiedNotice && (
        <div className="mb-4 bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-800 p-3 rounded-lg text-xs text-emerald-800 dark:text-emerald-300 flex items-center">
          <CheckCircle2 className="w-4 h-4 mr-2 text-emerald-600" />
          <span>Таблица статистики скопирована в буфер обмена (TSV/Excel формат)!</span>
        </div>
      )}

      {/* TAB 1: USERS */}
      {activeTab === 'users' && (
        <div className="bg-white dark:bg-slate-900 rounded-xl shadow-sm border border-gray-200 dark:border-slate-800 overflow-hidden transition-colors">
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200 dark:divide-slate-800 text-left text-xs">
              <thead className="bg-gray-50 dark:bg-slate-800 text-gray-500 dark:text-slate-400 font-semibold uppercase tracking-wider">
                <tr>
                  <th className="px-6 py-3">Username</th>
                  <th className="px-6 py-3">Role</th>
                  <th className="px-6 py-3">Provider</th>
                  <th className="px-6 py-3">Email</th>
                  <th className="px-6 py-3">Created At</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-slate-800 text-gray-900 dark:text-slate-200">
                {users.map((u) => (
                  <tr key={u.id} className="hover:bg-gray-50/80 dark:hover:bg-slate-800/40 transition-colors">
                    <td className="px-6 py-4 font-semibold text-gray-900 dark:text-white">
                      {u.username}
                    </td>
                    <td className="px-6 py-4">
                      <span className={`px-2 py-0.5 rounded text-[11px] font-bold uppercase ${
                        u.role === 'admin'
                          ? 'bg-red-100 dark:bg-red-950/60 text-red-800 dark:text-red-300'
                          : 'bg-gray-100 dark:bg-slate-800 text-gray-800 dark:text-slate-300'
                      }`}>
                        {u.role}
                      </span>
                    </td>
                    <td className="px-6 py-4 uppercase font-mono text-gray-500 dark:text-slate-400">
                      {u.provider}
                    </td>
                    <td className="px-6 py-4 text-gray-600 dark:text-slate-400">
                      {u.email || '—'}
                    </td>
                    <td className="px-6 py-4 text-gray-500 dark:text-slate-400">
                      {new Date(u.created_at).toLocaleString()}
                    </td>
                  </tr>
                ))}
                {users.length === 0 && !loading && (
                  <tr>
                    <td colSpan={5} className="px-6 py-8 text-center text-gray-400 dark:text-slate-500">
                      No users loaded.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* TAB: USER REQUEST STATISTICS */}
      {activeTab === 'stats' && (
        <div className="space-y-6">
          {/* Top KPI Summary Cards */}
          <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
            <div className="bg-white dark:bg-slate-900 p-4 rounded-xl border border-gray-200 dark:border-slate-800 shadow-sm transition-colors">
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs text-gray-500 dark:text-slate-400 font-medium">За 24 часа</span>
                <Clock className="w-4 h-4 text-blue-500" />
              </div>
              <p className="text-xl font-bold text-gray-900 dark:text-white font-mono">
                {statsLoading ? '...' : (statsData?.total.day_count ?? 0)}
              </p>
              <span className="text-[10px] text-gray-400 dark:text-slate-500">последние сутки</span>
            </div>

            <div className="bg-white dark:bg-slate-900 p-4 rounded-xl border border-gray-200 dark:border-slate-800 shadow-sm transition-colors">
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs text-gray-500 dark:text-slate-400 font-medium">За 7 дней</span>
                <Calendar className="w-4 h-4 text-emerald-500" />
              </div>
              <p className="text-xl font-bold text-gray-900 dark:text-white font-mono">
                {statsLoading ? '...' : (statsData?.total.week_count ?? 0)}
              </p>
              <span className="text-[10px] text-gray-400 dark:text-slate-500">последняя неделя</span>
            </div>

            <div className="bg-white dark:bg-slate-900 p-4 rounded-xl border border-gray-200 dark:border-slate-800 shadow-sm transition-colors">
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs text-gray-500 dark:text-slate-400 font-medium">За 30 дней</span>
                <Calendar className="w-4 h-4 text-indigo-500" />
              </div>
              <p className="text-xl font-bold text-gray-900 dark:text-white font-mono">
                {statsLoading ? '...' : (statsData?.total.month_count ?? 0)}
              </p>
              <span className="text-[10px] text-gray-400 dark:text-slate-500">последний месяц</span>
            </div>

            <div className="bg-white dark:bg-slate-900 p-4 rounded-xl border border-gray-200 dark:border-slate-800 shadow-sm transition-colors">
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs text-gray-500 dark:text-slate-400 font-medium">За год</span>
                <TrendingUp className="w-4 h-4 text-amber-500" />
              </div>
              <p className="text-xl font-bold text-gray-900 dark:text-white font-mono">
                {statsLoading ? '...' : (statsData?.total.year_count ?? 0)}
              </p>
              <span className="text-[10px] text-gray-400 dark:text-slate-500">365 дней</span>
            </div>

            <div className="bg-white dark:bg-slate-900 p-4 rounded-xl border border-gray-200 dark:border-slate-800 shadow-sm transition-colors col-span-2 sm:col-span-1">
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs text-gray-500 dark:text-slate-400 font-medium">Всего запросов</span>
                <BarChart3 className="w-4 h-4 text-red-500" />
              </div>
              <p className="text-xl font-bold text-red-600 dark:text-red-400 font-mono">
                {statsLoading ? '...' : (statsData?.total.total_count ?? 0)}
              </p>
              <span className="text-[10px] text-gray-400 dark:text-slate-500">за всё время</span>
            </div>
          </div>

          {/* Controls Bar: Search & Action Buttons */}
          <div className="bg-white dark:bg-slate-900 p-4 rounded-xl border border-gray-200 dark:border-slate-800 shadow-sm flex flex-col sm:flex-row justify-between items-stretch sm:items-center gap-3 transition-colors">
            <div className="relative flex-1 max-w-md">
              <Search className="w-4 h-4 text-gray-400 dark:text-slate-500 absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                placeholder="Поиск по имени пользователя или провайдеру..."
                value={statsSearch}
                onChange={(e) => setStatsSearch(e.target.value)}
                className="w-full pl-9 pr-4 py-1.5 text-xs bg-gray-50 dark:bg-slate-800/80 border border-gray-200 dark:border-slate-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-red-500"
              />
            </div>

            <div className="flex items-center space-x-2">
              <button
                onClick={handleCopyStats}
                disabled={!statsData || statsLoading}
                className="inline-flex items-center px-3 py-1.5 border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 rounded-lg text-xs font-semibold text-gray-700 dark:text-slate-200 hover:bg-gray-50 dark:hover:bg-slate-700/80 shadow-sm transition-all disabled:opacity-50"
                title="Копировать таблицу в буфер обмена для Excel / Google Sheets"
              >
                <FileSpreadsheet className="w-3.5 h-3.5 mr-1.5 text-emerald-600 dark:text-emerald-400" />
                Копировать TSV
              </button>

              <button
                onClick={fetchStats}
                disabled={statsLoading}
                className="inline-flex items-center px-3 py-1.5 border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 rounded-lg text-xs font-semibold text-gray-700 dark:text-slate-200 hover:bg-gray-50 dark:hover:bg-slate-700/80 shadow-sm transition-all disabled:opacity-50"
              >
                <RefreshCw className={`w-3.5 h-3.5 mr-1.5 ${statsLoading ? 'animate-spin text-red-600' : 'text-gray-500 dark:text-slate-400'}`} />
                Обновить
              </button>
            </div>
          </div>

          {/* Stats Error message if any */}
          {statsError && (
            <div className="bg-red-50 dark:bg-red-950/40 border border-red-200 dark:border-red-800 p-4 rounded-xl text-xs text-red-700 dark:text-red-300 flex items-center">
              <AlertCircle className="w-4 h-4 mr-2 flex-shrink-0" />
              <span>{statsError}</span>
            </div>
          )}

          {/* User Request Stats Table */}
          <div className="bg-white dark:bg-slate-900 rounded-xl shadow-sm border border-gray-200 dark:border-slate-800 overflow-hidden transition-colors">
            <div className="overflow-x-auto">
              <table className="min-w-full divide-y divide-gray-200 dark:divide-slate-800 text-left text-xs">
                <thead className="bg-gray-50 dark:bg-slate-800 text-gray-500 dark:text-slate-400 font-semibold uppercase tracking-wider select-none">
                  <tr>
                    <th
                      className="px-6 py-3.5 cursor-pointer hover:text-gray-900 dark:hover:text-white transition-colors"
                      onClick={() => handleSort('username')}
                    >
                      <div className="flex items-center space-x-1">
                        <span>Пользователь</span>
                        {sortField === 'username' ? (
                          sortDirection === 'asc' ? <ArrowUp className="w-3.5 h-3.5 text-red-600" /> : <ArrowDown className="w-3.5 h-3.5 text-red-600" />
                        ) : (
                          <ArrowUpDown className="w-3 h-3 text-gray-400" />
                        )}
                      </div>
                    </th>
                    <th
                      className="px-6 py-3.5 text-center cursor-pointer hover:text-gray-900 dark:hover:text-white transition-colors"
                      onClick={() => handleSort('day_count')}
                    >
                      <div className="flex items-center justify-center space-x-1">
                        <span>За день</span>
                        {sortField === 'day_count' ? (
                          sortDirection === 'asc' ? <ArrowUp className="w-3.5 h-3.5 text-red-600" /> : <ArrowDown className="w-3.5 h-3.5 text-red-600" />
                        ) : (
                          <ArrowUpDown className="w-3 h-3 text-gray-400" />
                        )}
                      </div>
                    </th>
                    <th
                      className="px-6 py-3.5 text-center cursor-pointer hover:text-gray-900 dark:hover:text-white transition-colors"
                      onClick={() => handleSort('week_count')}
                    >
                      <div className="flex items-center justify-center space-x-1">
                        <span>За неделю</span>
                        {sortField === 'week_count' ? (
                          sortDirection === 'asc' ? <ArrowUp className="w-3.5 h-3.5 text-red-600" /> : <ArrowDown className="w-3.5 h-3.5 text-red-600" />
                        ) : (
                          <ArrowUpDown className="w-3 h-3 text-gray-400" />
                        )}
                      </div>
                    </th>
                    <th
                      className="px-6 py-3.5 text-center cursor-pointer hover:text-gray-900 dark:hover:text-white transition-colors"
                      onClick={() => handleSort('month_count')}
                    >
                      <div className="flex items-center justify-center space-x-1">
                        <span>За месяц</span>
                        {sortField === 'month_count' ? (
                          sortDirection === 'asc' ? <ArrowUp className="w-3.5 h-3.5 text-red-600" /> : <ArrowDown className="w-3.5 h-3.5 text-red-600" />
                        ) : (
                          <ArrowUpDown className="w-3 h-3 text-gray-400" />
                        )}
                      </div>
                    </th>
                    <th
                      className="px-6 py-3.5 text-center cursor-pointer hover:text-gray-900 dark:hover:text-white transition-colors"
                      onClick={() => handleSort('year_count')}
                    >
                      <div className="flex items-center justify-center space-x-1">
                        <span>За год</span>
                        {sortField === 'year_count' ? (
                          sortDirection === 'asc' ? <ArrowUp className="w-3.5 h-3.5 text-red-600" /> : <ArrowDown className="w-3.5 h-3.5 text-red-600" />
                        ) : (
                          <ArrowUpDown className="w-3 h-3 text-gray-400" />
                        )}
                      </div>
                    </th>
                    <th
                      className="px-6 py-3.5 text-center cursor-pointer hover:text-gray-900 dark:hover:text-white transition-colors"
                      onClick={() => handleSort('total_count')}
                    >
                      <div className="flex items-center justify-center space-x-1">
                        <span>Всего</span>
                        {sortField === 'total_count' ? (
                          sortDirection === 'asc' ? <ArrowUp className="w-3.5 h-3.5 text-red-600" /> : <ArrowDown className="w-3.5 h-3.5 text-red-600" />
                        ) : (
                          <ArrowUpDown className="w-3 h-3 text-gray-400" />
                        )}
                      </div>
                    </th>
                    <th
                      className="px-6 py-3.5 text-right cursor-pointer hover:text-gray-900 dark:hover:text-white transition-colors"
                      onClick={() => handleSort('last_active')}
                    >
                      <div className="flex items-center justify-end space-x-1">
                        <span>Активность</span>
                        {sortField === 'last_active' ? (
                          sortDirection === 'asc' ? <ArrowUp className="w-3.5 h-3.5 text-red-600" /> : <ArrowDown className="w-3.5 h-3.5 text-red-600" />
                        ) : (
                          <ArrowUpDown className="w-3 h-3 text-gray-400" />
                        )}
                      </div>
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 dark:divide-slate-800">
                  {statsLoading && !statsData ? (
                    <tr>
                      <td colSpan={7} className="px-6 py-12 text-center text-gray-500 dark:text-slate-400">
                        <RefreshCw className="w-6 h-6 animate-spin mx-auto mb-2 text-red-600" />
                        Загрузка статистики использования...
                      </td>
                    </tr>
                  ) : filteredAndSortedStats.length > 0 ? (
                    filteredAndSortedStats.map((item) => (
                      <tr key={item.user_id} className="hover:bg-gray-50/60 dark:hover:bg-slate-800/40 transition-colors">
                        <td className="px-6 py-3.5 whitespace-nowrap">
                          <div className="flex items-center space-x-3">
                            <div className="w-7 h-7 rounded-full bg-red-100 dark:bg-red-950/60 text-red-700 dark:text-red-300 font-bold flex items-center justify-center text-xs flex-shrink-0">
                              {item.username.charAt(0).toUpperCase()}
                            </div>
                            <div>
                              <div className="flex items-center space-x-2">
                                <span className="font-semibold text-gray-900 dark:text-white">{item.username}</span>
                                <span className={`px-1.5 py-0.2 rounded text-[10px] font-bold uppercase ${
                                  item.role === 'admin'
                                    ? 'bg-red-100 dark:bg-red-950/60 text-red-700 dark:text-red-300'
                                    : 'bg-gray-100 dark:bg-slate-800 text-gray-700 dark:text-slate-300'
                                }`}>
                                  {item.role}
                                </span>
                                <span className="px-1.5 py-0.2 rounded text-[10px] bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-400 font-mono">
                                  {item.provider}
                                </span>
                              </div>
                            </div>
                          </div>
                        </td>
                        <td className="px-6 py-3.5 text-center font-mono whitespace-nowrap text-gray-700 dark:text-slate-300">
                          {item.day_count > 0 ? (
                            <span className="font-semibold text-blue-600 dark:text-blue-400">{item.day_count}</span>
                          ) : (
                            <span className="text-gray-300 dark:text-slate-600">0</span>
                          )}
                        </td>
                        <td className="px-6 py-3.5 text-center font-mono whitespace-nowrap text-gray-700 dark:text-slate-300">
                          {item.week_count > 0 ? (
                            <span className="font-semibold text-emerald-600 dark:text-emerald-400">{item.week_count}</span>
                          ) : (
                            <span className="text-gray-300 dark:text-slate-600">0</span>
                          )}
                        </td>
                        <td className="px-6 py-3.5 text-center font-mono whitespace-nowrap text-gray-700 dark:text-slate-300">
                          {item.month_count > 0 ? (
                            <span className="font-semibold text-indigo-600 dark:text-indigo-400">{item.month_count}</span>
                          ) : (
                            <span className="text-gray-300 dark:text-slate-600">0</span>
                          )}
                        </td>
                        <td className="px-6 py-3.5 text-center font-mono whitespace-nowrap text-gray-700 dark:text-slate-300">
                          {item.year_count > 0 ? (
                            <span className="font-semibold text-amber-600 dark:text-amber-400">{item.year_count}</span>
                          ) : (
                            <span className="text-gray-300 dark:text-slate-600">0</span>
                          )}
                        </td>
                        <td className="px-6 py-3.5 text-center font-mono whitespace-nowrap">
                          {item.total_count > 0 ? (
                            <span className="font-bold text-gray-900 dark:text-white px-2 py-0.5 rounded bg-gray-100 dark:bg-slate-800">
                              {item.total_count}
                            </span>
                          ) : (
                            <span className="text-gray-300 dark:text-slate-600">0</span>
                          )}
                        </td>
                        <td className="px-6 py-3.5 text-right whitespace-nowrap text-gray-500 dark:text-slate-400">
                          {formatLastActive(item.last_active)}
                        </td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan={7} className="px-6 py-8 text-center text-gray-400 dark:text-slate-500">
                        {statsSearch ? 'Пользователи не найдены по критерию поиска.' : 'Статистика пока отсутствует.'}
                      </td>
                    </tr>
                  )}
                </tbody>

                {/* Summary Total Row */}
                {statsData && statsData.users.length > 0 && (
                  <tfoot className="bg-gray-100/80 dark:bg-slate-800/80 font-bold border-t-2 border-gray-300 dark:border-slate-700">
                    <tr>
                      <td className="px-6 py-4 text-xs uppercase tracking-wider text-gray-900 dark:text-white">
                        <div className="flex items-center space-x-2">
                          <span className="font-extrabold">ИТОГО (все пользователи)</span>
                          <span className="text-[10px] font-normal text-gray-500 dark:text-slate-400 lowercase">
                            ({statsData.users.length} учётных записей)
                          </span>
                        </div>
                      </td>
                      <td className="px-6 py-4 text-center font-mono text-sm text-blue-600 dark:text-blue-400">
                        {statsData.total.day_count}
                      </td>
                      <td className="px-6 py-4 text-center font-mono text-sm text-emerald-600 dark:text-emerald-400">
                        {statsData.total.week_count}
                      </td>
                      <td className="px-6 py-4 text-center font-mono text-sm text-indigo-600 dark:text-indigo-400">
                        {statsData.total.month_count}
                      </td>
                      <td className="px-6 py-4 text-center font-mono text-sm text-amber-600 dark:text-amber-400">
                        {statsData.total.year_count}
                      </td>
                      <td className="px-6 py-4 text-center font-mono text-sm text-red-600 dark:text-red-400 font-extrabold">
                        {statsData.total.total_count}
                      </td>
                      <td className="px-6 py-4 text-right text-xs text-gray-500 dark:text-slate-400 font-normal">
                        Все диапазоны
                      </td>
                    </tr>
                  </tfoot>
                )}
              </table>
            </div>
          </div>
        </div>
      )}

      {/* TAB 2: KUBERNETES SA RBAC PERMISSIONS */}
      {activeTab === 'rbac' && (
        <div className="space-y-6">
          {/* Top RBAC Summary Cards */}
          <div className="grid grid-cols-1 sm:grid-cols-4 gap-4">
            <div className="bg-white dark:bg-slate-900 p-4 rounded-xl border border-gray-200 dark:border-slate-800 shadow-sm transition-colors">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-semibold text-gray-500 dark:text-slate-400">Target Subject</span>
                <span className="px-1.5 py-0.5 text-[10px] font-bold uppercase rounded bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300">
                  {rbacReport?.subject.kind || 'ServiceAccount'}
                </span>
              </div>
              <p className="text-sm font-bold text-gray-900 dark:text-white font-mono truncate">
                {rbacReport?.subject.name ? `${rbacReport.subject.namespace || 'kncc'}/${rbacReport.subject.name}` : 'kncc/kncc'}
              </p>
              <p className="text-[11px] text-gray-400 dark:text-slate-500 truncate mt-1 font-mono">
                {rbacReport?.subject.raw_username || 'system:serviceaccount:kncc:kncc'}
              </p>
            </div>

            <div className="bg-white dark:bg-slate-900 p-4 rounded-xl border border-gray-200 dark:border-slate-800 shadow-sm transition-colors">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-semibold text-gray-500 dark:text-slate-400">Evaluation Source</span>
                <span className={`px-1.5 py-0.5 text-[10px] font-bold uppercase rounded ${
                  rbacReport?.source === 'cluster-rbac'
                    ? 'bg-green-50 dark:bg-green-950/60 text-green-700 dark:text-green-300'
                    : rbacReport?.source === 'self-subject-rules-review'
                    ? 'bg-blue-50 dark:bg-blue-950/60 text-blue-700 dark:text-blue-300'
                    : 'bg-amber-50 dark:bg-amber-950/60 text-amber-700 dark:text-amber-300'
                }`}>
                  {rbacReport?.source || 'Detecting'}
                </span>
              </div>
              <p className="text-sm font-bold text-gray-900 dark:text-white">
                {rbacReport?.source === 'cluster-rbac' ? 'Cluster RBAC Graph' : rbacReport?.source === 'self-subject-rules-review' ? 'SelfSubjectRulesReview' : 'Simulated / Mock'}
              </p>
              <p className="text-[11px] text-gray-400 dark:text-slate-500 mt-1">
                {rbacReport?.evaluation_error ? 'Partial fallback applied' : 'Resolved without errors'}
              </p>
            </div>

            <div className="bg-white dark:bg-slate-900 p-4 rounded-xl border border-gray-200 dark:border-slate-800 shadow-sm transition-colors">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-semibold text-gray-500 dark:text-slate-400">Active Rules</span>
                <Activity className="w-4 h-4 text-gray-400" />
              </div>
              <p className="text-2xl font-bold text-gray-900 dark:text-white">
                {filteredRules.length}
                {filteredRules.length !== (rbacReport?.rules.length || 0) && (
                  <span className="text-xs font-normal text-gray-500 dark:text-slate-400 ml-1">
                    of {rbacReport?.rules.length || 0}
                  </span>
                )}
              </p>
              <p className="text-[11px] text-gray-400 dark:text-slate-500 mt-1">
                Flat policy rules available
              </p>
            </div>

            <div className="bg-white dark:bg-slate-900 p-4 rounded-xl border border-gray-200 dark:border-slate-800 shadow-sm transition-colors">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-semibold text-gray-500 dark:text-slate-400">Last Evaluated</span>
                <RefreshCw className={`w-3.5 h-3.5 text-gray-400 ${rbacLoading ? 'animate-spin' : ''}`} />
              </div>
              <p className="text-sm font-bold text-gray-900 dark:text-white">
                {rbacReport ? new Date(rbacReport.generated_at).toLocaleTimeString() : '—'}
              </p>
              <p className="text-[11px] text-gray-400 dark:text-slate-500 mt-1">
                {rbacReport ? new Date(rbacReport.generated_at).toLocaleDateString() : 'Pending'}
              </p>
            </div>
          </div>

          {/* Error notice if any */}
          {rbacError && (
            <div className="bg-red-50 dark:bg-red-950/40 border border-red-200 dark:border-red-800 p-4 rounded-xl text-xs text-red-700 dark:text-red-300 flex items-start">
              <AlertCircle className="w-4 h-4 mr-2 flex-shrink-0 mt-0.5" />
              <div>
                <p className="font-semibold">Unable to load RBAC permissions</p>
                <p className="mt-1">{rbacError}</p>
              </div>
            </div>
          )}

          {/* Action Bar: Search, Filters, and Export Buttons */}
          <div className="bg-white dark:bg-slate-900 p-4 rounded-xl border border-gray-200 dark:border-slate-800 shadow-sm flex flex-col md:flex-row justify-between items-center gap-3 transition-colors">
            <div className="flex flex-1 items-center gap-2 w-full md:w-auto">
              <div className="relative flex-1 max-w-sm">
                <Search className="w-4 h-4 text-gray-400 absolute left-3 top-2.5" />
                <input
                  type="text"
                  placeholder="Filter by resource, verb, group, ns..."
                  value={rbacSearch}
                  onChange={(e) => setRbacSearch(e.target.value)}
                  className="w-full pl-9 pr-3 py-1.5 bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 rounded-lg text-xs text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-slate-500 focus:ring-red-500 focus:border-red-500"
                />
              </div>

              <select
                value={rbacVerbFilter}
                onChange={(e) => setRbacVerbFilter(e.target.value)}
                className="px-2.5 py-1.5 border border-gray-300 dark:border-slate-700 rounded-lg text-xs bg-white dark:bg-slate-800 text-gray-700 dark:text-slate-200 focus:ring-red-500 focus:border-red-500"
              >
                <option value="all">All Verbs</option>
                <option value="get">get</option>
                <option value="list">list</option>
                <option value="watch">watch</option>
                <option value="create">create</option>
                <option value="delete">delete</option>
                <option value="*">* (Wildcard)</option>
              </select>

              <select
                value={rbacScopeFilter}
                onChange={(e) => setRbacScopeFilter(e.target.value)}
                className="px-2.5 py-1.5 border border-gray-300 dark:border-slate-700 rounded-lg text-xs bg-white dark:bg-slate-800 text-gray-700 dark:text-slate-200 focus:ring-red-500 focus:border-red-500"
              >
                <option value="all">All Scopes</option>
                <option value="cluster">Cluster-wide (*)</option>
                <option value="namespaced">Namespaced</option>
              </select>
            </div>

            <div className="flex items-center gap-2 w-full md:w-auto justify-end">
              <button
                onClick={handleCopyTable}
                className="inline-flex items-center px-3 py-1.5 border border-gray-300 dark:border-slate-700 rounded-lg text-xs font-semibold text-gray-700 dark:text-slate-200 hover:bg-gray-50 dark:hover:bg-slate-800 shadow-sm transition-all"
                title="Copy formatted table matching rbac-tool policy-rules"
              >
                <Terminal className="w-3.5 h-3.5 mr-1.5 text-gray-500 dark:text-slate-400" />
                Copy Table
              </button>

              <button
                onClick={handleCopyJSON}
                className="inline-flex items-center px-3 py-1.5 border border-gray-300 dark:border-slate-700 rounded-lg text-xs font-semibold text-gray-700 dark:text-slate-200 hover:bg-gray-50 dark:hover:bg-slate-800 shadow-sm transition-all"
                title="Copy raw JSON representation"
              >
                <Copy className="w-3.5 h-3.5 mr-1.5 text-gray-500 dark:text-slate-400" />
                Copy JSON
              </button>

              <button
                onClick={fetchRBAC}
                disabled={rbacLoading}
                className="inline-flex items-center px-3 py-1.5 border border-gray-300 dark:border-slate-700 rounded-lg text-xs font-semibold text-gray-700 dark:text-slate-200 hover:bg-gray-50 dark:hover:bg-slate-800 shadow-sm disabled:opacity-50 transition-all"
              >
                <RefreshCw className={`w-3.5 h-3.5 mr-1.5 text-gray-500 dark:text-slate-400 ${rbacLoading ? 'animate-spin' : ''}`} />
                Refresh
              </button>
            </div>
          </div>

          {/* RBAC Policy Rules Table matching rbac-tool policy-rules */}
          <div className="bg-white dark:bg-slate-900 rounded-xl shadow-sm border border-gray-200 dark:border-slate-800 overflow-hidden transition-colors">
            <div className="overflow-x-auto">
              <table className="min-w-full divide-y divide-gray-200 dark:divide-slate-800 text-left text-xs">
                <thead className="bg-gray-50 dark:bg-slate-800 text-gray-500 dark:text-slate-400 font-semibold uppercase tracking-wider font-mono">
                  <tr>
                    <th className="px-4 py-3">TYPE</th>
                    <th className="px-4 py-3">SUBJECT</th>
                    <th className="px-4 py-3">VERBS</th>
                    <th className="px-4 py-3">NAMESPACE</th>
                    <th className="px-4 py-3">API GROUP</th>
                    <th className="px-4 py-3">KIND / RESOURCE</th>
                    <th className="px-4 py-3">NAMES</th>
                    <th className="px-4 py-3">NONRESOURCEURI</th>
                    <th className="px-4 py-3">ORIGINATED FROM</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 dark:divide-slate-800 font-mono text-[11px] text-gray-900 dark:text-slate-200">
                  {filteredRules.map((rule, idx) => (
                    <tr key={idx} className="hover:bg-gray-50/80 dark:hover:bg-slate-800/40 transition-colors">
                      <td className="px-4 py-3 font-semibold text-gray-700 dark:text-slate-300">
                        <span className="px-1.5 py-0.5 rounded text-[10px] font-sans font-bold uppercase bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300">
                          {rule.type}
                        </span>
                      </td>
                      <td className="px-4 py-3 font-medium text-gray-900 dark:text-white truncate max-w-[140px]" title={rule.subject}>
                        {rule.subject}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex flex-wrap max-w-xs">
                          {rule.verbs.map((v) => renderVerbBadge(v))}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        {rule.namespace === '*' ? (
                          <span className="px-1.5 py-0.5 rounded text-[10px] font-sans font-bold bg-purple-100 dark:bg-purple-950/60 text-purple-800 dark:text-purple-300">
                            * (cluster)
                          </span>
                        ) : (
                          <span className="px-1.5 py-0.5 rounded text-[10px] font-sans bg-gray-100 dark:bg-slate-800 text-gray-700 dark:text-slate-300">
                            {rule.namespace}
                          </span>
                        )}
                      </td>
                      <td className="px-4 py-3 text-gray-600 dark:text-slate-400">
                        {rule.api_group === 'core' || rule.api_group === '' ? (
                          <span className="text-gray-400 dark:text-slate-500">core</span>
                        ) : (
                          <span className="font-semibold text-indigo-700 dark:text-indigo-400">{rule.api_group}</span>
                        )}
                      </td>
                      <td className="px-4 py-3 font-bold text-gray-900 dark:text-white">
                        {rule.resource || '—'}
                      </td>
                      <td className="px-4 py-3 text-gray-500 dark:text-slate-400">
                        {(rule.resource_names || []).join(',') || '*'}
                      </td>
                      <td className="px-4 py-3 text-gray-500 dark:text-slate-400">
                        {(rule.non_resource_urls || []).join(',') || '—'}
                      </td>
                      <td className="px-4 py-3 text-gray-600 dark:text-slate-400 truncate max-w-[200px]" title={rule.originated_from || ''}>
                        {rule.originated_from ? (
                          <span className={`px-2 py-0.5 rounded text-[10px] font-sans font-medium ${
                            rule.originated_from.startsWith('ClusterRole')
                              ? 'bg-blue-50 dark:bg-blue-950/60 text-blue-800 dark:text-blue-300'
                              : rule.originated_from.startsWith('Role')
                              ? 'bg-amber-50 dark:bg-amber-950/60 text-amber-800 dark:text-amber-300'
                              : 'bg-gray-100 dark:bg-slate-800 text-gray-700 dark:text-slate-300'
                          }`}>
                            {rule.originated_from}
                          </span>
                        ) : (
                          '—'
                        )}
                      </td>
                    </tr>
                  ))}

                  {filteredRules.length === 0 && !rbacLoading && (
                    <tr>
                      <td colSpan={9} className="px-6 py-10 text-center text-gray-400 dark:text-slate-500 font-sans">
                        <KeyRound className="w-8 h-8 mx-auto text-gray-300 dark:text-slate-600 mb-2" />
                        <p className="text-sm font-medium">No matching policy rules found.</p>
                        <p className="text-xs text-gray-400 dark:text-slate-500 mt-1">Try broadening your search or filter terms.</p>
                      </td>
                    </tr>
                  )}

                  {rbacLoading && filteredRules.length === 0 && (
                    <tr>
                      <td colSpan={9} className="px-6 py-10 text-center text-gray-400 dark:text-slate-500 font-sans">
                        <RefreshCw className="w-6 h-6 mx-auto text-red-500 animate-spin mb-2" />
                        <p className="text-xs">Evaluating Kubernetes RBAC policy rules...</p>
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {/* TAB 3: SETTINGS (APPLICATION CONFIGURATION) */}
      {activeTab === 'settings' && (
        <div className="space-y-6">
          {/* Default Theme Configuration Card */}
          <div className="bg-white dark:bg-slate-900 rounded-xl p-6 shadow-sm border border-gray-200 dark:border-slate-800 transition-colors">
            <div className="flex items-center space-x-3 mb-4">
              <div className="p-2.5 rounded-lg bg-red-50 dark:bg-red-950/50 text-red-600 dark:text-red-400">
                <Palette className="w-5 h-5" />
              </div>
              <div>
                <h3 className="text-base font-bold text-gray-900 dark:text-white">
                  Тема оформления по умолчанию
                </h3>
                <p className="text-xs text-gray-500 dark:text-slate-400 mt-0.5">
                  Определяет тему интерфейса, которая автоматически применяется для новых пользователей, гостей на экране авторизации и браузеров без сохранённых настроек.
                </p>
              </div>
            </div>

            {settingsSuccess && (
              <div className="mb-4 bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-800 p-3 rounded-lg text-xs text-emerald-800 dark:text-emerald-300 flex items-center">
                <CheckCircle2 className="w-4 h-4 mr-2 text-emerald-600" />
                <span>{settingsSuccess}</span>
              </div>
            )}

            {settingsError && (
              <div className="mb-4 bg-red-50 dark:bg-red-950/40 border border-red-200 dark:border-red-800 p-3 rounded-lg text-xs text-red-700 dark:text-red-300 flex items-center">
                <AlertCircle className="w-4 h-4 mr-2 text-red-600" />
                <span>{settingsError}</span>
              </div>
            )}

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 my-6">
              {/* Light Option */}
              <div
                onClick={() => setDefaultThemeSetting('light')}
                className={`cursor-pointer rounded-xl p-4 border-2 transition-all flex items-start space-x-3.5 ${
                  defaultThemeSetting === 'light'
                    ? 'border-red-600 bg-red-50/30 dark:bg-red-950/20 shadow-sm'
                    : 'border-gray-200 dark:border-slate-800 hover:border-gray-300 dark:hover:border-slate-700 bg-white dark:bg-slate-900'
                }`}
              >
                <div className={`p-2 rounded-lg ${
                  defaultThemeSetting === 'light'
                    ? 'bg-amber-100 text-amber-600'
                    : 'bg-gray-100 dark:bg-slate-800 text-gray-500 dark:text-slate-400'
                }`}>
                  <Sun className="w-5 h-5" />
                </div>
                <div className="flex-1">
                  <div className="flex items-center justify-between">
                    <span className="font-bold text-sm text-gray-900 dark:text-white">
                      Светлая тема (Light)
                    </span>
                    <input
                      type="radio"
                      name="default_theme"
                      checked={defaultThemeSetting === 'light'}
                      onChange={() => setDefaultThemeSetting('light')}
                      className="text-red-600 focus:ring-red-500 h-4 w-4"
                    />
                  </div>
                  <p className="text-xs text-gray-500 dark:text-slate-400 mt-1 leading-relaxed">
                    Классическое светлое оформление со светлым фоном и карточками. Рекомендуется для светлых помещений и дневной работы.
                  </p>
                </div>
              </div>

              {/* Dark Option */}
              <div
                onClick={() => setDefaultThemeSetting('dark')}
                className={`cursor-pointer rounded-xl p-4 border-2 transition-all flex items-start space-x-3.5 ${
                  defaultThemeSetting === 'dark'
                    ? 'border-red-600 bg-red-50/30 dark:bg-red-950/20 shadow-sm'
                    : 'border-gray-200 dark:border-slate-800 hover:border-gray-300 dark:hover:border-slate-700 bg-white dark:bg-slate-900'
                }`}
              >
                <div className={`p-2 rounded-lg ${
                  defaultThemeSetting === 'dark'
                    ? 'bg-indigo-100 dark:bg-indigo-950 text-indigo-600 dark:text-indigo-400'
                    : 'bg-gray-100 dark:bg-slate-800 text-gray-500 dark:text-slate-400'
                }`}>
                  <Moon className="w-5 h-5" />
                </div>
                <div className="flex-1">
                  <div className="flex items-center justify-between">
                    <span className="font-bold text-sm text-gray-900 dark:text-white">
                      Тёмная тема (Dark)
                    </span>
                    <input
                      type="radio"
                      name="default_theme"
                      checked={defaultThemeSetting === 'dark'}
                      onChange={() => setDefaultThemeSetting('dark')}
                      className="text-red-600 focus:ring-red-500 h-4 w-4"
                    />
                  </div>
                  <p className="text-xs text-gray-500 dark:text-slate-400 mt-1 leading-relaxed">
                    Высококонтрастное графитовое оформление (Slate-950). Снижает утомляемость глаз при круглосуточном мониторинге NOC и ночных сменах.
                  </p>
                </div>
              </div>
            </div>

            <div className="flex items-center justify-between pt-4 border-t border-gray-100 dark:border-slate-800">
              <span className="text-xs text-gray-500 dark:text-slate-400">
                Текущее значение на сервере: <strong className="capitalize text-gray-800 dark:text-slate-200 font-mono">{config?.default_theme || 'light'}</strong>
              </span>

              <button
                onClick={handleSaveThemeSettings}
                disabled={settingsSaving}
                className="inline-flex items-center px-4 py-2 border border-transparent rounded-lg text-xs font-semibold text-white shadow-sm transition-all disabled:opacity-50"
                style={{ backgroundColor: 'var(--color-accent, #dc2626)' }}
              >
                {settingsSaving ? (
                  <>
                    <RefreshCw className="w-3.5 h-3.5 mr-1.5 animate-spin" />
                    Сохранение...
                  </>
                ) : (
                  <>
                    <Check className="w-3.5 h-3.5 mr-1.5" />
                    Сохранить тему по умолчанию
                  </>
                )}
              </button>
            </div>
          </div>

          {/* Corporate Branding & Logo Configuration Card */}
          <div className="bg-white dark:bg-slate-900 rounded-xl p-6 shadow-sm border border-gray-200 dark:border-slate-800 transition-colors">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4">
              <div className="flex items-center space-x-3">
                <div className="p-2.5 rounded-lg bg-blue-50 dark:bg-blue-950/50 text-blue-600 dark:text-blue-400">
                  <ImageIcon className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-gray-900 dark:text-white">
                    Корпоративный брендинг и логотип компании
                  </h3>
                  <p className="text-xs text-gray-500 dark:text-slate-400 mt-0.5">
                    Логотип отображается на постоянной панели навигации (Navbar), на экране авторизации и в отчетах. Поддерживаются форматы SVG, PNG, JPEG, WebP (до 1 МБ).
                  </p>
                </div>
              </div>

              <span className={`px-2.5 py-1 rounded-full text-xs font-semibold whitespace-nowrap self-start sm:self-auto ${
                config?.has_custom_logo
                  ? 'bg-emerald-50 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
                  : 'bg-blue-50 dark:bg-blue-950/60 text-blue-700 dark:text-blue-300 border border-blue-200 dark:border-blue-800'
              }`}>
                {config?.has_custom_logo ? 'Пользовательский логотип' : 'Дефолтное судостроительное лого'}
              </span>
            </div>

            {logoSuccess && (
              <div className="mb-4 bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-800 p-3 rounded-lg text-xs text-emerald-800 dark:text-emerald-300 flex items-center">
                <CheckCircle2 className="w-4 h-4 mr-2 text-emerald-600 flex-shrink-0" />
                <span>{logoSuccess}</span>
              </div>
            )}

            {logoError && (
              <div className="mb-4 bg-red-50 dark:bg-red-950/40 border border-red-200 dark:border-red-800 p-3 rounded-lg text-xs text-red-700 dark:text-red-300 flex items-center">
                <AlertCircle className="w-4 h-4 mr-2 text-red-600 flex-shrink-0" />
                <span>{logoError}</span>
              </div>
            )}

            <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 my-6 items-start">
              {/* Preview Section */}
              <div className="lg:col-span-5 bg-gray-50 dark:bg-slate-800/50 rounded-xl p-4 border border-gray-200 dark:border-slate-700 flex flex-col items-center">
                <div className="flex items-center justify-between w-full mb-3">
                  <span className="text-xs font-bold text-gray-700 dark:text-slate-300">
                    {selectedLogoFile ? 'Предпросмотр нового файла:' : 'Текущий активный логотип:'}
                  </span>
                  
                  {/* Background Toggle for Preview */}
                  <div className="flex items-center space-x-1 bg-white dark:bg-slate-900 border border-gray-200 dark:border-slate-700 rounded-lg p-0.5">
                    <button
                      type="button"
                      onClick={() => setLogoPreviewTheme('light')}
                      className={`p-1 rounded text-xs transition-colors ${
                        logoPreviewTheme === 'light'
                          ? 'bg-gray-100 dark:bg-slate-800 text-gray-900 dark:text-white font-bold'
                          : 'text-gray-400 hover:text-gray-700'
                      }`}
                      title="Предпросмотр на светлом фоне"
                    >
                      <Sun className="w-3.5 h-3.5" />
                    </button>
                    <button
                      type="button"
                      onClick={() => setLogoPreviewTheme('dark')}
                      className={`p-1 rounded text-xs transition-colors ${
                        logoPreviewTheme === 'dark'
                          ? 'bg-gray-100 dark:bg-slate-800 text-gray-900 dark:text-white font-bold'
                          : 'text-gray-400 hover:text-gray-700'
                      }`}
                      title="Предпросмотр на тёмном фоне"
                    >
                      <Moon className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>

                {/* Preview Box */}
                <div className={`w-full h-36 rounded-xl flex items-center justify-center p-4 border transition-colors ${
                  logoPreviewTheme === 'dark'
                    ? 'bg-slate-950 border-slate-800 text-white'
                    : 'bg-white border-gray-200 text-gray-900 shadow-inner'
                }`}>
                  {previewError ? (
                    <div className="text-center text-xs text-red-500 dark:text-red-400 flex flex-col items-center">
                      <AlertCircle className="w-5 h-5 mb-1 text-red-500" />
                      <span>Не удалось отобразить превью</span>
                    </div>
                  ) : (
                    <img
                      key={logoPreviewUrl || config?.logo_url || 'default'}
                      src={logoPreviewUrl || config?.logo_url || '/api/system/logo'}
                      alt="Logo Preview"
                      onError={() => setPreviewError(true)}
                      onLoad={() => setPreviewError(false)}
                      className="max-h-24 max-w-full object-contain"
                    />
                  )}
                </div>

                <div className="mt-3 text-center">
                  <p className="text-[11px] text-gray-500 dark:text-slate-400">
                    {selectedLogoFile ? (
                      <span className="font-semibold text-blue-600 dark:text-blue-400">
                        {selectedLogoFile.name} ({(selectedLogoFile.size / 1024).toFixed(1)} КБ)
                      </span>
                    ) : config?.has_custom_logo ? (
                      <span>Загружен пользовательский логотип компании</span>
                    ) : (
                      <span>Используется первичное судостроительное векторное лого</span>
                    )}
                  </p>
                </div>
              </div>

              {/* Upload Dropzone & Controls */}
              <div className="lg:col-span-7 space-y-4">
                {/* Drag & Drop Area */}
                <div
                  onDragOver={(e) => {
                    e.preventDefault();
                    setIsDraggingLogo(true);
                  }}
                  onDragLeave={() => setIsDraggingLogo(false)}
                  onDrop={(e) => {
                    e.preventDefault();
                    setIsDraggingLogo(false);
                    if (e.dataTransfer.files && e.dataTransfer.files[0]) {
                      handleLogoFileSelect(e.dataTransfer.files[0]);
                    }
                  }}
                  className={`border-2 border-dashed rounded-xl p-6 text-center cursor-pointer transition-colors ${
                    isDraggingLogo
                      ? 'border-blue-500 bg-blue-50/50 dark:bg-blue-950/20'
                      : 'border-gray-300 dark:border-slate-700 hover:border-gray-400 dark:hover:border-slate-600 bg-gray-50/50 dark:bg-slate-800/30'
                  }`}
                  onClick={() => {
                    document.getElementById('logo-file-input')?.click();
                  }}
                >
                  <input
                    id="logo-file-input"
                    type="file"
                    accept=".svg,.png,.jpg,.jpeg,.webp"
                    className="hidden"
                    onChange={(e) => {
                      if (e.target.files && e.target.files[0]) {
                        handleLogoFileSelect(e.target.files[0]);
                      }
                    }}
                  />
                  <div className="flex flex-col items-center">
                    <div className="p-3 bg-white dark:bg-slate-800 rounded-full shadow-sm text-blue-600 dark:text-blue-400 mb-2">
                      <Upload className="w-5 h-5" />
                    </div>
                    <p className="text-xs font-semibold text-gray-900 dark:text-white">
                      Перетащите файл логотипа сюда или <span className="text-blue-600 dark:text-blue-400 underline">выберите с диска</span>
                    </p>
                    <p className="text-[11px] text-gray-400 dark:text-slate-500 mt-1">
                      SVG (рекомендуется), PNG, JPEG или WebP до 1 МБ
                    </p>
                  </div>
                </div>

                {/* Storage Info Banner */}
                <div className="p-3 bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-700/80 rounded-lg text-xs text-gray-600 dark:text-slate-400 flex items-start space-x-2">
                  <Layers className="w-4 h-4 text-blue-500 mt-0.5 flex-shrink-0" />
                  <div className="text-[11px] leading-relaxed">
                    Логотип сохраняется в выделенном <strong>Kubernetes ConfigMap (<code className="font-mono text-blue-600 dark:text-blue-400">kncc-logo</code>)</strong> и дублируется в SQLite. Изменения применяются мгновенно без перезапуска подов.
                  </div>
                </div>

                {/* Action Buttons */}
                <div className="flex flex-wrap items-center justify-between gap-3 pt-2">
                  <button
                    type="button"
                    onClick={handleResetLogo}
                    disabled={logoResetting || logoUploading || !config?.has_custom_logo}
                    className="inline-flex items-center px-3.5 py-2 border border-gray-300 dark:border-slate-700 rounded-lg text-xs font-medium text-gray-700 dark:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-800 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                    title="Сбросить логотип на первичное судостроительное лого"
                  >
                    <RotateCcw className={`w-3.5 h-3.5 mr-1.5 ${logoResetting ? 'animate-spin' : ''}`} />
                    {logoResetting ? 'Сброс...' : 'Сбросить к дефолтному'}
                  </button>

                  <div className="flex items-center space-x-2">
                    {selectedLogoFile && (
                      <button
                        type="button"
                        onClick={() => {
                          setSelectedLogoFile(null);
                          setLogoPreviewUrl(null);
                        }}
                        className="px-3 py-2 text-xs text-gray-500 hover:text-gray-700 dark:hover:text-gray-300"
                      >
                        Отмена
                      </button>
                    )}

                    <button
                      type="button"
                      onClick={handleUploadLogo}
                      disabled={!selectedLogoFile || logoUploading}
                      className="inline-flex items-center px-4 py-2 border border-transparent rounded-lg text-xs font-semibold text-white shadow-sm transition-all disabled:opacity-50 disabled:cursor-not-allowed"
                      style={{ backgroundColor: 'var(--color-accent, #dc2626)' }}
                    >
                      {logoUploading ? (
                        <>
                          <RefreshCw className="w-3.5 h-3.5 mr-1.5 animate-spin" />
                          Сохранение в ConfigMap...
                        </>
                      ) : (
                        <>
                          <Upload className="w-3.5 h-3.5 mr-1.5" />
                          Загрузить и применить логотип
                        </>
                      )}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          {/* Favicon Configuration Card */}
          <div className="bg-white dark:bg-slate-900 rounded-xl p-6 shadow-sm border border-gray-200 dark:border-slate-800 transition-colors">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4">
              <div className="flex items-center space-x-3">
                <div className="p-2.5 rounded-lg bg-indigo-50 dark:bg-indigo-950/50 text-indigo-600 dark:text-indigo-400">
                  <Globe className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-gray-900 dark:text-white">
                    Фавикон сайта (Favicon)
                  </h3>
                  <p className="text-xs text-gray-500 dark:text-slate-400 mt-0.5">
                    Иконка во вкладках браузера и закладках. По умолчанию фавикон автоматически формируется как миниатюра корпоративного логотипа. Поддерживаются форматы ICO, SVG, PNG, WebP (до 512 КБ).
                  </p>
                </div>
              </div>

              <span className={`px-2.5 py-1 rounded-full text-xs font-semibold whitespace-nowrap self-start sm:self-auto ${
                config?.has_custom_favicon
                  ? 'bg-emerald-50 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
                  : 'bg-blue-50 dark:bg-blue-950/60 text-blue-700 dark:text-blue-300 border border-blue-200 dark:border-blue-800'
              }`}>
                {config?.has_custom_favicon ? 'Пользовательский фавикон' : 'Миниатюра корпоративного логотипа'}
              </span>
            </div>

            {faviconSuccess && (
              <div className="mb-4 bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-800 p-3 rounded-lg text-xs text-emerald-800 dark:text-emerald-300 flex items-center">
                <CheckCircle2 className="w-4 h-4 mr-2 text-emerald-600 flex-shrink-0" />
                <span>{faviconSuccess}</span>
              </div>
            )}

            {faviconError && (
              <div className="mb-4 bg-red-50 dark:bg-red-950/40 border border-red-200 dark:border-red-800 p-3 rounded-lg text-xs text-red-700 dark:text-red-300 flex items-center">
                <AlertCircle className="w-4 h-4 mr-2 text-red-600 flex-shrink-0" />
                <span>{faviconError}</span>
              </div>
            )}

            <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 my-6 items-start">
              {/* Favicon Preview Section */}
              <div className="lg:col-span-5 bg-gray-50 dark:bg-slate-800/50 rounded-xl p-4 border border-gray-200 dark:border-slate-700 flex flex-col items-center">
                <div className="flex items-center justify-between w-full mb-3">
                  <span className="text-xs font-bold text-gray-700 dark:text-slate-300">
                    {selectedFaviconFile ? 'Предпросмотр нового фавикона:' : 'Текущий активный фавикон:'}
                  </span>
                  
                  {/* Background Toggle for Preview */}
                  <div className="flex items-center space-x-1 bg-white dark:bg-slate-900 border border-gray-200 dark:border-slate-700 rounded-lg p-0.5">
                    <button
                      type="button"
                      onClick={() => setFaviconPreviewTheme('light')}
                      className={`p-1 rounded text-xs transition-colors ${
                        faviconPreviewTheme === 'light'
                          ? 'bg-gray-100 dark:bg-slate-800 text-gray-900 dark:text-white font-bold'
                          : 'text-gray-400 hover:text-gray-700'
                      }`}
                      title="Предпросмотр на светлом фоне"
                    >
                      <Sun className="w-3.5 h-3.5" />
                    </button>
                    <button
                      type="button"
                      onClick={() => setFaviconPreviewTheme('dark')}
                      className={`p-1 rounded text-xs transition-colors ${
                        faviconPreviewTheme === 'dark'
                          ? 'bg-gray-100 dark:bg-slate-800 text-gray-900 dark:text-white font-bold'
                          : 'text-gray-400 hover:text-gray-700'
                      }`}
                      title="Предпросмотр на тёмном фоне"
                    >
                      <Moon className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>

                {/* Mock Browser Tab Preview */}
                <div className={`w-full rounded-lg p-3 transition-colors mb-3 ${
                  faviconPreviewTheme === 'light' ? 'bg-slate-200 text-slate-800' : 'bg-slate-950 text-slate-200'
                }`}>
                  <span className="text-[10px] font-semibold text-gray-500 uppercase tracking-wider block mb-2">
                    Вкладка браузера (16x16)
                  </span>
                  <div className={`inline-flex items-center space-x-2 px-3 py-1.5 rounded-t-md text-xs font-medium max-w-full shadow-sm border-t border-x ${
                    faviconPreviewTheme === 'light'
                      ? 'bg-white text-gray-900 border-gray-300'
                      : 'bg-slate-800 text-white border-slate-700'
                  }`}>
                    <img
                      src={faviconPreviewUrl || config?.favicon_url || '/api/system/favicon'}
                      alt="Favicon preview"
                      className="w-4 h-4 object-contain flex-shrink-0"
                    />
                    <span className="truncate max-w-[140px] text-xs">
                      {config?.app_name || 'kncc'}
                    </span>
                    <span className="text-gray-400 text-xs ml-1 hover:text-gray-600">✕</span>
                  </div>
                </div>

                {/* Enlarged Icon Preview */}
                <div className={`w-full rounded-lg p-4 flex flex-col items-center justify-center transition-colors ${
                  faviconPreviewTheme === 'light' ? 'bg-white border border-gray-200' : 'bg-slate-900 border border-slate-700'
                }`}>
                  <div className="flex items-center space-x-6">
                    <div className="text-center">
                      <div className="w-12 h-12 rounded-lg border border-dashed border-gray-300 dark:border-slate-700 flex items-center justify-center p-1 mb-1">
                        <img
                          src={faviconPreviewUrl || config?.favicon_url || '/api/system/favicon'}
                          alt="Favicon 32x32"
                          className="w-8 h-8 object-contain"
                        />
                      </div>
                      <span className="text-[10px] text-gray-400">32×32</span>
                    </div>

                    <div className="text-center">
                      <div className="w-16 h-16 rounded-xl border border-dashed border-gray-300 dark:border-slate-700 flex items-center justify-center p-2 mb-1">
                        <img
                          src={faviconPreviewUrl || config?.favicon_url || '/api/system/favicon'}
                          alt="Favicon 48x48"
                          className="w-12 h-12 object-contain"
                        />
                      </div>
                      <span className="text-[10px] text-gray-400">48×48</span>
                    </div>
                  </div>
                </div>

                <div className="w-full mt-2.5 flex items-center justify-between text-[11px] text-gray-500 dark:text-slate-400">
                  <span>Режим: {config?.has_custom_favicon ? 'Пользовательский' : 'Миниатюра логотипа'}</span>
                  <span>Лимит: 512 КБ</span>
                </div>
              </div>

              {/* Favicon Upload & Actions */}
              <div className="lg:col-span-7 flex flex-col justify-between h-full space-y-4">
                <div
                  onDragOver={(e) => {
                    e.preventDefault();
                    setIsDraggingFavicon(true);
                  }}
                  onDragLeave={() => setIsDraggingFavicon(false)}
                  onDrop={(e) => {
                    e.preventDefault();
                    setIsDraggingFavicon(false);
                    if (e.dataTransfer.files && e.dataTransfer.files[0]) {
                      handleFaviconFileSelect(e.dataTransfer.files[0]);
                    }
                  }}
                  className={`border-2 border-dashed rounded-xl p-6 text-center transition-all cursor-pointer ${
                    isDraggingFavicon
                      ? 'border-indigo-500 bg-indigo-50/50 dark:bg-indigo-950/20'
                      : 'border-gray-300 dark:border-slate-700 hover:border-gray-400 dark:hover:border-slate-600 bg-gray-50/50 dark:bg-slate-800/20'
                  }`}
                  onClick={() => document.getElementById('favicon-upload-input')?.click()}
                >
                  <input
                    id="favicon-upload-input"
                    type="file"
                    accept=".ico,.svg,.png,.webp,.jpeg,.jpg,image/*"
                    className="hidden"
                    onChange={(e) => {
                      if (e.target.files && e.target.files[0]) {
                        handleFaviconFileSelect(e.target.files[0]);
                      }
                    }}
                  />
                  <div className="flex flex-col items-center">
                    <div className="p-3 bg-white dark:bg-slate-800 rounded-full shadow-sm mb-3 text-indigo-600 dark:text-indigo-400 border border-gray-100 dark:border-slate-700">
                      <Upload className="w-5 h-5" />
                    </div>
                    <p className="text-xs font-semibold text-gray-700 dark:text-slate-200">
                      Перетащите файл фавикона сюда или <span className="text-indigo-600 dark:text-indigo-400 underline">выберите на диске</span>
                    </p>
                    <p className="text-[11px] text-gray-500 dark:text-slate-400 mt-1">
                      Поддерживаются ICO, SVG, PNG, WebP (рекомендуется 32×32 или векторный SVG)
                    </p>
                  </div>
                </div>

                {selectedFaviconFile && (
                  <div className="bg-indigo-50/60 dark:bg-indigo-950/30 border border-indigo-200 dark:border-indigo-800/50 rounded-lg p-3 text-xs flex items-center justify-between">
                    <div className="flex items-center space-x-2 truncate">
                      <Globe className="w-4 h-4 text-indigo-600 dark:text-indigo-400 flex-shrink-0" />
                      <span className="font-semibold text-indigo-950 dark:text-indigo-200 truncate">
                        {selectedFaviconFile.name}
                      </span>
                      <span className="text-indigo-600/75 dark:text-indigo-400/75 text-[11px]">
                        ({Math.round(selectedFaviconFile.size / 1024)} КБ)
                      </span>
                    </div>
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation();
                        setSelectedFaviconFile(null);
                        setFaviconPreviewUrl(null);
                      }}
                      className="text-gray-400 hover:text-red-500 font-bold ml-2 text-xs"
                    >
                      Отмена
                    </button>
                  </div>
                )}

                <div className="flex flex-col sm:flex-row items-center justify-between gap-3 pt-2 border-t border-gray-100 dark:border-slate-800">
                  <div className="text-[11px] text-gray-500 dark:text-slate-400 flex items-center">
                    <Shield className="w-3.5 h-3.5 mr-1 text-emerald-500" />
                    Синхронизируется с ConfigMap kncc-favicon
                  </div>

                  <div className="flex items-center space-x-2 w-full sm:w-auto justify-end">
                    {(config?.has_custom_favicon || selectedFaviconFile) && (
                      <button
                        type="button"
                        onClick={handleResetFavicon}
                        disabled={faviconResetting || faviconUploading}
                        className="inline-flex items-center px-3 py-2 border border-gray-300 dark:border-slate-700 rounded-lg text-xs font-medium text-gray-700 dark:text-slate-300 bg-white dark:bg-slate-800 hover:bg-gray-50 dark:hover:bg-slate-700 transition-colors disabled:opacity-50"
                      >
                        <RotateCcw className={`w-3.5 h-3.5 mr-1.5 ${faviconResetting ? 'animate-spin' : ''}`} />
                        Сбросить на логотип компании
                      </button>
                    )}

                    <button
                      type="button"
                      onClick={handleUploadFavicon}
                      disabled={!selectedFaviconFile || faviconUploading}
                      className="inline-flex items-center px-4 py-2 border border-transparent rounded-lg text-xs font-semibold text-white shadow-sm transition-all disabled:opacity-50 disabled:cursor-not-allowed"
                      style={{ backgroundColor: 'var(--color-accent, #dc2626)' }}
                    >
                      {faviconUploading ? (
                        <>
                          <RefreshCw className="w-3.5 h-3.5 mr-1.5 animate-spin" />
                          Сохранение в ConfigMap...
                        </>
                      ) : (
                        <>
                          <Upload className="w-3.5 h-3.5 mr-1.5" />
                          Загрузить и применить фавикон
                        </>
                      )}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          {/* System Configuration Reference Card */}
          <div className="bg-white dark:bg-slate-900 rounded-xl p-6 shadow-sm border border-gray-200 dark:border-slate-800 transition-colors">
            <h3 className="text-sm font-bold text-gray-900 dark:text-white mb-3 flex items-center">
              <Settings className="w-4 h-4 mr-2 text-gray-500" />
              Конфигурация параметров кластера и безопасности
            </h3>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 text-xs">
              <div className="p-3 bg-gray-50 dark:bg-slate-800/60 rounded-lg border border-gray-200 dark:border-slate-700">
                <span className="text-gray-500 dark:text-slate-400 block mb-1">Namespace Discovery</span>
                <span className="font-semibold text-gray-900 dark:text-white">
                  {config?.namespace_list_enabled ? 'Включено (чтение K8s Namespaces)' : 'Отключено (ручной ввод)'}
                </span>
              </div>
              <div className="p-3 bg-gray-50 dark:bg-slate-800/60 rounded-lg border border-gray-200 dark:border-slate-700">
                <span className="text-gray-500 dark:text-slate-400 block mb-1">Пространство имён по умолчанию</span>
                <span className="font-mono font-semibold text-gray-900 dark:text-white">
                  {config?.default_namespace || 'kncc'}
                </span>
              </div>
              <div className="p-3 bg-gray-50 dark:bg-slate-800/60 rounded-lg border border-gray-200 dark:border-slate-700">
                <span className="text-gray-500 dark:text-slate-400 block mb-1">Multi-Namespace Probing</span>
                <span className="font-semibold text-gray-900 dark:text-white">
                  {config?.multi_namespace_allowed ? 'Разрешено кластерной ролью' : 'Ограничено собственным namespace'}
                </span>
              </div>
              <div className="p-3 bg-gray-50 dark:bg-slate-800/60 rounded-lg border border-gray-200 dark:border-slate-700">
                <span className="text-gray-500 dark:text-slate-400 block mb-1">Лимит запросов в минуту</span>
                <span className="font-semibold text-gray-900 dark:text-white">
                  {config?.rate_limit_per_minute || 30} запросов / мин на пользователя
                </span>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Add User Modal */}
      {showAddModal && (
        <div className="fixed inset-0 z-50 overflow-y-auto bg-black/50 dark:bg-black/70 flex items-center justify-center p-4">
          <div className="bg-white dark:bg-slate-900 rounded-xl max-w-md w-full p-6 shadow-xl border border-gray-100 dark:border-slate-800 transition-colors">
            <h3 className="text-lg font-bold text-gray-900 dark:text-white mb-4 flex items-center">
              <UserPlus className="w-5 h-5 mr-2 text-red-600" />
              Create Local Account
            </h3>

            {createError && (
              <div className="mb-4 bg-red-50 dark:bg-red-950/40 p-3 rounded-md text-xs text-red-700 dark:text-red-300 flex items-start">
                <AlertCircle className="w-4 h-4 mr-1.5 flex-shrink-0 mt-0.5" />
                <span>{createError}</span>
              </div>
            )}
            {createSuccess && (
              <div className="mb-4 bg-green-50 dark:bg-green-950/40 p-3 rounded-md text-xs text-green-700 dark:text-green-300 flex items-center">
                <Check className="w-4 h-4 mr-1.5" />
                <span>User created successfully!</span>
              </div>
            )}

            <form onSubmit={handleCreateUser} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-gray-700 dark:text-slate-300 mb-1">
                  Username *
                </label>
                <input
                  type="text"
                  required
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="e.g. dev_engineer"
                  className="w-full px-3 py-1.5 bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 rounded-lg text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-slate-500 focus:ring-red-500 focus:border-red-500"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-gray-700 dark:text-slate-300 mb-1">
                  Password *
                </label>
                <input
                  type="password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="••••••••"
                  className="w-full px-3 py-1.5 bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 rounded-lg text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-slate-500 focus:ring-red-500 focus:border-red-500"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-gray-700 dark:text-slate-300 mb-1">
                  Email (optional)
                </label>
                <input
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="dev@corp.local"
                  className="w-full px-3 py-1.5 border border-gray-300 dark:border-slate-700 rounded-lg text-sm bg-white dark:bg-slate-800 text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-slate-500 focus:ring-red-500 focus:border-red-500"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-gray-700 dark:text-slate-300 mb-1">
                  Role
                </label>
                <select
                  value={role}
                  onChange={(e) => setRole(e.target.value as 'user' | 'admin')}
                  className="w-full px-3 py-1.5 border border-gray-300 dark:border-slate-700 rounded-lg text-sm bg-white dark:bg-slate-800 text-gray-900 dark:text-white focus:ring-red-500 focus:border-red-500"
                >
                  <option value="user">User (can create and view checks)</option>
                  <option value="admin">Admin (full access & user management)</option>
                </select>
              </div>

              <div className="flex justify-end space-x-3 pt-4 border-t border-gray-100 dark:border-slate-800">
                <button
                  type="button"
                  onClick={() => setShowAddModal(false)}
                  className="px-3.5 py-1.5 border border-gray-300 dark:border-slate-700 rounded-lg text-xs font-semibold text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-800"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={createLoading}
                  className="px-4 py-1.5 rounded-lg text-xs font-semibold text-white shadow-sm disabled:opacity-50"
                  style={{ backgroundColor: 'var(--color-accent, #dc2626)' }}
                >
                  {createLoading ? 'Saving...' : 'Create User'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
