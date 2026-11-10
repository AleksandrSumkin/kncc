import React, { useEffect, useState } from 'react';
import { api } from '../api';
import { Report } from '../types';
import { History, ChevronLeft, ChevronRight, RotateCw, ExternalLink, Activity, CheckCircle, XCircle } from 'lucide-react';

interface HistoryPageProps {
  onSelectReport: (reportId: string) => void;
}

export const HistoryPage: React.FC<HistoryPageProps> = ({ onSelectReport }) => {
  const [reports, setReports] = useState<Report[]>([]);
  const [total, setTotal] = useState(0);
  const [limit] = useState(15);
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(true);

  const fetchReports = async (newOffset = offset) => {
    setLoading(true);
    try {
      const res = await api.listReports(limit, newOffset);
      setReports(res.reports || []);
      setTotal(res.total || 0);
      setOffset(newOffset);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchReports(0);
  }, []);

  return (
    <div className="max-w-6xl mx-auto py-8 px-4 sm:px-6 lg:px-8">
      <div className="flex justify-between items-center mb-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white tracking-tight flex items-center">
            <History className="w-6 h-6 mr-2 text-gray-700 dark:text-slate-300" />
            Check History
          </h1>
          <p className="text-xs text-gray-500 dark:text-slate-400 mt-1">
            Browse previous network connectivity check results across cluster nodes.
          </p>
        </div>

        <button
          onClick={() => fetchReports(offset)}
          disabled={loading}
          className="inline-flex items-center px-3 py-1.5 border border-gray-300 dark:border-slate-700 rounded-lg text-xs font-semibold text-gray-700 dark:text-slate-200 bg-white dark:bg-slate-900 hover:bg-gray-50 dark:hover:bg-slate-800 transition-colors shadow-sm"
        >
          <RotateCw className={`w-3.5 h-3.5 mr-1.5 ${loading ? 'animate-spin' : ''}`} />
          Refresh
        </button>
      </div>

      <div className="bg-white dark:bg-slate-900 rounded-xl shadow-sm border border-gray-200 dark:border-slate-800 overflow-hidden transition-colors">
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-slate-800 text-left text-xs">
            <thead className="bg-gray-50 dark:bg-slate-800 text-gray-500 dark:text-slate-400 font-semibold uppercase tracking-wider">
              <tr>
                <th className="px-6 py-3">Report ID</th>
                <th className="px-6 py-3">Status</th>
                <th className="px-6 py-3">Mode</th>
                <th className="px-6 py-3">Targets</th>
                <th className="px-6 py-3">Nodes</th>
                <th className="px-6 py-3">Created By</th>
                <th className="px-6 py-3">Date</th>
                <th className="px-6 py-3 text-right">Action</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-slate-800">
              {reports.map((r) => (
                <tr
                  key={r.id}
                  onClick={() => onSelectReport(r.id)}
                  className="hover:bg-gray-50/80 dark:hover:bg-slate-800/40 cursor-pointer transition-colors"
                >
                  <td className="px-6 py-4 font-mono font-semibold text-gray-900 dark:text-white">
                    #{r.id.slice(0, 8)}
                  </td>
                  <td className="px-6 py-4">
                    <StatusPill status={r.status} />
                  </td>
                  <td className="px-6 py-4 uppercase font-semibold text-gray-700 dark:text-slate-300">
                    {r.mode}
                    {r.mode === 'pod' && (
                      <span className="text-gray-400 dark:text-slate-500 font-normal lowercase block text-[11px]">
                        ns: {r.namespace}
                      </span>
                    )}
                  </td>
                  <td className="px-6 py-4 text-gray-600 dark:text-slate-400">
                    <span className="font-semibold text-gray-900 dark:text-white">{r.targets?.length || 0}</span> targets
                  </td>
                  <td className="px-6 py-4 text-gray-600 dark:text-slate-400">
                    {r.completed_nodes} / {r.total_nodes}
                  </td>
                  <td className="px-6 py-4 font-medium text-gray-800 dark:text-slate-200">
                    {r.username}
                  </td>
                  <td className="px-6 py-4 text-gray-500 dark:text-slate-400">
                    {new Date(r.created_at).toLocaleString()}
                  </td>
                  <td className="px-6 py-4 text-right">
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        onSelectReport(r.id);
                      }}
                      className="inline-flex items-center text-xs font-semibold text-red-600 dark:text-red-400 hover:text-red-800 dark:hover:text-red-300"
                    >
                      <span>View</span>
                      <ExternalLink className="w-3.5 h-3.5 ml-1" />
                    </button>
                  </td>
                </tr>
              ))}

              {reports.length === 0 && !loading && (
                <tr>
                  <td colSpan={8} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">
                    No reports found. Submit a new check to get started.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination Footer */}
        <div className="px-6 py-3 bg-gray-50 dark:bg-slate-800/80 border-t border-gray-200 dark:border-slate-800 flex items-center justify-between">
          <div className="text-xs text-gray-500 dark:text-slate-400">
            Showing <strong className="text-gray-900 dark:text-white">{reports.length > 0 ? offset + 1 : 0}</strong> to{' '}
            <strong className="text-gray-900 dark:text-white">{Math.min(offset + limit, total)}</strong> of{' '}
            <strong className="text-gray-900 dark:text-white">{total}</strong> results
          </div>

          <div className="flex space-x-2">
            <button
              disabled={offset === 0}
              onClick={() => fetchReports(Math.max(0, offset - limit))}
              className="p-1.5 border border-gray-300 dark:border-slate-700 rounded bg-white dark:bg-slate-800 text-gray-700 dark:text-slate-200 hover:bg-gray-50 dark:hover:bg-slate-700 disabled:opacity-30 disabled:cursor-not-allowed"
            >
              <ChevronLeft className="w-4 h-4" />
            </button>
            <button
              disabled={offset + limit >= total}
              onClick={() => fetchReports(offset + limit)}
              className="p-1.5 border border-gray-300 dark:border-slate-700 rounded bg-white dark:bg-slate-800 text-gray-700 dark:text-slate-200 hover:bg-gray-50 dark:hover:bg-slate-700 disabled:opacity-30 disabled:cursor-not-allowed"
            >
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

const StatusPill: React.FC<{ status: string }> = ({ status }) => {
  switch (status) {
    case 'running':
    case 'pending':
      return (
        <span className="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-semibold bg-yellow-100 dark:bg-yellow-950/60 text-yellow-800 dark:text-yellow-300">
          <Activity className="w-3 h-3 mr-1 animate-spin" />
          Running
        </span>
      );
    case 'completed':
      return (
        <span className="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-semibold bg-green-100 dark:bg-green-950/60 text-green-800 dark:text-green-300">
          <CheckCircle className="w-3 h-3 mr-1 text-green-600 dark:text-green-400" />
          Completed
        </span>
      );
    case 'failed':
      return (
        <span className="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-semibold bg-red-100 dark:bg-red-950/60 text-red-800 dark:text-red-300">
          <XCircle className="w-3 h-3 mr-1 text-red-600 dark:text-red-400" />
          Failed
        </span>
      );
    default:
      return <span>{status}</span>;
  }
};
