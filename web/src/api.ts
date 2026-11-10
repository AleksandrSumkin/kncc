import { CheckRequest, Report, SystemConfig, User, RBACReport, UserStatsResponse, TargetService } from './types';

const API_BASE = '/api';

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

async function request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const token = localStorage.getItem('kncc_token');
  const headers = new Headers(options.headers || {});
  if (!(options.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json');
  }
  if (token) {
    headers.set('Authorization', `Bearer ${token}`);
  }

  const res = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    headers,
  });

  if (res.status === 401 && !endpoint.includes('/auth/login')) {
    localStorage.removeItem('kncc_token');
    window.dispatchEvent(new Event('kncc_unauthorized'));
    throw new ApiError('Session expired', 401);
  }

  const data = await res.json().catch(() => ({}));

  if (!res.ok) {
    throw new ApiError(data.error || `HTTP error ${res.status}`, res.status);
  }

  return data as T;
}

export const api = {
  getSystemConfig: () => request<SystemConfig>('/system/config'),
  getAuthProviders: () => request<{ providers: string[]; oidc_auth_url?: string }>('/auth/providers'),
  
  login: (credentials: { username: string; password: string; provider?: string }) =>
    request<{ user: User; token: string }>('/auth/login', {
      method: 'POST',
      body: JSON.stringify(credentials),
    }),
  
  logout: () => request<{ success: boolean }>('/auth/logout', { method: 'POST' }),
  getMe: () => request<{ user: User }>('/auth/me'),

  createCheck: (req: CheckRequest) =>
    request<Report>('/checks', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  getQuickAddTargets: () =>
    request<{ quick_add: TargetService[] }>('/checks/quick-add'),

  listReports: (limit = 20, offset = 0) =>
    request<{ reports: Report[]; total: number; limit: number; offset: number }>(
      `/reports?limit=${limit}&offset=${offset}`
    ),

  getReport: (id: string) => request<Report>(`/reports/${id}`),

  getNamespaces: () => request<{ namespaces: string[] }>('/k8s/namespaces'),

  rerunReport: (id: string) =>
    request<Report>(`/reports/${id}/rerun`, {
      method: 'POST',
    }),

  listUsers: () => request<{ users: User[] }>('/admin/users'),

  getUserStats: () => request<UserStatsResponse>('/admin/stats/users'),

  createUser: (userData: { username: string; password: string; role: string; email?: string }) =>
    request<User>('/admin/users', {
      method: 'POST',
      body: JSON.stringify(userData),
    }),

  getRBACReport: () => request<RBACReport>('/admin/rbac'),

  updateThemeSettings: (settings: { default_theme?: 'light' | 'dark'; default_language?: 'en' | 'ru' } | 'light' | 'dark') => {
    const payload = typeof settings === 'string' ? { default_theme: settings } : settings;
    return request<{ success: boolean; default_theme?: string; default_language?: string }>('/admin/settings/theme', {
      method: 'PUT',
      body: JSON.stringify(payload),
    });
  },

  uploadLogo: (file: File) => {
    const formData = new FormData();
    formData.append('logo', file);
    return request<{ success: boolean; logo_url: string; message: string }>('/admin/settings/logo', {
      method: 'POST',
      body: formData,
    });
  },

  resetLogo: () =>
    request<{ success: boolean; logo_url: string; message: string }>('/admin/settings/logo', {
      method: 'DELETE',
    }),

  uploadFavicon: (file: File) => {
    const formData = new FormData();
    formData.append('favicon', file);
    return request<{ success: boolean; favicon_url: string; message: string }>('/admin/settings/favicon', {
      method: 'POST',
      body: formData,
    });
  },

  resetFavicon: () =>
    request<{ success: boolean; favicon_url: string; message: string }>('/admin/settings/favicon', {
      method: 'DELETE',
    }),
};
