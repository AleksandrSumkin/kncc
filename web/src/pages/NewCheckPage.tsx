import React, { useState, useEffect } from 'react';
import { api } from '../api';
import { useTheme } from '../context/ThemeContext';
import { useTranslation } from '../context/LanguageContext';
import { CheckMode, Protocol, TargetService } from '../types';
import { Plus, Trash2, ArrowRight, ShieldAlert, Globe, Network, ListFilter, Edit3, Loader2, RotateCcw, Play } from 'lucide-react';

export interface PrefillData {
  fromReportId?: string;
  mode: CheckMode;
  namespace?: string;
  targets: TargetService[];
}

interface NewCheckPageProps {
  onCheckCreated: (reportId: string) => void;
  initialData?: PrefillData | null;
  onClearPrefill?: () => void;
}

export const NewCheckPage: React.FC<NewCheckPageProps> = ({ onCheckCreated, initialData, onClearPrefill }) => {
  const { config } = useTheme();
  const { t } = useTranslation();

  const [mode, setMode] = useState<CheckMode>(initialData?.mode || 'host');
  const [namespace, setNamespace] = useState<string>(
    initialData?.namespace || config?.default_namespace || 'kncc'
  );
  const [availableNamespaces, setAvailableNamespaces] = useState<string[]>([]);
  const [loadingNamespaces, setLoadingNamespaces] = useState<boolean>(false);
  const [namespacesError, setNamespacesError] = useState<string | null>(null);
  const [isManualInput, setIsManualInput] = useState<boolean>(false);
  const [targets, setTargets] = useState<TargetService[]>(() => {
    if (initialData?.targets && initialData.targets.length > 0) {
      return JSON.parse(JSON.stringify(initialData.targets));
    }
    return [{ protocol: 'tcp', host: '', port: 443, timeout_seconds: 5 }];
  });
  const [quickAddItems, setQuickAddItems] = useState<TargetService[]>(() => {
    if (config?.quick_add_presets && config.quick_add_presets.length > 0) {
      return config.quick_add_presets.slice(0, 3);
    }
    return [
      { name: 'K8s API', protocol: 'tcp', host: '10.96.0.1', port: 443 },
      { name: 'Google DNS', protocol: 'icmp', host: '8.8.8.8' },
    ];
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.getQuickAddTargets()
      .then((res) => {
        if (res.quick_add && res.quick_add.length > 0) {
          setQuickAddItems(res.quick_add);
        }
      })
      .catch((err) => {
        console.warn('Failed to load dynamic quick-add targets:', err);
      });
  }, []);

  useEffect(() => {
    if (config?.quick_add_presets && config.quick_add_presets.length > 0) {
      setQuickAddItems((current) => {
        if (!current || current.length === 0) {
          return config.quick_add_presets!.slice(0, 3);
        }
        return current;
      });
    }
  }, [config?.quick_add_presets]);

  useEffect(() => {
    if (config?.default_namespace && !namespace && !initialData?.namespace) {
      setNamespace(config.default_namespace);
    }
  }, [config?.default_namespace, initialData]);

  useEffect(() => {
    if (initialData) {
      if (initialData.mode) setMode(initialData.mode);
      if (initialData.namespace) setNamespace(initialData.namespace);
      if (initialData.targets && initialData.targets.length > 0) {
        setTargets(JSON.parse(JSON.stringify(initialData.targets)));
      }
    }
  }, [initialData]);

  const handleResetForm = () => {
    setMode('host');
    setNamespace(config?.default_namespace || 'kncc');
    setTargets([{ protocol: 'tcp', host: '', port: 443, timeout_seconds: 5 }]);
    setError(null);
    onClearPrefill?.();
  };

  useEffect(() => {
    if (mode === 'pod' && config?.namespace_list_enabled && config?.multi_namespace_allowed) {
      setLoadingNamespaces(true);
      setNamespacesError(null);
      api.getNamespaces()
        .then((res) => {
          const list = res.namespaces || [];
          setAvailableNamespaces(list);
          if (list.length > 0 && !namespace) {
            setNamespace(list[0]);
          }
        })
        .catch((err) => {
          console.warn('Failed to load namespaces list:', err);
          setNamespacesError(err.message || 'Unable to list cluster namespaces');
          setIsManualInput(true);
        })
        .finally(() => {
          setLoadingNamespaces(false);
        });
    }
  }, [mode, config?.namespace_list_enabled, config?.multi_namespace_allowed]);

  const handleAddTarget = (preset?: Partial<TargetService>) => {
    if (targets.length >= 10) return;
    const newTarget: TargetService = {
      name: preset?.name,
      protocol: preset?.protocol || 'tcp',
      host: preset?.host || '',
      port: preset?.port !== undefined ? preset.port : (preset?.protocol === 'icmp' ? undefined : 80),
      path: preset?.path || '',
      timeout_seconds: preset?.timeout_seconds || 5,
      insecure_tls: preset?.insecure_tls || false,
    };

    if (targets.length === 1 && !targets[0].host.trim()) {
      setTargets([newTarget]);
    } else {
      setTargets([...targets, newTarget]);
    }
  };

  const handleRemoveTarget = (index: number) => {
    if (targets.length <= 1) return;
    setTargets(targets.filter((_, i) => i !== index));
  };

  const handleTargetChange = (index: number, field: keyof TargetService, value: any) => {
    const updated = [...targets];
    updated[index] = { ...updated[index], [field]: value };

    if (field === 'protocol') {
      if (value === 'http' && (!updated[index].port || updated[index].port === 443)) {
        updated[index].port = 80;
      } else if (value === 'https' && (!updated[index].port || updated[index].port === 80)) {
        updated[index].port = 443;
      } else if (value === 'icmp') {
        delete updated[index].port;
        delete updated[index].path;
      }
    }

    setTargets(updated);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    // Validation
    if (targets.length === 0) {
      setError(t('newCheck.atLeastOneTarget'));
      return;
    }

    for (let i = 0; i < targets.length; i++) {
      const tgt = targets[i];
      if (!tgt.host.trim()) {
        setError(`${t('newCheck.targetService')} #${i + 1}: ${t('newCheck.hostRequired')}`);
        return;
      }
      if (tgt.protocol !== 'icmp') {
        if (!tgt.port || tgt.port < 1 || tgt.port > 65535) {
          setError(`${t('newCheck.targetService')} #${i + 1}: ${t('newCheck.portRequired')}`);
          return;
        }
      }
    }

    setLoading(true);
    try {
      const report = await api.createCheck({
        mode,
        namespace: mode === 'pod' ? namespace : undefined,
        targets,
      });
      onClearPrefill?.();
      onCheckCreated(report.id);
    } catch (err: any) {
      setError(err.message || 'Failed to submit check');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="max-w-4xl mx-auto py-8 px-4 sm:px-6 lg:px-8">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900 dark:text-white tracking-tight">
          {t('newCheck.title')}
        </h1>
        <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
          {t('newCheck.subtitle')}
        </p>
      </div>

      {initialData && (
        <div className="mb-6 bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800 rounded-xl p-4 flex flex-col sm:flex-row sm:items-center justify-between gap-3 text-xs text-amber-900 dark:text-amber-300 shadow-sm">
          <div className="flex items-center space-x-2.5">
            <RotateCcw className="w-4 h-4 text-amber-600 dark:text-amber-400 flex-shrink-0" />
            <span>
              {t('newCheck.prefillNotice')} {initialData.fromReportId ? `(#${initialData.fromReportId.slice(0, 8)})` : ''}.
            </span>
          </div>
          <button
            type="button"
            onClick={handleResetForm}
            className="self-end sm:self-auto px-2.5 py-1 bg-white dark:bg-slate-800 border border-amber-300 dark:border-amber-700 hover:bg-amber-100 dark:hover:bg-slate-700 rounded-md text-amber-800 dark:text-amber-300 font-semibold transition-colors"
          >
            {t('newCheck.clearPrefill')}
          </button>
        </div>
      )}

      {error && (
        <div className="mb-6 bg-red-50 dark:bg-red-950/40 border-l-4 border-red-500 p-4 rounded-md flex items-start">
          <ShieldAlert className="w-5 h-5 text-red-500 mr-2 flex-shrink-0 mt-0.5" />
          <div className="text-sm text-red-700 dark:text-red-300">{error}</div>
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-6">
        {/* Network Mode Card */}
        <div className="bg-white dark:bg-slate-900 rounded-xl shadow-sm border border-gray-200 dark:border-slate-800 p-6 transition-colors">
          <h2 className="text-base font-semibold text-gray-900 dark:text-white mb-3 flex items-center">
            <Network className="w-4 h-4 mr-2 text-gray-500 dark:text-slate-400" />
            {t('newCheck.mode')}
          </h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <label
              className={`relative flex flex-col p-4 border rounded-xl cursor-pointer transition-all ${
                mode === 'host'
                  ? 'border-red-500 ring-2 ring-red-100 dark:ring-red-950/60 bg-red-50/20 dark:bg-red-950/20'
                  : 'border-gray-200 dark:border-slate-800 hover:border-gray-300 dark:hover:border-slate-700 bg-white dark:bg-slate-900'
              }`}
            >
              <div className="flex items-center justify-between mb-1">
                <span className="font-semibold text-sm text-gray-900 dark:text-white">{t('newCheck.hostMode')}</span>
                <input
                  type="radio"
                  name="mode"
                  value="host"
                  checked={mode === 'host'}
                  onChange={() => setMode('host')}
                  className="text-red-600 focus:ring-red-500"
                />
              </div>
              <p className="text-xs text-gray-500 dark:text-slate-400 leading-relaxed">
                {t('newCheck.hostModeDesc')}
              </p>
            </label>

            <label
              className={`relative flex flex-col p-4 border rounded-xl cursor-pointer transition-all ${
                mode === 'pod'
                  ? 'border-red-500 ring-2 ring-red-100 dark:ring-red-950/60 bg-red-50/20 dark:bg-red-950/20'
                  : 'border-gray-200 dark:border-slate-800 hover:border-gray-300 dark:hover:border-slate-700 bg-white dark:bg-slate-900'
              }`}
            >
              <div className="flex items-center justify-between mb-1">
                <span className="font-semibold text-sm text-gray-900 dark:text-white">{t('newCheck.podMode')}</span>
                <input
                  type="radio"
                  name="mode"
                  value="pod"
                  checked={mode === 'pod'}
                  onChange={() => setMode('pod')}
                  className="text-red-600 focus:ring-red-500"
                />
              </div>
              <p className="text-xs text-gray-500 dark:text-slate-400 leading-relaxed">
                {t('newCheck.podModeDesc')}
              </p>
            </label>
          </div>

          {mode === 'pod' && (
            <div className="mt-4 pt-4 border-t border-gray-100 dark:border-slate-800">
              <div className="flex justify-between items-center mb-1">
                <label className="block text-xs font-semibold text-gray-700 dark:text-slate-300">
                  {t('newCheck.namespace')}
                </label>
                {config?.namespace_list_enabled && config?.multi_namespace_allowed && availableNamespaces.length > 0 && (
                  <button
                    type="button"
                    onClick={() => setIsManualInput(!isManualInput)}
                    className="text-xs text-red-600 dark:text-red-400 hover:text-red-700 dark:hover:text-red-300 flex items-center font-medium"
                  >
                    {isManualInput ? (
                      <>
                        <ListFilter className="w-3.5 h-3.5 mr-1" />
                        {t('common.filter')}
                      </>
                    ) : (
                      <>
                        <Edit3 className="w-3.5 h-3.5 mr-1" />
                        {t('common.actions')}
                      </>
                    )}
                  </button>
                )}
              </div>

              {loadingNamespaces ? (
                <div className="flex items-center text-xs text-gray-500 dark:text-slate-400 py-2">
                  <Loader2 className="w-4 h-4 animate-spin mr-2 text-red-600" />
                  Loading cluster namespaces...
                </div>
              ) : config?.namespace_list_enabled && config?.multi_namespace_allowed && availableNamespaces.length > 0 && !isManualInput ? (
                <select
                  value={namespace}
                  onChange={(e) => setNamespace(e.target.value)}
                  className="block w-full px-3 py-2 bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 rounded-lg text-sm text-gray-900 dark:text-white focus:ring-red-500 focus:border-red-500 font-mono"
                >
                  {availableNamespaces.map((ns) => (
                    <option key={ns} value={ns}>
                      {ns}
                    </option>
                  ))}
                </select>
              ) : (
                <input
                  type="text"
                  required
                  value={namespace}
                  onChange={(e) => setNamespace(e.target.value)}
                  placeholder="e.g. production, backend"
                  className="block w-full px-3 py-2 bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 rounded-lg text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-slate-500 focus:ring-red-500 focus:border-red-500 font-mono"
                />
              )}

              {namespacesError && (
                <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
                  {namespacesError} (manual input active)
                </p>
              )}
              <p className="mt-1.5 text-xs text-gray-500 dark:text-slate-400">
                {t('newCheck.namespaceHelp')}
              </p>
            </div>
          )}
        </div>

        {/* Target Services Card */}
        <div className="bg-white dark:bg-slate-900 rounded-xl shadow-sm border border-gray-200 dark:border-slate-800 p-6 transition-colors">
          <div className="flex justify-between items-center mb-4">
            <div>
              <h2 className="text-base font-semibold text-gray-900 dark:text-white flex items-center">
                <Globe className="w-4 h-4 mr-2 text-gray-500 dark:text-slate-400" />
                {t('newCheck.targets')} ({targets.length})
              </h2>
              <p className="text-xs text-gray-500 dark:text-slate-400 mt-0.5">
                {t('newCheck.subtitle')}
              </p>
            </div>

            {/* Quick Presets */}
            {quickAddItems.length > 0 && (
              <div className="hidden sm:flex items-center space-x-2 text-xs">
                <span className="text-gray-400 dark:text-slate-500">{t('newCheck.quickAdd')}:</span>
                {quickAddItems.map((item, idx) => {
                  const label = item.name
                    ? `+ ${item.name}`
                    : item.protocol === 'icmp'
                      ? `+ ${item.host} (ICMP)`
                      : `+ ${item.host}${item.port ? `:${item.port}` : ''}`;
                  const title = `${item.protocol.toUpperCase()} ${item.host}${item.port ? `:${item.port}` : ''}${item.path || ''}`;
                  return (
                    <button
                      key={idx}
                      type="button"
                      title={title}
                      onClick={() => handleAddTarget(item)}
                      className="px-2 py-1 bg-gray-100 dark:bg-slate-800 hover:bg-gray-200 dark:hover:bg-slate-700 text-gray-700 dark:text-slate-300 rounded transition-colors truncate max-w-[150px]"
                    >
                      {label}
                    </button>
                  );
                })}
              </div>
            )}
          </div>

          <div className="space-y-3">
            {targets.map((target, idx) => (
              <div
                key={idx}
                className="flex flex-col md:flex-row items-start md:items-center gap-3 p-3 bg-gray-50/70 dark:bg-slate-800/60 border border-gray-200 dark:border-slate-700 rounded-xl"
              >
                <div className="w-full md:w-28 flex-shrink-0">
                  <label className="block text-xs text-gray-500 dark:text-slate-400 mb-0.5">{t('newCheck.protocol')}</label>
                  <select
                    value={target.protocol}
                    onChange={(e) => handleTargetChange(idx, 'protocol', e.target.value as Protocol)}
                    className="block w-full px-2.5 py-1.5 bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 rounded-md text-sm font-semibold text-gray-800 dark:text-slate-200 uppercase focus:ring-red-500 focus:border-red-500"
                  >
                    <option value="tcp">TCP</option>
                    <option value="udp">UDP</option>
                    <option value="http">HTTP</option>
                    <option value="https">HTTPS</option>
                    <option value="icmp">ICMP (Ping)</option>
                  </select>
                </div>

                <div className="flex-1 w-full">
                  <label className="block text-xs text-gray-500 dark:text-slate-400 mb-0.5">{t('newCheck.host')}</label>
                  <input
                    type="text"
                    required
                    value={target.host}
                    onChange={(e) => handleTargetChange(idx, 'host', e.target.value)}
                    placeholder="e.g. postgres.internal, 192.168.1.10"
                    className="block w-full px-3 py-1.5 bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 rounded-md text-sm font-mono text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-slate-500 focus:ring-red-500 focus:border-red-500"
                  />
                </div>

                {target.protocol !== 'icmp' && (
                  <div className="w-full md:w-24 flex-shrink-0">
                    <label className="block text-xs text-gray-500 dark:text-slate-400 mb-0.5">{t('newCheck.port')}</label>
                    <input
                      type="number"
                      required
                      min={1}
                      max={65535}
                      value={target.port || ''}
                      onChange={(e) => handleTargetChange(idx, 'port', parseInt(e.target.value) || 0)}
                      placeholder="80"
                      className="block w-full px-2.5 py-1.5 bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 rounded-md text-sm font-mono text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-slate-500 focus:ring-red-500 focus:border-red-500"
                    />
                  </div>
                )}

                {(target.protocol === 'http' || target.protocol === 'https') && (
                  <div className="w-full md:w-36 flex-shrink-0">
                    <label className="block text-xs text-gray-500 dark:text-slate-400 mb-0.5">{t('newCheck.path')}</label>
                    <input
                      type="text"
                      value={target.path || '/'}
                      onChange={(e) => handleTargetChange(idx, 'path', e.target.value)}
                      placeholder="/healthz"
                      className="block w-full px-2.5 py-1.5 bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 rounded-md text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-slate-500 focus:ring-red-500 focus:border-red-500"
                    />
                  </div>
                )}

                {target.protocol === 'https' && (
                  <div className="flex items-center pt-4 md:pt-4">
                    <label className="inline-flex items-center text-xs text-gray-600 dark:text-slate-300 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={target.insecure_tls || false}
                        onChange={(e) => handleTargetChange(idx, 'insecure_tls', e.target.checked)}
                        className="rounded text-red-600 focus:ring-red-500 mr-1.5"
                      />
                      {t('newCheck.insecureTls')}
                    </label>
                  </div>
                )}

                <div className="w-full md:w-20 flex-shrink-0">
                  <label className="block text-xs text-gray-500 dark:text-slate-400 mb-0.5">{t('newCheck.timeout')}</label>
                  <div className="flex items-center">
                    <input
                      type="number"
                      min={1}
                      max={30}
                      value={target.timeout_seconds || 5}
                      onChange={(e) => handleTargetChange(idx, 'timeout_seconds', parseInt(e.target.value) || 5)}
                      className="block w-full px-2 py-1.5 bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 rounded-md text-sm text-gray-900 dark:text-white focus:ring-red-500 focus:border-red-500 text-center"
                    />
                    <span className="text-xs text-gray-400 dark:text-slate-500 ml-1">{t('newCheck.seconds')}</span>
                  </div>
                </div>

                <div className="pt-2 md:pt-4">
                  <button
                    type="button"
                    disabled={targets.length <= 1}
                    onClick={() => handleRemoveTarget(idx)}
                    className="p-1.5 text-gray-400 dark:text-slate-500 hover:text-red-500 dark:hover:text-red-400 disabled:opacity-20 rounded transition-colors"
                    title={t('newCheck.removeTarget')}
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            ))}
          </div>

          <div className="mt-4 flex justify-between items-center">
            <button
              type="button"
              disabled={targets.length >= 10}
              onClick={() => handleAddTarget()}
              className="inline-flex items-center px-3 py-1.5 border border-dashed border-gray-300 dark:border-slate-700 text-xs font-semibold text-gray-700 dark:text-slate-300 bg-white dark:bg-slate-900 hover:bg-gray-50 dark:hover:bg-slate-800 rounded-lg transition-colors disabled:opacity-50"
            >
              <Plus className="w-3.5 h-3.5 mr-1" />
              {t('newCheck.addTarget')}
            </button>
            <span className="text-xs text-gray-400 dark:text-slate-500">Up to 10 targets per check</span>
          </div>
        </div>

        {/* Submit */}
        <div className="flex justify-end">
          <button
            type="submit"
            disabled={loading}
            className="inline-flex items-center px-6 py-2.5 border border-transparent rounded-lg shadow-sm text-sm font-semibold text-white transition-all disabled:opacity-50"
            style={{ backgroundColor: 'var(--color-accent, #dc2626)' }}
          >
            {loading ? (
              <div className="w-5 h-5 border-2 border-white border-t-transparent rounded-full animate-spin mr-2" />
            ) : (
              <Play className="w-4 h-4 mr-2 fill-current" />
            )}
            <span>{loading ? t('newCheck.runningCheck') : t('newCheck.runCheck')}</span>
            {!loading && <ArrowRight className="w-4 h-4 ml-2" />}
          </button>
        </div>
      </form>
    </div>
  );
};
