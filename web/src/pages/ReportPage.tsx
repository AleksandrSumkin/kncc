import React, { useEffect, useState, useMemo } from 'react';
import { api } from '../api';
import { useTranslation } from '../context/LanguageContext';
import { Report, NodeResult, ProbeOutput, TargetService, CheckMode } from '../types';
import { copyToClipboard } from '../utils/clipboard';
import { formatDuration } from '../utils/format';
export { formatDuration };
import {
  CheckCircle,
  XCircle,
  AlertTriangle,
  Clock,
  RotateCw,
  Share2,
  Server,
  Network,
  Activity,
  Check,
  Search,
  ChevronDown,
  ChevronUp,
  Target,
  Globe,
  ArrowLeft,
  Home,
  ShieldCheck,
  ShieldAlert,
  X,
} from 'lucide-react';

interface ReportPageProps {
  reportId: string;
  onRerunCreated: (newReportId: string) => void;
  onBackToEdit?: (data: {
    fromReportId: string;
    mode: CheckMode;
    namespace: string;
    targets: TargetService[];
  }) => void;
  onNavigateHome?: () => void;
}

export interface TargetNodeStatus {
  node_name: string;
  node_ip?: string;
  node_status: string; // 'completed' | 'failed' | 'timeout' | 'running'
  node_error?: string;
  probe?: ProbeOutput;
  success: boolean;
  latency_ms: number;
  resolved_ip?: string;
  http_status_code?: number;
  error?: string;
  details?: string;
  cert_expiry?: string;
  cert_days_left?: number;
  cert_status?: 'valid' | 'expiring_soon' | 'expired';
  cert_subject?: string;
  cert_issuer?: string;
}

export interface TargetGroup {
  key: string;
  target: TargetService;
  displayLabel: string;
  protocolBadge: string;
  totalNodes: number;
  completedNodes: number;
  successfulNodes: number;
  failedNodes: number;
  runningNodes: number;
  avgLatencyMs: number;
  overallStatus: 'all_ok' | 'partial' | 'all_failed' | 'running';
  certStatus?: 'valid' | 'expiring_soon' | 'expired';
  certExpiry?: string;
  certDaysLeft?: number;
  nodes: TargetNodeStatus[];
}

export const ReportPage: React.FC<ReportPageProps> = ({ reportId, onRerunCreated, onBackToEdit, onNavigateHome }) => {
  const { t } = useTranslation();
  const [report, setReport] = useState<Report | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const [rerunning, setRerunning] = useState(false);

  const handleBackToEdit = () => {
    if (!report || !onBackToEdit) return;
    const targetsToPrefill =
      report.targets && report.targets.length > 0
        ? report.targets
        : targetGroups.map((tg) => tg.target);

    onBackToEdit({
      fromReportId: report.id,
      mode: report.mode,
      namespace: report.namespace,
      targets: JSON.parse(JSON.stringify(targetsToPrefill)),
    });
  };

  // View state: 'target' (default) or 'node'
  const [viewMode, setViewMode] = useState<'target' | 'node'>('target');
  const [targetFilter, setTargetFilter] = useState<'all' | 'issues' | 'success'>('all');
  const [nodeFilter, setNodeFilter] = useState<'all' | 'failed' | 'success'>('all');
  const [searchTerm, setSearchTerm] = useState('');
  const [expandedTargets, setExpandedTargets] = useState<Record<string, boolean>>({});

  const fetchReport = async () => {
    try {
      const data = await api.getReport(reportId);
      setReport(data);
      setError(null);
    } catch (err: any) {
      setError(err.message || 'Failed to load report');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchReport();

    // Live polling while report is pending or running
    const interval = setInterval(() => {
      if (report?.status === 'pending' || report?.status === 'running') {
        fetchReport();
      }
    }, 1500);

    return () => clearInterval(interval);
  }, [reportId, report?.status]);

  const handleCopyLink = async () => {
    const shareUrl = window.location.href;
    const ok = await copyToClipboard(shareUrl);
    if (ok) {
      setCopied(true);
      setToastMessage('Report link copied to clipboard');
      setTimeout(() => setCopied(false), 2500);
      setTimeout(() => setToastMessage(null), 3500);
    } else {
      window.prompt('Copy report link to clipboard:', shareUrl);
    }
  };

  const handleRerun = async () => {
    setRerunning(true);
    try {
      const newReport = await api.rerunReport(reportId);
      onRerunCreated(newReport.id);
    } catch (err: any) {
      alert('Rerun failed: ' + err.message);
    } finally {
      setRerunning(false);
    }
  };

  const isRunning = report?.status === 'pending' || report?.status === 'running';
  const nodeResultsList: NodeResult[] = useMemo(() => {
    return Object.values(report?.node_results || {});
  }, [report?.node_results]);

  // Transform results into target-centric groups
  const targetGroups: TargetGroup[] = useMemo(() => {
    if (!report) return [];

    // Gather unique targets
    const targetMap = new Map<string, TargetService>();
    (report.targets || []).forEach((t) => {
      targetMap.set(formatTargetKey(t), t);
    });

    // Also collect from service_results in case targets array is missing in legacy reports
    nodeResultsList.forEach((nr) => {
      nr.service_results?.forEach((sr) => {
        if (sr.target) {
          const key = formatTargetKey(sr.target);
          if (!targetMap.has(key)) {
            targetMap.set(key, sr.target);
          }
        }
      });
    });

    const groups: TargetGroup[] = [];

    targetMap.forEach((target, targetKey) => {
      const displayLabel = formatTargetDisplayLabel(target);
      const protocolBadge = target.protocol.toUpperCase();

      const nodes: TargetNodeStatus[] = [];
      let totalLatency = 0;
      let latencyCount = 0;
      let successfulNodes = 0;
      let failedNodes = 0;
      let runningNodes = 0;
      let completedNodes = 0;

      nodeResultsList.forEach((nr) => {
        const matchingProbe = nr.service_results?.find(
          (sr) => formatTargetKey(sr.target) === targetKey
        );

        if (nr.status === 'running' && !matchingProbe) {
          runningNodes++;
          nodes.push({
            node_name: nr.node_name,
            node_ip: nr.node_ip,
            node_status: 'running',
            success: false,
            latency_ms: 0,
            details: 'Probe running on node...',
          });
          return;
        }

        if (matchingProbe) {
          completedNodes++;
          if (matchingProbe.success) {
            successfulNodes++;
            if (matchingProbe.latency_ms > 0) {
              totalLatency += matchingProbe.latency_ms;
              latencyCount++;
            }
          } else {
            failedNodes++;
          }

          const probeError = matchingProbe.error?.trim();
          const nodeError = nr.status !== 'completed' && nr.error ? nr.error.trim() : undefined;
          const effectiveError = !matchingProbe.success ? (probeError || nodeError || 'Check failed') : undefined;
          const effectiveDetails = matchingProbe.details?.trim();

          nodes.push({
            node_name: nr.node_name,
            node_ip: nr.node_ip,
            node_status: nr.status,
            node_error: nodeError,
            probe: matchingProbe,
            success: matchingProbe.success,
            latency_ms: matchingProbe.latency_ms,
            resolved_ip: matchingProbe.resolved_ip,
            http_status_code: matchingProbe.http_status_code,
            error: effectiveError,
            details: effectiveDetails,
            cert_expiry: matchingProbe.cert_expiry,
            cert_days_left: matchingProbe.cert_days_left,
            cert_status: matchingProbe.cert_status,
            cert_subject: matchingProbe.cert_subject,
            cert_issuer: matchingProbe.cert_issuer,
          });
        } else {
          // Node finished or failed, but target probe was not reported
          completedNodes++;
          failedNodes++;
          const nodeErr = nr.error?.trim() || 'No result returned from probe pod';
          nodes.push({
            node_name: nr.node_name,
            node_ip: nr.node_ip,
            node_status: nr.status,
            node_error: nodeErr,
            success: false,
            latency_ms: 0,
            error: nodeErr,
          });
        }
      });

      const avgLatencyMs = latencyCount > 0 ? Math.round(totalLatency / latencyCount) : 0;
      let overallStatus: 'all_ok' | 'partial' | 'all_failed' | 'running' = 'all_ok';

      if (runningNodes > 0 && successfulNodes === 0 && failedNodes === 0) {
        overallStatus = 'running';
      } else if (failedNodes > 0 && successfulNodes > 0) {
        overallStatus = 'partial';
      } else if (failedNodes > 0 && successfulNodes === 0) {
        overallStatus = 'all_failed';
      } else if (successfulNodes > 0 && failedNodes === 0) {
        overallStatus = 'all_ok';
      } else {
        overallStatus = isRunning ? 'running' : 'partial';
      }

      // Aggregate certificate status for HTTPS targets
      let certStatus: 'valid' | 'expiring_soon' | 'expired' | undefined;
      let certExpiry: string | undefined;
      let certDaysLeft: number | undefined;

      if (target.protocol === 'https') {
        for (const n of nodes) {
          if (n.cert_status === 'expired') {
            certStatus = 'expired';
            certExpiry = n.cert_expiry;
            certDaysLeft = n.cert_days_left;
            break; // expired is highest severity
          } else if (n.cert_status === 'expiring_soon' && certStatus !== 'expired') {
            certStatus = 'expiring_soon';
            certExpiry = n.cert_expiry;
            certDaysLeft = n.cert_days_left;
          } else if (n.cert_status === 'valid' && !certStatus) {
            certStatus = 'valid';
            certExpiry = n.cert_expiry;
            certDaysLeft = n.cert_days_left;
          }
        }
      }

      groups.push({
        key: targetKey,
        target,
        displayLabel,
        protocolBadge,
        totalNodes: nodeResultsList.length,
        completedNodes,
        successfulNodes,
        failedNodes,
        runningNodes,
        avgLatencyMs,
        overallStatus,
        certStatus,
        certExpiry,
        certDaysLeft,
        nodes,
      });
    });

    return groups;
  }, [report, nodeResultsList, isRunning]);

  // Filter targets based on search and status
  const filteredTargets = useMemo(() => {
    return targetGroups.filter((tg) => {
      if (searchTerm) {
        const query = searchTerm.toLowerCase();
        const matchesTarget =
          tg.key.toLowerCase().includes(query) ||
          tg.target.host.toLowerCase().includes(query) ||
          tg.target.protocol.toLowerCase().includes(query) ||
          (tg.target.port && tg.target.port.toString().includes(query));
        const matchesNode = tg.nodes.some(
          (n) =>
            n.node_name.toLowerCase().includes(query) ||
            (n.node_ip && n.node_ip.includes(query)) ||
            (n.error && n.error.toLowerCase().includes(query))
        );
        if (!matchesTarget && !matchesNode) return false;
      }

      if (targetFilter === 'issues') {
        return tg.overallStatus === 'partial' || tg.overallStatus === 'all_failed';
      }
      if (targetFilter === 'success') {
        return tg.overallStatus === 'all_ok';
      }
      return true;
    });
  }, [targetGroups, searchTerm, targetFilter]);

  // By-node filtering
  const filteredNodes = useMemo(() => {
    return nodeResultsList.filter((n) => {
      if (searchTerm && !n.node_name.toLowerCase().includes(searchTerm.toLowerCase())) {
        return false;
      }
      const hasFailures = n.service_results?.some((s) => !s.success) || n.status !== 'completed';
      if (nodeFilter === 'failed') return hasFailures;
      if (nodeFilter === 'success') return !hasFailures;
      return true;
    });
  }, [nodeResultsList, searchTerm, nodeFilter]);

  // Overall metric computations
  const { totalTargetsChecked, successfulTargets, avgLatency } = useMemo(() => {
    let checked = 0;
    let ok = 0;
    let latencySum = 0;
    let latencyCnt = 0;

    nodeResultsList.forEach((nr) => {
      nr.service_results?.forEach((sr) => {
        checked++;
        if (sr.success) {
          ok++;
          if (sr.latency_ms > 0) {
            latencySum += sr.latency_ms;
            latencyCnt++;
          }
        }
      });
    });

    return {
      totalTargetsChecked: checked,
      successfulTargets: ok,
      avgLatency: latencyCnt > 0 ? Math.round(latencySum / latencyCnt) : 0,
    };
  }, [nodeResultsList]);

  // Toggle card expansion (collapsed by default)
  const toggleExpand = (key: string) => {
    setExpandedTargets((prev) => ({
      ...prev,
      [key]: !prev[key],
    }));
  };

  const isTargetExpanded = (key: string) => {
    // Default to collapsed for clean high-level overview
    return !!expandedTargets[key];
  };

  const setAllExpanded = (expanded: boolean) => {
    const updated: Record<string, boolean> = {};
    targetGroups.forEach((tg) => {
      updated[tg.key] = expanded;
    });
    setExpandedTargets(updated);
  };

  if (loading && !report) {
    return (
      <div className="min-h-[60vh] flex flex-col items-center justify-center">
        <div className="w-10 h-10 border-4 border-red-600 border-t-transparent rounded-full animate-spin mb-4" />
        <p className="text-sm text-gray-500 font-medium">Loading report results...</p>
      </div>
    );
  }

  if (error || !report) {
    return (
      <div className="max-w-3xl mx-auto py-12 px-4 text-center">
        <XCircle className="w-12 h-12 text-red-500 mx-auto mb-3" />
        <h2 className="text-xl font-bold text-gray-900 mb-2">Report Error</h2>
        <p className="text-sm text-gray-600 mb-6">{error || 'Report not found'}</p>
        <div className="flex items-center justify-center space-x-3">
          <button
            onClick={onNavigateHome ? onNavigateHome : () => { window.location.href = '/'; }}
            className="inline-flex items-center px-4 py-2 border border-gray-300 text-gray-700 bg-white text-sm font-medium rounded-lg hover:bg-gray-50 focus:outline-none transition-colors"
          >
            <Home className="w-4 h-4 mr-1.5" />
            Home
          </button>
          <button
            onClick={fetchReport}
            className="inline-flex items-center px-4 py-2 bg-gray-900 text-white text-sm font-medium rounded-lg hover:bg-gray-800 transition-colors"
          >
            <RotateCw className="w-4 h-4 mr-1.5" />
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="max-w-7xl mx-auto py-8 px-4 sm:px-6 lg:px-8">
      {/* Top Header Card */}
      <div className="bg-white dark:bg-slate-900 rounded-xl shadow-sm border border-gray-200 dark:border-slate-800 p-6 mb-6 transition-colors">
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-6 border-b border-gray-100 dark:border-slate-800">
          <div>
            <div className="flex flex-wrap items-center gap-3 mb-1">
              <h1 className="text-2xl font-bold text-gray-900 dark:text-white tracking-tight">
                {t('report.title')} #{report.id.slice(0, 8)}
              </h1>
              {getStatusBadge(report.status, t)}
              {getCheckModeBadge(report.mode, report.namespace, true, t)}
            </div>
            <p className="text-xs text-gray-500 dark:text-slate-400 flex items-center space-x-2">
              <span>{t('report.initiatedBy')}: <strong className="text-gray-700 dark:text-slate-300">{report.username}</strong></span>
              <span>•</span>
              <span className="flex items-center">
                <Clock className="w-3.5 h-3.5 mr-1 text-gray-400 dark:text-slate-500" />
                {new Date(report.created_at).toLocaleString()}
              </span>
              <span>•</span>
              <span className="flex items-center">
                <Network className="w-3.5 h-3.5 mr-1 text-gray-400 dark:text-slate-500" />
                {t('report.modeLabel')}: <strong className="ml-1 text-gray-700 dark:text-slate-300 uppercase">{report.mode}</strong>
                {report.mode === 'pod' && ` (ns: ${report.namespace})`}
              </span>
            </p>
          </div>

          <div className="flex items-center space-x-3">
            <button
              onClick={handleCopyLink}
              className={`inline-flex items-center px-3.5 py-2 border rounded-lg text-xs font-semibold shadow-sm transition-all ${
                copied
                  ? 'border-emerald-300 dark:border-emerald-700 bg-emerald-50 dark:bg-emerald-950/40 text-emerald-700 dark:text-emerald-300 ring-2 ring-emerald-200 dark:ring-emerald-800'
                  : 'border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-gray-700 dark:text-slate-200 hover:bg-gray-50 dark:hover:bg-slate-800'
              }`}
            >
              {copied ? (
                <>
                  <Check className="w-3.5 h-3.5 mr-1.5 text-emerald-600 dark:text-emerald-400" />
                  <span className="text-emerald-700 dark:text-emerald-300">{t('common.copied')}</span>
                </>
              ) : (
                <>
                  <Share2 className="w-3.5 h-3.5 mr-1.5 text-gray-500 dark:text-slate-400" />
                  <span>{t('report.share')}</span>
                </>
              )}
            </button>

            {onBackToEdit && (
              <button
                onClick={handleBackToEdit}
                className="inline-flex items-center px-3.5 py-2 border border-gray-300 dark:border-slate-700 rounded-lg text-xs font-semibold text-gray-700 dark:text-slate-200 bg-white dark:bg-slate-900 hover:bg-gray-50 dark:hover:bg-slate-800 shadow-sm transition-all hover:border-gray-400 dark:hover:border-slate-600"
                title="Edit these parameters in a new check"
              >
                <ArrowLeft className="w-3.5 h-3.5 mr-1.5 text-gray-500 dark:text-slate-400" />
                <span>{t('report.editCheck')}</span>
              </button>
            )}

            <button
              onClick={handleRerun}
              disabled={rerunning}
              className="inline-flex items-center px-4 py-2 border border-transparent rounded-lg text-xs font-semibold text-white shadow-sm transition-all disabled:opacity-50"
              style={{ backgroundColor: 'var(--color-accent, #dc2626)' }}
            >
              <RotateCw className={`w-3.5 h-3.5 mr-1.5 ${rerunning ? 'animate-spin' : ''}`} />
              <span>{rerunning ? t('report.rerunning') : t('report.rerun')}</span>
            </button>
          </div>
        </div>

        {/* Progress Bar (when running) */}
        {isRunning && (
          <div className="pt-4">
            <div className="flex justify-between items-center text-xs text-gray-600 dark:text-slate-400 mb-1">
              <span className="font-medium flex items-center">
                <div className="w-2.5 h-2.5 rounded-full bg-red-600 animate-ping mr-2" />
                {t('report.running')}
              </span>
              <span className="font-semibold text-gray-800 dark:text-slate-200">
                {report.completed_nodes} / {report.total_nodes} {t('report.totalNodes').toLowerCase()}
              </span>
            </div>
            <div className="w-full bg-gray-100 dark:bg-slate-800 rounded-full h-2 overflow-hidden">
              <div
                className="bg-red-600 h-2 transition-all duration-500 rounded-full"
                style={{
                  width: `${report.total_nodes > 0 ? (report.completed_nodes / report.total_nodes) * 100 : 10}%`,
                }}
              />
            </div>
          </div>
        )}

        {/* Metrics Grid */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mt-6">
          <div className="p-3.5 bg-gray-50 dark:bg-slate-800/60 rounded-lg border border-gray-100 dark:border-slate-700/60">
            <div className="text-xs text-gray-500 dark:text-slate-400 uppercase font-semibold">{t('report.totalNodes')}</div>
            <div className="text-2xl font-bold text-gray-900 dark:text-white mt-1">
              {report.completed_nodes} / {report.total_nodes || nodeResultsList.length}
            </div>
          </div>

          <div className="p-3.5 bg-gray-50 dark:bg-slate-800/60 rounded-lg border border-gray-100 dark:border-slate-700/60">
            <div className="text-xs text-gray-500 dark:text-slate-400 uppercase font-semibold">{t('report.passedNodes')}</div>
            <div className="text-2xl font-bold text-green-600 dark:text-green-400 mt-1">
              {successfulTargets} / {totalTargetsChecked}
            </div>
          </div>

          <div className="p-3.5 bg-gray-50 dark:bg-slate-800/60 rounded-lg border border-gray-100 dark:border-slate-700/60">
            <div className="text-xs text-gray-500 dark:text-slate-400 uppercase font-semibold">{t('report.failedNodes')}</div>
            <div className={`text-2xl font-bold mt-1 ${totalTargetsChecked - successfulTargets > 0 ? 'text-red-600 dark:text-red-400' : 'text-gray-900 dark:text-white'}`}>
              {totalTargetsChecked - successfulTargets}
            </div>
          </div>

          <div className="p-3.5 bg-gray-50 dark:bg-slate-800/60 rounded-lg border border-gray-100 dark:border-slate-700/60">
            <div className="text-xs text-gray-500 dark:text-slate-400 uppercase font-semibold">{t('report.avgLatency')}</div>
            <div className="text-2xl font-bold text-gray-900 dark:text-white mt-1">
              {avgLatency > 0 ? `${avgLatency} ms` : '—'}
            </div>
          </div>
        </div>
      </div>

      {/* Row 1: Back Button, Mode Switch, Section Title (Full-width row) */}
      <div className="flex flex-wrap items-center gap-3.5 mb-3.5 pb-3 border-b border-gray-200 dark:border-slate-800">
        {onBackToEdit && (
          <button
            onClick={handleBackToEdit}
            className="inline-flex items-center px-3 py-1.5 border border-gray-300 dark:border-slate-700 rounded-lg text-xs font-semibold text-gray-700 dark:text-slate-200 bg-white dark:bg-slate-900 hover:bg-gray-50 dark:hover:bg-slate-800 hover:text-gray-900 dark:hover:text-white shadow-sm transition-all hover:border-gray-400 dark:hover:border-slate-600"
            title="Return to check form with these parameters pre-filled to fix typos or adjust targets"
          >
            <ArrowLeft className="w-3.5 h-3.5 mr-1.5 text-red-600 dark:text-red-400" />
            <span>{t('report.backToEdit')}</span>
          </button>
        )}

        {/* View Mode Toggle Button */}
        <div className="inline-flex bg-gray-200/90 dark:bg-slate-800 p-0.5 rounded-lg text-xs font-semibold shadow-inner">
          <button
            onClick={() => setViewMode('target')}
            className={`flex items-center px-3 py-1.5 rounded-md transition-all ${
              viewMode === 'target'
                ? 'bg-white dark:bg-slate-700 text-gray-900 dark:text-white shadow-sm'
                : 'text-gray-600 dark:text-slate-400 hover:text-gray-900 dark:hover:text-white'
            }`}
          >
            <Target className="w-3.5 h-3.5 mr-1.5 text-red-600 dark:text-red-400" />
            {t('report.byTarget')}
          </button>
          <button
            onClick={() => setViewMode('node')}
            className={`flex items-center px-3 py-1.5 rounded-md transition-all ${
              viewMode === 'node'
                ? 'bg-white dark:bg-slate-700 text-gray-900 dark:text-white shadow-sm'
                : 'text-gray-600 dark:text-slate-400 hover:text-gray-900 dark:hover:text-white'
            }`}
          >
            <Server className="w-3.5 h-3.5 mr-1.5 text-gray-600 dark:text-slate-300" />
            {t('report.byNode')}
          </button>
        </div>

        {/* Check Execution Mode Badge */}
        {getCheckModeBadge(report.mode, report.namespace, false, t)}

        <h2 className="text-lg font-bold text-gray-900 dark:text-white flex items-center">
          {viewMode === 'target' ? (
            <>
              <Target className="w-5 h-5 mr-2 text-red-600 dark:text-red-400" />
              {t('report.targetResultsHeader')} ({filteredTargets.length})
            </>
          ) : (
            <>
              <Server className="w-5 h-5 mr-2 text-gray-600 dark:text-slate-300" />
              {t('report.nodeResultsHeader')} ({filteredNodes.length})
            </>
          )}
        </h2>
      </div>

      {/* Row 2: Search Input, Status Filters, Expand/Collapse (Full-width row) */}
      <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 mb-5">
        {/* Search Input */}
        <div className="relative flex-1 sm:max-w-md">
          <Search className="w-3.5 h-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-slate-500" />
          <input
            type="text"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            placeholder={viewMode === 'target' ? t('report.searchTargetPlaceholder') : t('report.searchNodePlaceholder')}
            className="w-full pl-8 pr-3 py-1.5 bg-white dark:bg-slate-800 border border-gray-200 dark:border-slate-700 rounded-lg text-xs text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-slate-500 focus:ring-red-500 focus:border-red-500 shadow-sm"
          />
        </div>

        <div className="flex flex-wrap items-center justify-between sm:justify-end gap-3">
          {/* Filter Pills based on active view mode */}
          {viewMode === 'target' ? (
            <div className="flex bg-gray-200/80 dark:bg-slate-800 p-0.5 rounded-lg text-xs font-medium">
              <button
                onClick={() => setTargetFilter('all')}
                className={`px-2.5 py-1 rounded-md transition-all ${
                  targetFilter === 'all'
                    ? 'bg-white dark:bg-slate-700 shadow text-gray-900 dark:text-white'
                    : 'text-gray-600 dark:text-slate-400 hover:text-gray-900 dark:hover:text-white'
                }`}
              >
                {t('report.filterAll')}
              </button>
              <button
                onClick={() => setTargetFilter('issues')}
                className={`px-2.5 py-1 rounded-md transition-all ${
                  targetFilter === 'issues'
                    ? 'bg-white dark:bg-slate-700 shadow text-red-600 dark:text-red-400 font-semibold'
                    : 'text-gray-600 dark:text-slate-400 hover:text-gray-900 dark:hover:text-white'
                }`}
              >
                {t('report.filterIssuesOnly')}
              </button>
              <button
                onClick={() => setTargetFilter('success')}
                className={`px-2.5 py-1 rounded-md transition-all ${
                  targetFilter === 'success'
                    ? 'bg-white dark:bg-slate-700 shadow text-green-600 dark:text-green-400 font-semibold'
                    : 'text-gray-600 dark:text-slate-400 hover:text-gray-900 dark:hover:text-white'
                }`}
              >
                {t('report.filterAllReachable')}
              </button>
            </div>
          ) : (
            <div className="flex bg-gray-200/80 dark:bg-slate-800 p-0.5 rounded-lg text-xs font-medium">
              <button
                onClick={() => setNodeFilter('all')}
                className={`px-2.5 py-1 rounded-md transition-all ${
                  nodeFilter === 'all'
                    ? 'bg-white dark:bg-slate-700 shadow text-gray-900 dark:text-white'
                    : 'text-gray-600 dark:text-slate-400 hover:text-gray-900 dark:hover:text-white'
                }`}
              >
                {t('report.filterAll')}
              </button>
              <button
                onClick={() => setNodeFilter('failed')}
                className={`px-2.5 py-1 rounded-md transition-all ${
                  nodeFilter === 'failed'
                    ? 'bg-white dark:bg-slate-700 shadow text-red-600 dark:text-red-400 font-semibold'
                    : 'text-gray-600 dark:text-slate-400 hover:text-gray-900 dark:hover:text-white'
                }`}
              >
                {t('report.filterFailed')}
              </button>
              <button
                onClick={() => setNodeFilter('success')}
                className={`px-2.5 py-1 rounded-md transition-all ${
                  nodeFilter === 'success'
                    ? 'bg-white dark:bg-slate-700 shadow text-green-600 dark:text-green-400 font-semibold'
                    : 'text-gray-600 dark:text-slate-400 hover:text-gray-900 dark:hover:text-white'
                }`}
              >
                {t('report.filterSuccess')}
              </button>
            </div>
          )}

          {/* Quick Collapse/Expand button in Target mode */}
          {viewMode === 'target' && targetGroups.length > 0 && (
            <div className="flex items-center space-x-1.5 text-xs">
              <button
                onClick={() => setAllExpanded(true)}
                className="px-2.5 py-1 text-gray-600 dark:text-slate-300 hover:text-gray-900 dark:hover:text-white hover:bg-gray-100 dark:hover:bg-slate-800 rounded-md transition-colors font-medium border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-900 shadow-sm"
              >
                {t('report.expandAll')}
              </button>
              <button
                onClick={() => setAllExpanded(false)}
                className="px-2.5 py-1 text-gray-600 dark:text-slate-300 hover:text-gray-900 dark:hover:text-white hover:bg-gray-100 dark:hover:bg-slate-800 rounded-md transition-colors font-medium border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-900 shadow-sm"
              >
                {t('report.collapseAll')}
              </button>
            </div>
          )}
        </div>
      </div>

      {/* Main View: Target-Centric View */}
      {viewMode === 'target' && (
        <div className="space-y-4">
          {filteredTargets.map((group) => (
            <TargetCard
              key={group.key}
              group={group}
              isExpanded={isTargetExpanded(group.key)}
              onToggleExpand={() => toggleExpand(group.key)}
            />
          ))}

          {filteredTargets.length === 0 && (
            <div className="py-12 text-center bg-white rounded-xl border border-dashed border-gray-200">
              <Target className="w-8 h-8 text-gray-300 mx-auto mb-2" />
              <p className="text-sm text-gray-500 font-medium">
                {isRunning ? t('report.waitingTargets') : t('report.noTargetsMatch')}
              </p>
            </div>
          )}
        </div>
      )}

      {/* Alternate View: Node-Centric View */}
      {viewMode === 'node' && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {filteredNodes.map((node) => (
            <NodeCard key={node.node_name} node={node} />
          ))}

          {filteredNodes.length === 0 && (
            <div className="col-span-full py-12 text-center bg-white rounded-xl border border-dashed border-gray-200">
              <Server className="w-8 h-8 text-gray-300 mx-auto mb-2" />
              <p className="text-sm text-gray-500 font-medium">
                {isRunning ? t('report.waitingNodes') : t('report.noNodesFound')}
              </p>
            </div>
          )}
        </div>
      )}

      {/* Floating Bottom-Right Toast Notification */}
      {toastMessage && (
        <div className="fixed bottom-6 right-6 z-50 flex items-center bg-gray-900 text-white dark:bg-slate-800 dark:text-slate-100 px-4 py-3 rounded-xl shadow-2xl border border-gray-700 dark:border-slate-600 animate-in fade-in slide-in-from-bottom-5">
          <div className="w-6 h-6 rounded-full bg-emerald-500/20 text-emerald-400 flex items-center justify-center mr-3 flex-shrink-0">
            <Check className="w-4 h-4" />
          </div>
          <div className="text-xs font-medium mr-4">
            {toastMessage}
          </div>
          <button
            onClick={() => setToastMessage(null)}
            className="text-gray-400 hover:text-white dark:hover:text-slate-200 transition-colors p-0.5 rounded"
            title="Close"
          >
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      )}
    </div>
  );
};

// ==========================================
// TARGET CARD COMPONENT (Target-Centric View)
// ==========================================
interface TargetCardProps {
  group: TargetGroup;
  isExpanded: boolean;
  onToggleExpand: () => void;
}

const TargetCard: React.FC<TargetCardProps> = ({ group, isExpanded, onToggleExpand }) => {
  const { t } = useTranslation();
  const {
    target,
    displayLabel,
    protocolBadge,
    totalNodes,
    successfulNodes,
    avgLatencyMs,
    overallStatus,
    nodes,
  } = group;

  const percentReachable = totalNodes > 0 ? Math.round((successfulNodes / totalNodes) * 100) : 0;
  const isProblematic = overallStatus === 'partial' || overallStatus === 'all_failed' || group.certStatus === 'expired';

  return (
    <div
      className={`bg-white dark:bg-slate-900 rounded-xl border shadow-sm transition-all overflow-hidden ${
        isProblematic
          ? 'border-red-200 dark:border-red-900 ring-1 ring-red-100 dark:ring-red-950'
          : 'border-gray-200 dark:border-slate-800 hover:border-gray-300 dark:hover:border-slate-700'
      }`}
    >
      {/* Target Header Banner */}
      <div
        onClick={onToggleExpand}
        className="p-4 sm:p-5 cursor-pointer bg-white dark:bg-slate-900 hover:bg-gray-50/70 dark:hover:bg-slate-800/60 transition-colors flex flex-col md:flex-row md:items-center justify-between gap-4 select-none"
      >
        <div className="flex items-start sm:items-center space-x-3.5">
          {/* Protocol Badge */}
          <span
            className={`px-2.5 py-1 rounded-md text-xs font-mono font-bold tracking-wider uppercase border ${getProtocolBadgeStyle(
              target.protocol
            )}`}
          >
            {protocolBadge}
          </span>

          <div>
            <div className="flex items-center space-x-2">
              <span className="font-mono font-bold text-base text-gray-900 dark:text-white tracking-tight">
                {displayLabel}
              </span>
              {target.path && (
                <span className="text-xs font-mono text-gray-500 dark:text-slate-400">{target.path}</span>
              )}
            </div>

            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 mt-1 text-xs text-gray-500 dark:text-slate-400">
              <span className="flex items-center">
                <Globe className="w-3 h-3 mr-1 text-gray-400 dark:text-slate-500" />
                {t('report.colHost')}: <strong className="ml-1 text-gray-700 dark:text-slate-300">{target.host}</strong>
              </span>
              {target.port && (
                <span>{t('report.colPort')}: <strong className="text-gray-700 dark:text-slate-300">{target.port}</strong></span>
              )}
              {target.insecure_tls && (
                <span className="text-amber-600 dark:text-amber-400 bg-amber-50 dark:bg-amber-950/50 px-1.5 py-0.2 rounded border border-amber-200 dark:border-amber-800 font-medium">
                  {t('newCheck.insecureTls')}
                </span>
              )}
            </div>
          </div>
        </div>

        {/* Right side: Overall Target Status & Metrics */}
        <div className="flex items-center justify-between md:justify-end space-x-3 sm:space-x-4">
          <div className="text-right">
            <div className="flex items-center justify-end space-x-2">
              {getTargetStatusBadge(overallStatus, successfulNodes, totalNodes, t)}
              {target.protocol === 'https' && group.certStatus && renderCertBadge(group.certStatus, group.certExpiry, group.certDaysLeft, t)}
            </div>

            <div className="text-xs text-gray-500 dark:text-slate-400 mt-1 flex items-center justify-end space-x-2">
              <span>
                {t('report.reachable')}: <strong className={successfulNodes === totalNodes ? 'text-green-700 dark:text-green-400 font-semibold' : 'text-gray-800 dark:text-slate-200'}>
                  {successfulNodes}/{totalNodes} ({percentReachable}%)
                </strong>
              </span>
              {avgLatencyMs > 0 && (
                <>
                  <span className="text-gray-300 dark:text-slate-600">•</span>
                  <span className="font-mono text-gray-600 dark:text-slate-300">~{avgLatencyMs} ms</span>
                </>
              )}
              {target.protocol === 'https' && group.certExpiry && (
                <>
                  <span className="text-gray-300 dark:text-slate-600">•</span>
                  <span className="font-mono text-gray-600 dark:text-slate-300" title={`Certificate expires on: ${group.certExpiry}`}>
                    SSL exp: {formatCertDate(group.certExpiry)}
                  </span>
                </>
              )}
            </div>
          </div>

          <button
            type="button"
            className="p-1 rounded-lg text-gray-400 dark:text-slate-500 hover:text-gray-600 dark:hover:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-800"
            aria-label="Toggle details"
          >
            {isExpanded ? <ChevronUp className="w-5 h-5" /> : <ChevronDown className="w-5 h-5" />}
          </button>
        </div>
      </div>

      {/* Quick Node Status Matrix (Glance Bar) */}
      <div className="px-4 sm:px-5 py-2.5 bg-gray-50/70 dark:bg-slate-800/50 border-t border-b border-gray-100 dark:border-slate-800 flex flex-wrap items-center gap-2">
        <span className="text-[11px] font-semibold uppercase text-gray-400 dark:text-slate-500 mr-1 flex items-center">
          <Server className="w-3 h-3 mr-1" />
          {t('report.colNodes')}:
        </span>
        {nodes.map((node) => (
          <div
            key={node.node_name}
            className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-mono border transition-all ${
              node.node_status === 'running'
                ? 'bg-yellow-50 dark:bg-yellow-950/40 text-yellow-800 dark:text-yellow-300 border-yellow-200 dark:border-yellow-800'
                : node.success
                ? 'bg-green-50 dark:bg-green-950/40 text-green-800 dark:text-green-300 border-green-200 dark:border-green-800'
                : 'bg-red-50 dark:bg-red-950/40 text-red-800 dark:text-red-300 border-red-200 dark:border-red-800 font-semibold'
            }`}
          >
            {node.node_status === 'running' ? (
              <Activity className="w-3 h-3 mr-1 animate-spin text-yellow-600 dark:text-yellow-400" />
            ) : node.success ? (
              <Check className="w-3 h-3 mr-1 text-green-600 dark:text-green-400" />
            ) : (
              <XCircle className="w-3 h-3 mr-1 text-red-600 dark:text-red-400" />
            )}
            <span>{node.node_name}</span>
            {node.latency_ms > 0 && (
              <span className="ml-1 text-[10px] text-gray-500 dark:text-slate-400 font-sans">({node.latency_ms}ms)</span>
            )}
          </div>
        ))}
      </div>

      {/* Expanded Breakdown: Per-Node Details */}
      {isExpanded && (
        <div className="p-4 sm:p-5 bg-white dark:bg-slate-900 space-y-2.5 transition-colors">
          <div className="text-xs font-semibold text-gray-500 dark:text-slate-400 uppercase tracking-wider mb-2">
            {t('report.perNodeBreakdown')} ({nodes.length})
          </div>

          <div className="space-y-2">
            {nodes.map((n) => (
              <TargetNodeDetailRow key={n.node_name} node={n} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
};

// ==========================================
// TARGET NODE DETAIL ROW COMPONENT
// ==========================================
const TargetNodeDetailRow: React.FC<{ node: TargetNodeStatus }> = ({ node }) => {
  const { t } = useTranslation();
  const isRunning = node.node_status === 'running';
  const isFailed = !isRunning && !node.success;

  return (
    <div
      className={`p-3 rounded-lg text-xs border transition-all ${
        isRunning
          ? 'bg-yellow-50/40 dark:bg-yellow-950/20 border-yellow-200 dark:border-yellow-900'
          : isFailed
          ? 'bg-red-50/60 dark:bg-red-950/30 border-red-200 dark:border-red-900'
          : 'bg-green-50/40 dark:bg-green-950/20 border-green-200 dark:border-green-900'
      }`}
    >
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
        <div className="flex items-center space-x-2">
          {isRunning ? (
            <Activity className="w-4 h-4 text-yellow-600 dark:text-yellow-400 animate-spin flex-shrink-0" />
          ) : isFailed ? (
            <XCircle className="w-4 h-4 text-red-600 dark:text-red-400 flex-shrink-0" />
          ) : (
            <CheckCircle className="w-4 h-4 text-green-600 dark:text-green-400 flex-shrink-0" />
          )}

          <div className="flex items-baseline space-x-1.5">
            <span className="font-bold text-gray-900 dark:text-white font-mono">{node.node_name}</span>
            {node.node_ip && (
              <span className="text-[11px] text-gray-400 dark:text-slate-500">({node.node_ip})</span>
            )}
          </div>
        </div>

        <div className="flex items-center space-x-2 sm:self-auto self-end">
          {node.latency_ms > 0 && (
            <span className="font-mono font-medium text-gray-600 dark:text-slate-300">{node.latency_ms} ms</span>
          )}

          <span
            className={`px-2 py-0.5 rounded font-bold uppercase text-[10px] tracking-wide ${
              isRunning
                ? 'bg-yellow-500 text-white'
                : node.success
                ? 'bg-green-600 text-white'
                : 'bg-red-600 text-white'
            }`}
          >
            {isRunning ? t('report.statusRunning') : node.success ? t('report.statusReachable') : t('report.statusBlocked')}
          </span>
        </div>
      </div>

      {/* Diagnostic details, IP & Certificate */}
      <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-gray-500 dark:text-slate-400 text-[11px]">
        {node.resolved_ip && (
          <span>
            Resolved IP: <code className="text-gray-800 dark:text-slate-200 font-mono font-semibold">{node.resolved_ip}</code>
          </span>
        )}
        {node.http_status_code ? (
          <span>
            HTTP Code: <code className="text-gray-800 dark:text-slate-200 font-mono font-semibold">{node.http_status_code}</code>
          </span>
        ) : null}
        {node.cert_expiry && (
          <span className="flex items-center space-x-1">
            <span>{t('report.certExpiry')}:</span>
            <code className="text-gray-800 dark:text-slate-200 font-mono font-semibold">{formatCertDate(node.cert_expiry)}</code>
            {node.cert_status === 'expired' ? (
              <span className="text-red-600 dark:text-red-400 font-bold ml-1 inline-flex items-center">
                <ShieldAlert className="w-3 h-3 mr-0.5" />
                ({node.cert_days_left !== undefined ? `${Math.abs(node.cert_days_left)}d ago` : t('report.certExpired')})
              </span>
            ) : node.cert_status === 'expiring_soon' ? (
              <span className="text-amber-600 dark:text-amber-400 font-semibold ml-1 inline-flex items-center">
                <AlertTriangle className="w-3 h-3 mr-0.5" />
                ({node.cert_days_left}d left)
              </span>
            ) : (
              <span className="text-emerald-700 dark:text-emerald-400 ml-1 inline-flex items-center">
                <ShieldCheck className="w-3 h-3 mr-0.5" />
                ({node.cert_days_left !== undefined ? `${node.cert_days_left}d left` : t('report.certValid')})
              </span>
            )}
          </span>
        )}
      </div>

      {/* Prominent Certificate Warning/Error Banners */}
      {node.cert_status === 'expired' && (
        <div className="mt-2 p-2 bg-red-100/90 dark:bg-red-950/60 border border-red-300 dark:border-red-800 rounded text-red-900 dark:text-red-200 font-medium text-xs leading-relaxed flex items-start space-x-1.5">
          <ShieldAlert className="w-4 h-4 text-red-600 dark:text-red-400 flex-shrink-0 mt-0.5" />
          <div>
            <strong>{t('report.certExpired')}:</strong> Expired on {formatCertDate(node.cert_expiry)} ({node.cert_days_left !== undefined ? `${Math.abs(node.cert_days_left)} days ago` : 'expired'}).
            {node.cert_subject && <div className="text-[11px] text-red-700 dark:text-red-300 font-mono mt-0.5">Subject: {node.cert_subject}</div>}
          </div>
        </div>
      )}

      {node.cert_status === 'expiring_soon' && (
        <div className="mt-2 p-2 bg-amber-50 dark:bg-amber-950/60 border border-amber-300 dark:border-amber-800 rounded text-amber-900 dark:text-amber-200 font-medium text-xs leading-relaxed flex items-start space-x-1.5">
          <AlertTriangle className="w-4 h-4 text-amber-600 dark:text-amber-400 flex-shrink-0 mt-0.5" />
          <div>
            <strong>{t('report.certExpiringSoon')}:</strong> Expires in {node.cert_days_left} days ({formatCertDate(node.cert_expiry)}).
            {node.cert_subject && <div className="text-[11px] text-amber-700 dark:text-amber-300 font-mono mt-0.5">Subject: {node.cert_subject}</div>}
          </div>
        </div>
      )}

      {/* Prominent Error Banner */}
      {node.error && node.error.trim() !== '' && (
        <div className="mt-2 p-2 bg-red-100/70 dark:bg-red-950/50 border border-red-200 dark:border-red-800 rounded text-red-800 dark:text-red-300 font-medium text-xs leading-relaxed">
          <div className="flex items-start space-x-1.5">
            <AlertTriangle className="w-3.5 h-3.5 text-red-600 dark:text-red-400 flex-shrink-0 mt-0.5" />
            <span>{node.error.trim()}</span>
          </div>
        </div>
      )}

      {/* Normal Details */}
      {(!node.error || node.error.trim() === '') && node.details && node.details.trim() !== '' && (
        <div className="mt-1 text-gray-600 dark:text-slate-400 text-[11px]">
          {node.details.trim()}
        </div>
      )}
    </div>
  );
};

// ==========================================
// CLASSIC NODE CARD COMPONENT (Node-Centric)
// ==========================================
const NodeCard: React.FC<{ node: NodeResult }> = ({ node }) => {
  const { t } = useTranslation();
  const isFailed = node.status !== 'completed' || node.service_results?.some((s) => !s.success);

  return (
    <div
      className={`bg-white dark:bg-slate-900 rounded-xl border p-4 shadow-sm transition-all ${
        isFailed
          ? 'border-red-200 dark:border-red-900 ring-1 ring-red-100 dark:ring-red-950'
          : 'border-gray-200 dark:border-slate-800 hover:border-gray-300 dark:hover:border-slate-700'
      }`}
    >
      {/* Node Header */}
      <div className="flex justify-between items-center mb-3 pb-2.5 border-b border-gray-100 dark:border-slate-800">
        <div className="flex items-center space-x-2">
          {isFailed ? (
            <XCircle className="w-4 h-4 text-red-500 flex-shrink-0" />
          ) : (
            <CheckCircle className="w-4 h-4 text-green-500 flex-shrink-0" />
          )}
          <span className="font-bold text-sm text-gray-900 dark:text-white">{node.node_name}</span>
          {node.node_ip && (
            <span className="text-xs text-gray-400 dark:text-slate-500">({node.node_ip})</span>
          )}
        </div>

        <div className="flex items-center space-x-2">
          {node.duration_ms > 0 && (
            <span
              className="inline-flex items-center space-x-1.5 px-2 py-0.5 rounded-md text-xs font-medium bg-gray-100 dark:bg-slate-800 text-gray-600 dark:text-slate-300 border border-gray-200 dark:border-slate-700/80 cursor-help"
              title={t('report.nodeProbeDurationTooltip')}
            >
              <Clock className="w-3.5 h-3.5 text-gray-400 dark:text-slate-400 flex-shrink-0" />
              <span className="text-[11px] text-gray-500 dark:text-slate-400 font-normal">{t('report.nodeProbeDuration')}:</span>
              <span className="font-semibold text-gray-800 dark:text-slate-200 font-mono">{formatDuration(node.duration_ms)}</span>
            </span>
          )}
          <span
            className={`px-2 py-0.5 text-xs font-semibold rounded-full ${
              node.status === 'completed'
                ? isFailed
                  ? 'bg-red-100 dark:bg-red-950/60 text-red-800 dark:text-red-300'
                  : 'bg-green-100 dark:bg-green-950/60 text-green-800 dark:text-green-300'
                : 'bg-yellow-100 dark:bg-yellow-950/60 text-yellow-800 dark:text-yellow-300'
            }`}
          >
            {node.status === 'completed' ? t('report.statusCompleted') : node.status === 'failed' ? t('report.statusFailed') : t('report.statusRunning')}
          </span>
        </div>
      </div>

      {/* Node-level Error */}
      {node.error && (
        <div className="mb-3 p-2.5 bg-red-50 dark:bg-red-950/50 text-xs text-red-700 dark:text-red-300 rounded-md border border-red-200 dark:border-red-800">
          <strong>{t('report.errorDetails')}:</strong> {node.error}
        </div>
      )}

      {/* Target Service Probes on this Node */}
      <div className="space-y-2">
        {node.service_results?.map((res, i) => (
          <TargetResultRow key={i} result={res} />
        ))}
      </div>
    </div>
  );
};

const TargetResultRow: React.FC<{ result: ProbeOutput }> = ({ result }) => {
  const { t } = useTranslation();
  const {
    target,
    success,
    latency_ms,
    resolved_ip,
    http_status_code,
    error,
    details,
    cert_expiry,
    cert_days_left,
    cert_status,
  } = result;

  const targetLabel = formatTargetKey(target);

  return (
    <div
      className={`p-2.5 rounded-lg text-xs border ${
        success
          ? 'bg-green-50/40 dark:bg-green-950/20 border-green-200 dark:border-green-900'
          : 'bg-red-50/60 dark:bg-red-950/30 border-red-200 dark:border-red-900'
      }`}
    >
      <div className="flex justify-between items-center mb-1">
        <span className="font-mono font-semibold text-gray-900 dark:text-white">{targetLabel}</span>
        <div className="flex items-center space-x-2">
          {latency_ms > 0 && <span className="text-gray-500 dark:text-slate-400 font-mono">{latency_ms} ms</span>}
          <span
            className={`px-1.5 py-0.5 rounded font-bold uppercase text-[10px] ${
              success ? 'bg-green-600 text-white' : 'bg-red-600 text-white'
            }`}
          >
            {success ? t('report.statusReachable') : t('report.statusBlocked')}
          </span>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-gray-500 dark:text-slate-400 text-[11px]">
        {resolved_ip && (
          <span>
            IP: <code className="text-gray-700 dark:text-slate-300 font-mono">{resolved_ip}</code>
          </span>
        )}
        {http_status_code ? (
          <span>
            HTTP: <code className="text-gray-700 dark:text-slate-300 font-mono">{http_status_code}</code>
          </span>
        ) : null}
        {cert_expiry && (
          <span className="flex items-center space-x-1">
            <span>{t('report.certExpiry')}:</span>
            <code className="text-gray-700 dark:text-slate-300 font-mono">{formatCertDate(cert_expiry)}</code>
            {cert_status === 'expired' ? (
              <span className="text-red-600 dark:text-red-400 font-bold ml-1 inline-flex items-center">
                <ShieldAlert className="w-3 h-3 mr-0.5" />
                ({cert_days_left !== undefined ? `${Math.abs(cert_days_left)}d ago` : 'expired'})
              </span>
            ) : cert_status === 'expiring_soon' ? (
              <span className="text-amber-600 dark:text-amber-400 font-semibold ml-1 inline-flex items-center">
                <AlertTriangle className="w-3 h-3 mr-0.5" />
                ({cert_days_left}d left)
              </span>
            ) : (
              <span className="text-emerald-700 dark:text-emerald-400 ml-1 inline-flex items-center">
                <ShieldCheck className="w-3 h-3 mr-0.5" />
                ({cert_days_left !== undefined ? `${cert_days_left}d left` : 'valid'})
              </span>
            )}
          </span>
        )}
      </div>

      {cert_status === 'expired' && (
        <div className="mt-1.5 p-2 bg-red-100/90 dark:bg-red-950/60 border border-red-300 dark:border-red-800 rounded text-red-900 dark:text-red-200 font-medium text-xs leading-relaxed flex items-start space-x-1.5">
          <ShieldAlert className="w-3.5 h-3.5 text-red-600 dark:text-red-400 flex-shrink-0 mt-0.5" />
          <span>SSL Certificate Expired on {formatCertDate(cert_expiry)} ({cert_days_left !== undefined ? `${Math.abs(cert_days_left)} days ago` : 'expired'}).</span>
        </div>
      )}
      {cert_status === 'expiring_soon' && (
        <div className="mt-1.5 p-2 bg-amber-50 dark:bg-amber-950/60 border border-amber-300 dark:border-amber-800 rounded text-amber-900 dark:text-amber-200 font-medium text-xs leading-relaxed flex items-start space-x-1.5">
          <AlertTriangle className="w-3.5 h-3.5 text-amber-600 dark:text-amber-400 flex-shrink-0 mt-0.5" />
          <span>SSL Certificate Expiring Soon ({cert_days_left} days left).</span>
        </div>
      )}

      {!success && error && error.trim() !== '' && (
        <div className="mt-1 text-red-700 dark:text-red-400 font-medium">{error.trim()}</div>
      )}
      {(success || !error || error.trim() === '') && details && details.trim() !== '' && (
        <div className="mt-0.5 text-gray-600 dark:text-slate-400">{details.trim()}</div>
      )}
    </div>
  );
};

// ==========================================
// UTILITY HELPERS
// ==========================================
function formatTargetKey(target: TargetService): string {
  if (target.protocol === 'icmp') {
    return `icmp://${target.host}`;
  }
  if (target.protocol === 'http' || target.protocol === 'https') {
    const defaultPort = target.protocol === 'http' ? 80 : 443;
    const port = target.port || defaultPort;
    const path = target.path || '/';
    return `${target.protocol}://${target.host}:${port}${path}`;
  }
  return `${target.protocol}://${target.host}:${target.port}`;
}

function formatTargetDisplayLabel(target: TargetService): string {
  if (target.protocol === 'icmp') {
    return target.host;
  }
  if (target.protocol === 'http' || target.protocol === 'https') {
    const defaultPort = target.protocol === 'http' ? 80 : 443;
    const portPart = target.port && target.port !== defaultPort ? `:${target.port}` : '';
    return `${target.host}${portPart}`;
  }
  return `${target.host}:${target.port}`;
}

function getProtocolBadgeStyle(protocol: string): string {
  switch (protocol) {
    case 'tcp':
      return 'bg-blue-50 dark:bg-blue-950/60 text-blue-700 dark:text-blue-300 border-blue-200 dark:border-blue-800';
    case 'udp':
      return 'bg-purple-50 dark:bg-purple-950/60 text-purple-700 dark:text-purple-300 border-purple-200 dark:border-purple-800';
    case 'http':
      return 'bg-amber-50 dark:bg-amber-950/60 text-amber-700 dark:text-amber-300 border-amber-200 dark:border-amber-800';
    case 'https':
      return 'bg-emerald-50 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800';
    case 'icmp':
      return 'bg-cyan-50 dark:bg-cyan-950/60 text-cyan-700 dark:text-cyan-300 border-cyan-200 dark:border-cyan-800';
    default:
      return 'bg-gray-50 dark:bg-slate-800 text-gray-700 dark:text-slate-300 border-gray-200 dark:border-slate-700';
  }
}

function getTargetStatusBadge(
  status: 'all_ok' | 'partial' | 'all_failed' | 'running',
  successfulNodes: number,
  totalNodes: number,
  t?: (key: string) => string
) {
  switch (status) {
    case 'all_ok':
      return (
        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-bold bg-green-100 dark:bg-green-950/60 text-green-800 dark:text-green-300 border border-green-200 dark:border-green-800">
          <CheckCircle className="w-3.5 h-3.5 mr-1 text-green-600 dark:text-green-400" />
          {t ? t('report.allNodesReachable') : 'ALL NODES REACHABLE'}
        </span>
      );
    case 'partial':
      return (
        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-bold bg-amber-100 dark:bg-amber-950/60 text-amber-800 dark:text-amber-300 border border-amber-200 dark:border-amber-800">
          <AlertTriangle className="w-3.5 h-3.5 mr-1 text-amber-600 dark:text-amber-400" />
          {t ? t('report.partiallyReachable') : 'PARTIALLY REACHABLE'} ({successfulNodes}/{totalNodes})
        </span>
      );
    case 'all_failed':
      return (
        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-bold bg-red-100 dark:bg-red-950/60 text-red-800 dark:text-red-300 border border-red-200 dark:border-red-800">
          <XCircle className="w-3.5 h-3.5 mr-1 text-red-600 dark:text-red-400" />
          {t ? t('report.allNodesFailed') : 'UNREACHABLE'}
        </span>
      );
    case 'running':
      return (
        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-bold bg-yellow-100 dark:bg-yellow-950/60 text-yellow-800 dark:text-yellow-300 border border-yellow-200 dark:border-yellow-800">
          <Activity className="w-3.5 h-3.5 mr-1 animate-spin text-yellow-600 dark:text-yellow-400" />
          {t ? t('report.probing') : 'PROBING...'}
        </span>
      );
  }
}

function getStatusBadge(status: string, t?: (key: string) => string) {
  switch (status) {
    case 'pending':
    case 'running':
      return (
        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-semibold bg-yellow-100 dark:bg-yellow-950/60 text-yellow-800 dark:text-yellow-300">
          <Activity className="w-3 h-3 mr-1 animate-spin" />
          {t ? t('report.statusRunning') : 'RUNNING'}
        </span>
      );
    case 'completed':
      return (
        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-semibold bg-green-100 dark:bg-green-950/60 text-green-800 dark:text-green-300">
          <CheckCircle className="w-3 h-3 mr-1" />
          {t ? t('report.statusCompleted') : 'COMPLETED'}
        </span>
      );
    case 'failed':
      return (
        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-semibold bg-red-100 dark:bg-red-950/60 text-red-800 dark:text-red-300">
          <AlertTriangle className="w-3 h-3 mr-1" />
          {t ? t('report.statusFailed') : 'FAILED'}
        </span>
      );
    default:
      return null;
  }
}

function getCheckModeBadge(mode: CheckMode, namespace?: string, compact: boolean = false, t?: any) {
  if (mode === 'host') {
    return (
      <span
        className={`inline-flex items-center font-semibold rounded-md border shadow-xs transition-colors bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 border-indigo-200 dark:border-indigo-800/80 ${
          compact ? 'px-2 py-0.5 text-xs' : 'px-2.5 py-1 text-xs'
        }`}
        title="Check executed directly in host network mode (hostNetwork: true) on cluster node interfaces"
      >
        <Server className="w-3.5 h-3.5 mr-1.5 text-indigo-600 dark:text-indigo-400 shrink-0" />
        <span>{t ? t('report.hostNetwork') : 'Host Network'}</span>
      </span>
    );
  }

  return (
    <span
      className={`inline-flex items-center font-semibold rounded-md border shadow-xs transition-colors bg-sky-50 dark:bg-sky-950/60 text-sky-700 dark:text-sky-300 border-sky-200 dark:border-sky-800/80 ${
        compact ? 'px-2 py-0.5 text-xs' : 'px-2.5 py-1 text-xs'
      }`}
      title={`Check executed in pod network mode (CNI overlay)${namespace ? ` in namespace: ${namespace}` : ''}`}
    >
      <Network className="w-3.5 h-3.5 mr-1.5 text-sky-600 dark:text-sky-400 shrink-0" />
      <span>{t ? t('report.podNetwork') : 'Pod Network'}</span>
      {namespace && (
        <span className="ml-1.5 px-1.5 py-0.2 rounded bg-sky-100/90 dark:bg-sky-900/60 text-sky-800 dark:text-sky-200 font-mono text-[11px] font-normal border border-sky-200/60 dark:border-sky-700/60">
          ns: {namespace}
        </span>
      )}
    </span>
  );
}

function formatCertDate(dateStr?: string): string {
  if (!dateStr) return '';
  const d = new Date(dateStr);
  if (isNaN(d.getTime())) {
    return dateStr.split(' ')[0] || dateStr;
  }
  return d.toISOString().split('T')[0];
}

function renderCertBadge(
  status: 'valid' | 'expiring_soon' | 'expired',
  expiry?: string,
  daysLeft?: number,
  t?: (key: string) => string
) {
  const formattedDate = formatCertDate(expiry);
  if (status === 'expired') {
    return (
      <span
        className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-bold bg-red-100 text-red-800 border border-red-300"
        title={`SSL Certificate expired on ${expiry || 'unknown'}`}
      >
        <ShieldAlert className="w-3.5 h-3.5 mr-1 text-red-600" />
        {t ? t('report.certExpiredBadge') : 'CERT EXPIRED'} {daysLeft !== undefined ? `(${Math.abs(daysLeft)}d ago)` : ''}
      </span>
    );
  }
  if (status === 'expiring_soon') {
    return (
      <span
        className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold bg-amber-100 text-amber-800 border border-amber-300"
        title={`SSL Certificate expires on ${expiry || 'unknown'} (${daysLeft} days left)`}
      >
        <AlertTriangle className="w-3.5 h-3.5 mr-1 text-amber-600" />
        {t ? t('report.certExpiringBadge') : 'EXPIRING SOON'} ({daysLeft !== undefined ? `${daysLeft}d left` : formattedDate})
      </span>
    );
  }
  return (
    <span
      className="inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-medium bg-emerald-50 text-emerald-700 border border-emerald-200"
      title={`SSL Certificate valid until ${expiry || 'unknown'}`}
    >
      <ShieldCheck className="w-3.5 h-3.5 mr-1 text-emerald-600" />
      {t ? t('report.certValidBadge') : 'CERT VALID'} {daysLeft !== undefined ? `(${daysLeft}d left)` : ''}
    </span>
  );
}
