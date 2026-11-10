export type Protocol = 'tcp' | 'udp' | 'http' | 'https' | 'icmp';
export type CheckMode = 'host' | 'pod';
export type ReportStatus = 'pending' | 'running' | 'completed' | 'failed';
export type Role = 'admin' | 'user';

export interface TargetService {
  name?: string;
  protocol: Protocol;
  host: string;
  port?: number;
  path?: string;
  insecure_tls?: boolean;
  timeout_seconds?: number;
}

export interface CheckRequest {
  targets: TargetService[];
  mode: CheckMode;
  namespace?: string;
}

export type CertStatus = 'valid' | 'expiring_soon' | 'expired';

export interface ProbeOutput {
  target: TargetService;
  success: boolean;
  latency_ms: number;
  resolved_ip?: string;
  http_status_code?: number;
  error?: string;
  details?: string;
  cert_expiry?: string;
  cert_days_left?: number;
  cert_status?: CertStatus;
  cert_subject?: string;
  cert_issuer?: string;
}

export interface NodeResult {
  node_name: string;
  node_ip?: string;
  status: string; // 'completed' | 'failed' | 'timeout' | 'running'
  service_results: ProbeOutput[];
  error?: string;
  duration_ms: number;
  finished_at: string;
}

export interface Report {
  id: string;
  user_id: string;
  username: string;
  mode: CheckMode;
  namespace: string;
  targets: TargetService[];
  status: ReportStatus;
  total_nodes: number;
  completed_nodes: number;
  node_results: Record<string, NodeResult>;
  created_at: string;
  completed_at?: string;
  expires_at: string;
}

export interface User {
  id: string;
  username: string;
  role: Role;
  provider: string;
  email?: string;
  created_at: string;
}

export interface UserStats {
  user_id: string;
  username: string;
  role: Role;
  provider: string;
  day_count: number;
  week_count: number;
  month_count: number;
  year_count: number;
  total_count: number;
  last_active?: string;
}

export interface UserStatsSummary {
  day_count: number;
  week_count: number;
  month_count: number;
  year_count: number;
  total_count: number;
}

export interface UserStatsResponse {
  users: UserStats[];
  total: UserStatsSummary;
}

export interface SystemConfig {
  version?: string;
  app_name: string;
  primary_color: string;
  accent_color: string;
  default_theme?: 'light' | 'dark';
  default_language?: 'en' | 'ru' | string;
  auth_providers: string[];
  multi_namespace_allowed: boolean;
  namespace_list_enabled: boolean;
  default_namespace: string;
  rate_limit_per_minute: number;
  quick_add_presets?: TargetService[];
  has_custom_logo?: boolean;
  logo_url?: string;
  has_custom_favicon?: boolean;
  favicon_url?: string;
}

export interface SubjectInfo {
  kind: string;
  name: string;
  namespace?: string;
  raw_username?: string;
}

export interface PolicyRule {
  type: string;
  subject: string;
  verbs: string[];
  namespace: string;
  api_group: string;
  resource: string;
  resource_names?: string[];
  non_resource_urls?: string[];
  originated_from?: string;
}

export interface RBACReport {
  subject: SubjectInfo;
  rules: PolicyRule[];
  source: string;
  evaluation_error?: string;
  generated_at: string;
}
