import React, { useState, useEffect } from 'react';
import { api } from '../api';
import { useTheme } from '../context/ThemeContext';
import { useTranslation } from '../context/LanguageContext';
import { ScheduledCheck, CheckMode, Protocol, TargetService } from '../types';
import {
  Activity,
  Plus,
  Play,
  Pause,
  Trash2,
  ExternalLink,
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Clock,
  Server,
  Shield,
  RefreshCw,
  Loader2,
  Layers,
  ListFilter,
  Edit3,
} from 'lucide-react';

interface MonitoringPageProps {
  onSelectReport?: (reportId: string) => void;
}

const getReportStatusTooltip = (status?: string, t?: (key: string) => string) => {
  if (status === 'completed') return t ? t('monitoring.statusReportCompleted') : 'Completed';
  if (status === 'failed') return t ? t('monitoring.statusReportFailed') : 'Failed';
  if (status === 'running' || status === 'pending') return t ? t('monitoring.statusReportRunning') : 'Running';
  return t ? t('monitoring.statusReportNever') : 'Never run';
};

export const MonitoringPage: React.FC<MonitoringPageProps> = ({ onSelectReport }) => {
  const { config } = useTheme();
  const { t } = useTranslation();

  const [checks, setChecks] = useState<ScheduledCheck[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [runningId, setRunningId] = useState<string | null>(null);
  const [isModalOpen, setIsModalOpen] = useState(false);

  const allowedIntervals = config?.monitoring?.allowed_intervals && config.monitoring.allowed_intervals.length > 0
    ? config.monitoring.allowed_intervals
    : ['30s', '1m', '5m', '15m', '30m', '1h'];

  const metricsPort = config?.metrics?.port || 9090;
  const metricsPath = config?.metrics?.path || '/metrics';
  const metricsAuthEnabled = config?.metrics?.auth_enabled || false;

  const fetchChecks = async (isManualRefresh = false) => {
    if (isManualRefresh) setRefreshing(true);
    try {
      const res = await api.listScheduledChecks();
      setChecks(res.scheduled_checks || []);
      setError(null);
    } catch (err: any) {
      setError(err.message || 'Failed to fetch scheduled checks');
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => {
    fetchChecks();
    const interval = setInterval(() => {
      fetchChecks();
    }, 10000);
    return () => clearInterval(interval);
  }, []);

  const handleToggle = async (check: ScheduledCheck) => {
    try {
      const updated = await api.toggleScheduledCheck(check.id);
      setChecks((prev) => prev.map((c) => (c.id === check.id ? updated : c)));
    } catch (err: any) {
      alert(`Error toggling check: ${err.message}`);
    }
  };

  const handleRunNow = async (check: ScheduledCheck) => {
    setRunningId(check.id);
    try {
      const report = await api.runScheduledCheck(check.id);
      await fetchChecks();
      if (onSelectReport && report.id) {
        onSelectReport(report.id);
      }
    } catch (err: any) {
      alert(`Error running check: ${err.message}`);
    } finally {
      setRunningId(null);
    }
  };

  const handleDelete = async (check: ScheduledCheck) => {
    if (check.is_system) {
      alert('System checks cannot be deleted');
      return;
    }
    if (!confirm(t('monitoring.deleteConfirm'))) {
      return;
    }
    try {
      await api.deleteScheduledCheck(check.id);
      setChecks((prev) => prev.filter((c) => c.id !== check.id));
    } catch (err: any) {
      alert(`Error deleting check: ${err.message}`);
    }
  };

  const systemCount = checks.filter((c) => c.is_system).length;
  const ephemeralCount = checks.filter((c) => c.is_ephemeral).length;
  const activeCount = checks.filter((c) => c.enabled).length;

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8 space-y-6">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <div className="flex items-center space-x-3">
            <div className="w-10 h-10 rounded-xl bg-red-600/10 dark:bg-red-500/20 text-red-600 dark:text-red-400 flex items-center justify-center shadow-sm">
              <Activity className="w-6 h-6 stroke-[2.5]" />
            </div>
            <div>
              <h1 className="text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                {t('monitoring.title')}
              </h1>
              <p className="text-sm text-gray-500 dark:text-slate-400">
                {t('monitoring.subtitle')}
              </p>
            </div>
          </div>
        </div>
        <div className="flex items-center space-x-3">
          <button
            onClick={() => fetchChecks(true)}
            disabled={refreshing}
            className="inline-flex items-center px-3 py-2 border border-gray-300 dark:border-slate-700 shadow-sm text-sm font-medium rounded-lg text-gray-700 dark:text-slate-200 bg-white dark:bg-slate-800 hover:bg-gray-50 dark:hover:bg-slate-700 transition"
          >
            <RefreshCw className={`w-4 h-4 mr-1.5 ${refreshing ? 'animate-spin' : ''}`} />
            {t('common.refresh')}
          </button>
          <button
            onClick={() => setIsModalOpen(true)}
            className="inline-flex items-center px-4 py-2 border border-transparent shadow-sm text-sm font-medium rounded-lg text-white bg-red-600 hover:bg-red-700 transition"
          >
            <Plus className="w-4 h-4 mr-1.5" />
            {t('monitoring.newScheduledCheck')}
          </button>
        </div>
      </div>

      {/* Exporter Status Cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div className="bg-white dark:bg-slate-900 border border-gray-200 dark:border-slate-800 rounded-xl p-4 shadow-sm">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-slate-400">
              {t('monitoring.exporterCardTitle')}
            </span>
            <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-400">
              <span className="w-1.5 h-1.5 mr-1 rounded-full bg-emerald-500 animate-pulse" />
              Active
            </span>
          </div>
          <div className="mt-2 flex items-baseline justify-between">
            <div className="text-xl font-bold text-gray-900 dark:text-white">
              Port :{metricsPort}
            </div>
            <span className="text-xs text-gray-400 dark:text-slate-500 font-mono">
              {metricsPath}
            </span>
          </div>
          <div className="mt-2 text-xs text-gray-500 dark:text-slate-400 truncate">
            Scrape: <code className="text-xs font-mono bg-gray-100 dark:bg-slate-800 px-1 py-0.5 rounded">:{metricsPort}{metricsPath}</code>
          </div>
        </div>

        <div className="bg-white dark:bg-slate-900 border border-gray-200 dark:border-slate-800 rounded-xl p-4 shadow-sm">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-slate-400">
              {t('monitoring.auth')}
            </span>
            <Shield className="w-4 h-4 text-gray-400 dark:text-slate-500" />
          </div>
          <div className="mt-2 text-xl font-bold text-gray-900 dark:text-white">
            {metricsAuthEnabled ? 'Protected' : 'None (Open)'}
          </div>
          <div className="mt-2 text-xs text-gray-500 dark:text-slate-400">
            {metricsAuthEnabled ? 'Basic / Bearer Auth active' : 'Internal scrape network mode'}
          </div>
        </div>

        <div className="bg-white dark:bg-slate-900 border border-gray-200 dark:border-slate-800 rounded-xl p-4 shadow-sm">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-slate-400">
              {t('monitoring.systemChecks')}
            </span>
            <Layers className="w-4 h-4 text-blue-500" />
          </div>
          <div className="mt-2 text-xl font-bold text-gray-900 dark:text-white">
            {systemCount}
          </div>
          <div className="mt-2 text-xs text-gray-500 dark:text-slate-400">
            Persistent from Helm values
          </div>
        </div>

        <div className="bg-white dark:bg-slate-900 border border-gray-200 dark:border-slate-800 rounded-xl p-4 shadow-sm">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-slate-400">
              {t('monitoring.ephemeralChecks')}
            </span>
            <Clock className="w-4 h-4 text-amber-500" />
          </div>
          <div className="mt-2 text-xl font-bold text-gray-900 dark:text-white">
            {ephemeralCount} <span className="text-xs font-normal text-gray-400">({activeCount} enabled)</span>
          </div>
          <div className="mt-2 text-xs text-amber-600 dark:text-amber-400">
            Reset upon kncc restart
          </div>
        </div>
      </div>

      {/* Ephemeral Warning Banner */}
      <div className="bg-amber-50 dark:bg-amber-950/30 border-l-4 border-amber-500 p-4 rounded-r-xl shadow-sm">
        <div className="flex items-start">
          <div className="flex-shrink-0">
            <AlertTriangle className="h-5 w-5 text-amber-500" aria-hidden="true" />
          </div>
          <div className="ml-3">
            <h3 className="text-sm font-semibold text-amber-900 dark:text-amber-200">
              {t('monitoring.ephemeralWarningBanner')}
            </h3>
          </div>
        </div>
      </div>

      {/* Scheduled Checks Table */}
      <div className="bg-white dark:bg-slate-900 border border-gray-200 dark:border-slate-800 rounded-xl shadow-sm overflow-hidden">
        {loading ? (
          <div className="p-12 text-center text-gray-500 dark:text-slate-400">
            <Loader2 className="w-8 h-8 mx-auto animate-spin mb-3 text-red-600" />
            <p className="text-sm">{t('common.loading')}</p>
          </div>
        ) : error ? (
          <div className="p-8 text-center text-red-600 dark:text-red-400">
            <AlertTriangle className="w-8 h-8 mx-auto mb-2" />
            <p className="text-sm font-semibold">{error}</p>
          </div>
        ) : checks.length === 0 ? (
          <div className="p-12 text-center text-gray-500 dark:text-slate-400">
            <Server className="w-12 h-12 mx-auto text-gray-300 dark:text-slate-600 mb-3" />
            <h3 className="text-base font-semibold text-gray-900 dark:text-white">
              {t('monitoring.noChecksTitle')}
            </h3>
            <p className="text-sm mt-1 max-w-sm mx-auto">
              {t('monitoring.noChecksDesc')}
            </p>
            <button
              onClick={() => setIsModalOpen(true)}
              className="mt-4 inline-flex items-center px-4 py-2 text-sm font-medium rounded-lg text-white bg-red-600 hover:bg-red-700 shadow-sm"
            >
              <Plus className="w-4 h-4 mr-1.5" />
              {t('monitoring.newScheduledCheck')}
            </button>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200 dark:divide-slate-800 text-left text-sm">
              <thead className="bg-gray-50 dark:bg-slate-800/50 text-xs uppercase font-semibold text-gray-500 dark:text-slate-400 tracking-wider">
                <tr>
                  <th scope="col" className="px-6 py-3.5">
                    {t('monitoring.colName')}
                  </th>
                  <th scope="col" className="px-6 py-3.5">
                    {t('monitoring.colInterval')}
                  </th>
                  <th scope="col" className="px-6 py-3.5">
                    {t('monitoring.colMode')}
                  </th>
                  <th scope="col" className="px-6 py-3.5">
                    {t('monitoring.colTargets')}
                  </th>
                  <th scope="col" className="px-6 py-3.5">
                    {t('monitoring.colLastRun')}
                  </th>
                  <th scope="col" className="px-6 py-3.5 text-right">
                    {t('monitoring.colActions')}
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-slate-800">
                {checks.map((check) => {
                  const isRunningThis = runningId === check.id;
                  const isHealthy = check.last_report_status === 'completed';
                  const isDegraded = check.last_report_status === 'failed';
                  const statusTooltip = getReportStatusTooltip(check.last_report_status, t);

                  return (
                    <tr
                      key={check.id}
                      className="hover:bg-gray-50/80 dark:hover:bg-slate-800/40 transition"
                    >
                      <td className="px-6 py-4 whitespace-nowrap">
                        <div className="flex items-center space-x-3">
                          <div className="flex-shrink-0 cursor-help" title={statusTooltip}>
                            {check.enabled ? (
                              isHealthy ? (
                                <CheckCircle2 className="w-5 h-5 text-emerald-500" />
                              ) : isDegraded ? (
                                <XCircle className="w-5 h-5 text-red-500" />
                              ) : (
                                <Activity className="w-5 h-5 text-blue-500 animate-pulse" />
                              )
                            ) : (
                              <Pause className="w-5 h-5 text-gray-400" />
                            )}
                          </div>
                          <div>
                            <div className="flex items-center space-x-2">
                              <span className="font-semibold text-gray-900 dark:text-white">
                                {check.name}
                              </span>
                              {check.is_system ? (
                                <span className="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-semibold bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-300">
                                  {t('monitoring.systemBadge')}
                                </span>
                              ) : (
                                <span className="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-semibold bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300">
                                  {t('monitoring.ephemeralBadge')}
                                </span>
                              )}
                              {!check.enabled && (
                                <span className="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-semibold bg-gray-100 text-gray-600 dark:bg-slate-800 dark:text-slate-400">
                                  {t('monitoring.pausedBadge')}
                                </span>
                              )}
                            </div>
                            <span className="text-xs text-gray-400 dark:text-slate-500 font-mono">
                              ID: {check.id}
                            </span>
                          </div>
                        </div>
                      </td>

                      <td className="px-6 py-4 whitespace-nowrap">
                        <span className="inline-flex items-center px-2.5 py-1 rounded-md text-xs font-medium bg-gray-100 dark:bg-slate-800 text-gray-700 dark:text-slate-300 font-mono">
                          <Clock className="w-3.5 h-3.5 mr-1 text-gray-400" />
                          {check.interval}
                        </span>
                      </td>

                      <td className="px-6 py-4 whitespace-nowrap">
                        <span className="text-xs font-medium text-gray-700 dark:text-slate-300">
                          {check.mode === 'host'
                            ? t('monitoring.modeHostShort')
                            : `${t('monitoring.modePodShort')}${check.namespace ? ` (${check.namespace})` : ''}`}
                        </span>
                      </td>

                      <td className="px-6 py-4">
                        <div className="flex flex-col space-y-1 max-w-xs">
                          {check.targets.slice(0, 2).map((tgt, idx) => (
                            <span
                              key={idx}
                              className="text-xs font-mono text-gray-600 dark:text-slate-400 truncate"
                            >
                              <strong className="uppercase text-gray-800 dark:text-slate-200">{tgt.protocol}</strong> {tgt.host}{tgt.port ? `:${tgt.port}` : ''}{tgt.path || ''}
                            </span>
                          ))}
                          {check.targets.length > 2 && (
                            <span className="text-[11px] text-gray-400">
                              +{check.targets.length - 2} more...
                            </span>
                          )}
                        </div>
                      </td>

                      <td className="px-6 py-4 whitespace-nowrap text-xs text-gray-500 dark:text-slate-400">
                        {check.last_report_id ? (
                          <div className="flex items-center space-x-2.5">
                            <div className="flex-shrink-0 cursor-help" title={statusTooltip}>
                              {isHealthy ? (
                                <CheckCircle2 className="w-4 h-4 text-emerald-500" />
                              ) : isDegraded ? (
                                <XCircle className="w-4 h-4 text-red-500" />
                              ) : (
                                <Activity className="w-4 h-4 text-blue-500 animate-pulse" />
                              )}
                            </div>
                            <div className="space-y-0.5">
                              <div>
                                {check.last_run_at ? new Date(check.last_run_at).toLocaleTimeString() : t('monitoring.neverRun')}
                              </div>
                              <button
                                onClick={() => onSelectReport && check.last_report_id && onSelectReport(check.last_report_id)}
                                className="inline-flex items-center text-xs font-medium text-red-600 dark:text-red-400 hover:underline"
                              >
                                {t('monitoring.viewReport')}
                                <ExternalLink className="w-3 h-3 ml-1" />
                              </button>
                            </div>
                          </div>
                        ) : (
                          <div className="flex items-center space-x-2">
                            <span className="flex-shrink-0 cursor-help" title={t('monitoring.statusReportNever')}>
                              <Clock className="w-4 h-4 text-gray-400" />
                            </span>
                            <span className="italic text-gray-400">{t('monitoring.neverRun')}</span>
                          </div>
                        )}
                      </td>

                      <td className="px-6 py-4 whitespace-nowrap text-right text-xs font-medium space-x-2">
                        <button
                          onClick={() => handleRunNow(check)}
                          disabled={isRunningThis}
                          title={isRunningThis ? t('monitoring.running') : t('monitoring.runNow')}
                          className="inline-flex items-center justify-center p-2 rounded-lg border border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800 text-gray-700 dark:text-slate-200 hover:bg-gray-50 dark:hover:bg-slate-700 shadow-sm transition disabled:opacity-50"
                        >
                          {isRunningThis ? (
                            <Loader2 className="w-4 h-4 animate-spin text-emerald-600" />
                          ) : (
                            <Play className="w-4 h-4 text-emerald-600 fill-emerald-600" />
                          )}
                        </button>

                        <button
                          onClick={() => handleToggle(check)}
                          title={check.enabled ? t('monitoring.pause') : t('monitoring.resume')}
                          className="inline-flex items-center justify-center p-2 rounded-lg border border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800 text-gray-700 dark:text-slate-200 hover:bg-gray-50 dark:hover:bg-slate-700 shadow-sm transition"
                        >
                          {check.enabled ? (
                            <Pause className="w-4 h-4 text-amber-600" />
                          ) : (
                            <Play className="w-4 h-4 text-blue-600" />
                          )}
                        </button>

                        {!check.is_system && (
                          <button
                            onClick={() => handleDelete(check)}
                            title={t('common.delete')}
                            className="inline-flex items-center p-2 rounded-lg border border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800 text-red-600 hover:bg-red-50 dark:hover:bg-red-950/40 shadow-sm transition"
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Create Modal Component with isolated state */}
      <CreateScheduledCheckModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onSuccess={fetchChecks}
        defaultNamespace={config?.default_namespace || 'kncc'}
        allowedIntervals={allowedIntervals}
        namespaceListEnabled={!!config?.namespace_list_enabled}
        multiNamespaceAllowed={!!config?.multi_namespace_allowed}
      />
    </div>
  );
};

interface CreateScheduledCheckModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => Promise<void>;
  defaultNamespace: string;
  allowedIntervals: string[];
  namespaceListEnabled: boolean;
  multiNamespaceAllowed: boolean;
}

const CreateScheduledCheckModal: React.FC<CreateScheduledCheckModalProps> = ({
  isOpen,
  onClose,
  onSuccess,
  defaultNamespace,
  allowedIntervals,
  namespaceListEnabled,
  multiNamespaceAllowed,
}) => {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  const [interval, setInterval] = useState(allowedIntervals[0] || '1m');
  const [mode, setMode] = useState<CheckMode>('host');
  const [namespace, setNamespace] = useState(defaultNamespace || 'kncc');
  const [availableNamespaces, setAvailableNamespaces] = useState<string[]>([]);
  const [loadingNamespaces, setLoadingNamespaces] = useState(false);
  const [namespacesError, setNamespacesError] = useState<string | null>(null);
  const [isManualInput, setIsManualInput] = useState(false);
  const [targets, setTargets] = useState<TargetService[]>([
    { protocol: 'http', host: '', port: 80, path: '/', timeout_seconds: 5 },
  ]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (isOpen) {
      setName('');
      setInterval(allowedIntervals[0] || '1m');
      setMode('host');
      setNamespace(defaultNamespace || 'kncc');
      setTargets([{ protocol: 'http', host: '', port: 80, path: '/', timeout_seconds: 5 }]);
      setError(null);
      setIsManualInput(false);
    }
  }, [isOpen, defaultNamespace, allowedIntervals]);

  useEffect(() => {
    if (isOpen && mode === 'pod' && namespaceListEnabled && multiNamespaceAllowed) {
      setLoadingNamespaces(true);
      setNamespacesError(null);
      api.getNamespaces()
        .then((res) => {
          const list = res.namespaces || [];
          setAvailableNamespaces(list);
          if (list.length > 0 && (!namespace || namespace === 'kncc')) {
            setNamespace(list[0]);
          }
        })
        .catch((err) => {
          setNamespacesError(err.message || 'Unable to list cluster namespaces');
          setIsManualInput(true);
        })
        .finally(() => {
          setLoadingNamespaces(false);
        });
    }
  }, [isOpen, mode, namespaceListEnabled, multiNamespaceAllowed]);

  if (!isOpen) return null;

  const handleAddTarget = () => {
    setTargets((prev) => [
      ...prev,
      { protocol: 'http', host: '', port: 80, path: '/', timeout_seconds: 5 },
    ]);
  };

  const handleRemoveTarget = (index: number) => {
    if (targets.length <= 1) return;
    setTargets((prev) => prev.filter((_, i) => i !== index));
  };

  const handleTargetChange = (index: number, field: keyof TargetService, val: any) => {
    setTargets((prev) => {
      const updated = [...prev];
      updated[index] = { ...updated[index], [field]: val };

      if (field === 'protocol') {
        const proto = val as Protocol;
        if (proto === 'icmp') {
          delete updated[index].port;
          delete updated[index].path;
          delete updated[index].insecure_tls;
        } else if (proto === 'tcp' || proto === 'udp') {
          delete updated[index].path;
          delete updated[index].insecure_tls;
          if (!updated[index].port) {
            updated[index].port = proto === 'tcp' ? 80 : 53;
          }
        } else if (proto === 'http') {
          delete updated[index].insecure_tls;
          if (!updated[index].port || updated[index].port === 443) {
            updated[index].port = 80;
          }
        } else if (proto === 'https') {
          if (!updated[index].port || updated[index].port === 80) {
            updated[index].port = 443;
          }
        }
      }
      return updated;
    });
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setError('Check name is required');
      return;
    }
    for (const tgt of targets) {
      if (!tgt.host.trim()) {
        setError('Host is required for all targets');
        return;
      }
      if (tgt.protocol !== 'icmp') {
        if (!tgt.port || tgt.port < 1 || tgt.port > 65535) {
          setError('Valid port (1-65535) is required');
          return;
        }
      }
    }

    setSubmitting(true);
    setError(null);

    const sanitizedTargets: TargetService[] = targets.map((tgt) => {
      const item: TargetService = {
        name: tgt.name?.trim() || undefined,
        protocol: tgt.protocol,
        host: tgt.host.trim(),
        timeout_seconds: tgt.timeout_seconds || 5,
      };
      if (tgt.protocol !== 'icmp') {
        item.port = tgt.port || (tgt.protocol === 'https' ? 443 : 80);
      }
      if (tgt.protocol === 'http' || tgt.protocol === 'https') {
        item.path = tgt.path?.trim() ? (tgt.path.startsWith('/') ? tgt.path.trim() : `/${tgt.path.trim()}`) : '/';
      }
      if (tgt.protocol === 'https' && tgt.insecure_tls) {
        item.insecure_tls = true;
      }
      return item;
    });

    try {
      await api.createScheduledCheck({
        name: name.trim(),
        interval,
        mode,
        namespace: mode === 'pod' ? namespace.trim() : undefined,
        targets: sanitizedTargets,
      });
      await onSuccess();
      onClose();
    } catch (err: any) {
      setError(err.message || 'Failed to create check');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 overflow-y-auto bg-black/50 backdrop-blur-sm flex items-center justify-center p-4">
      <div className="bg-white dark:bg-slate-900 border border-gray-200 dark:border-slate-800 rounded-2xl max-w-2xl w-full p-6 shadow-xl space-y-5">
        <div className="flex items-center justify-between pb-3 border-b border-gray-200 dark:border-slate-800">
          <h2 className="text-lg font-bold text-gray-900 dark:text-white flex items-center space-x-2">
            <Activity className="w-5 h-5 text-red-600" />
            <span>{t('monitoring.createModalTitle')}</span>
          </h2>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600 dark:hover:text-slate-300"
          >
            ✕
          </button>
        </div>

        <div className="bg-amber-50 dark:bg-amber-950/40 border-l-4 border-amber-500 p-3 rounded-r-lg text-xs text-amber-800 dark:text-amber-200">
          <div className="flex items-start space-x-2">
            <AlertTriangle className="w-4 h-4 text-amber-500 flex-shrink-0 mt-0.5" />
            <span>{t('monitoring.ephemeralModalWarning')}</span>
          </div>
        </div>

        {error && (
          <div className="bg-red-50 dark:bg-red-950/40 border border-red-200 dark:border-red-900 p-3 rounded-lg text-xs text-red-600 dark:text-red-400 font-semibold">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-semibold text-gray-700 dark:text-slate-300 uppercase mb-1">
              {t('monitoring.nameLabel')} *
            </label>
            <input
              type="text"
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('monitoring.namePlaceholder')}
              className="w-full px-3 py-2 border border-gray-300 dark:border-slate-700 rounded-lg text-sm bg-white dark:bg-slate-800 text-gray-900 dark:text-white focus:ring-2 focus:ring-red-500"
            />
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-semibold text-gray-700 dark:text-slate-300 uppercase mb-1">
                {t('monitoring.intervalLabel')}
              </label>
              <select
                value={interval}
                onChange={(e) => setInterval(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 dark:border-slate-700 rounded-lg text-sm bg-white dark:bg-slate-800 text-gray-900 dark:text-white"
              >
                {allowedIntervals.map((inv) => (
                  <option key={inv} value={inv}>
                    {inv}
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label className="block text-xs font-semibold text-gray-700 dark:text-slate-300 uppercase mb-1">
                {t('monitoring.modeLabel')}
              </label>
              <select
                value={mode}
                onChange={(e) => setMode(e.target.value as CheckMode)}
                className="w-full px-3 py-2 border border-gray-300 dark:border-slate-700 rounded-lg text-sm bg-white dark:bg-slate-800 text-gray-900 dark:text-white"
              >
                <option value="host">{t('monitoring.hostMode')}</option>
                <option value="pod">{t('monitoring.podMode')}</option>
              </select>
            </div>
          </div>

          {mode === 'pod' && (
            <div>
              <div className="flex justify-between items-center mb-1">
                <label className="block text-xs font-semibold text-gray-700 dark:text-slate-300 uppercase">
                  {t('monitoring.namespaceLabel')}
                </label>
                {namespaceListEnabled && multiNamespaceAllowed && availableNamespaces.length > 0 && (
                  <button
                    type="button"
                    onClick={() => setIsManualInput(!isManualInput)}
                    className="text-xs text-red-600 dark:text-red-400 hover:text-red-700 flex items-center font-medium"
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
              ) : namespaceListEnabled && multiNamespaceAllowed && availableNamespaces.length > 0 && !isManualInput ? (
                <select
                  value={namespace}
                  onChange={(e) => setNamespace(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 dark:border-slate-700 rounded-lg text-sm bg-white dark:bg-slate-800 text-gray-900 dark:text-white font-mono"
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
                  className="w-full px-3 py-2 border border-gray-300 dark:border-slate-700 rounded-lg text-sm bg-white dark:bg-slate-800 text-gray-900 dark:text-white font-mono"
                />
              )}

              {namespacesError && (
                <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
                  {namespacesError} (manual input active)
                </p>
              )}
            </div>
          )}

          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="text-xs font-semibold text-gray-700 dark:text-slate-300 uppercase">
                {t('monitoring.targetsLabel')}
              </label>
              <button
                type="button"
                onClick={handleAddTarget}
                className="text-xs text-red-600 dark:text-red-400 font-semibold hover:underline inline-flex items-center"
              >
                <Plus className="w-3.5 h-3.5 mr-0.5" />
                {t('monitoring.addTarget')}
              </button>
            </div>

            <div className="space-y-2 max-h-56 overflow-y-auto pr-1">
              {targets.map((target, idx) => (
                <div
                  key={idx}
                  className="flex items-center space-x-2 bg-gray-50 dark:bg-slate-800/60 p-2 rounded-lg border border-gray-200 dark:border-slate-700 text-xs"
                >
                  <select
                    value={target.protocol}
                    onChange={(e) => handleTargetChange(idx, 'protocol', e.target.value as Protocol)}
                    className="px-2 py-1.5 border border-gray-300 dark:border-slate-700 rounded bg-white dark:bg-slate-800 uppercase font-semibold text-gray-900 dark:text-white"
                  >
                    <option value="http">HTTP</option>
                    <option value="https">HTTPS</option>
                    <option value="tcp">TCP</option>
                    <option value="udp">UDP</option>
                    <option value="icmp">ICMP</option>
                  </select>

                  <input
                    type="text"
                    placeholder="Host / IP / FQDN"
                    required
                    value={target.host}
                    onChange={(e) => handleTargetChange(idx, 'host', e.target.value)}
                    className="flex-1 px-2.5 py-1.5 border border-gray-300 dark:border-slate-700 rounded bg-white dark:bg-slate-800 text-gray-900 dark:text-white"
                  />

                  {target.protocol !== 'icmp' && (
                    <input
                      type="number"
                      placeholder="Port"
                      value={target.port || ''}
                      onChange={(e) => handleTargetChange(idx, 'port', parseInt(e.target.value) || 0)}
                      className="w-16 px-2 py-1.5 border border-gray-300 dark:border-slate-700 rounded bg-white dark:bg-slate-800 text-gray-900 dark:text-white"
                    />
                  )}

                  {(target.protocol === 'http' || target.protocol === 'https') && (
                    <input
                      type="text"
                      placeholder="Path (/)"
                      value={target.path || ''}
                      onChange={(e) => handleTargetChange(idx, 'path', e.target.value)}
                      className="w-28 px-2 py-1.5 border border-gray-300 dark:border-slate-700 rounded bg-white dark:bg-slate-800 text-gray-900 dark:text-white"
                    />
                  )}

                  <button
                    type="button"
                    onClick={() => handleRemoveTarget(idx)}
                    disabled={targets.length <= 1}
                    className="p-1 text-gray-400 hover:text-red-600 disabled:opacity-30"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              ))}
            </div>
          </div>

          <div className="flex items-center justify-end space-x-3 pt-3 border-t border-gray-200 dark:border-slate-800">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 text-sm font-medium rounded-lg text-gray-700 dark:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-800"
            >
              {t('common.cancel')}
            </button>
            <button
              type="submit"
              disabled={submitting}
              className="inline-flex items-center px-4 py-2 text-sm font-medium rounded-lg text-white bg-red-600 hover:bg-red-700 shadow-sm disabled:opacity-50"
            >
              {submitting ? (
                <>
                  <Loader2 className="w-4 h-4 mr-1.5 animate-spin" />
                  {t('monitoring.creating')}
                </>
              ) : (
                t('common.confirm')
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
