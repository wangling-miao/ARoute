// ─── Core Entities ───────────────────────────────────────────────

export interface User {
  id: string;
  username: string;
  email: string;
  roles: string[];
  permissions: Permission[];
  status: 'active' | 'inactive' | 'suspended';
  created_at: string;
  updated_at: string;
}

export interface Role {
  id: string;
  name: string;
  description: string;
  permissions: Permission[];
}

export interface Permission {
  resource: string;
  actions: string[];
}

// ─── Content ─────────────────────────────────────────────────────

export interface ContentItem {
  id: string;
  content_type: string;
  status: 'draft' | 'published';
  author_id: string;
  created_at: string;
  updated_at: string;
  published_at?: string;
  title?: string;
  slug?: string;
  body?: string;
  seo_title?: string;
  seo_description?: string;
  [key: string]: unknown;
}

export interface ContentType {
  name: string;
  display_name: string;
  description: string;
  table_name: string;
  fields: Field[];
}

export interface RelationConfig {
  target_content_type: string;
  relation_type: 'one-to-one' | 'one-to-many' | 'many-to-many';
  through_table?: string;
}

export interface Field {
  name: string;
  display_name: string;
  type: FieldType;
  required: boolean;
  unique: boolean;
  default_value?: unknown;
  validation?: ValidationRules;
  relation_config?: RelationConfig;
}

export type FieldType =
  | 'text'
  | 'richtext'
  | 'number'
  | 'boolean'
  | 'date'
  | 'datetime'
  | 'media'
  | 'relation'
  | 'enum'
  | 'json'
  | 'email'
  | 'url'
  | 'slug'
  | 'color'
  | 'markdown';

export interface ValidationRules {
  min_length?: number;
  max_length?: number;
  pattern?: string;
  min?: number;
  max?: number;
  values?: string[];
}

// ─── Media ───────────────────────────────────────────────────────

export interface MediaFile {
  id: string;
  filename: string;
  mime_type: string;
  size: number;
  width?: number;
  height?: number;
  url: string;
  created_at: string;
}

// ─── Plugins ─────────────────────────────────────────────────────

export interface Plugin {
  name: string;
  version: string;
  description: string;
  author: string;
  enabled: boolean;
  state: string;
  is_system: boolean;
  engine?: string;
  trust_level?: string;
  effective_trust?: string;
  risk_score?: number;
  trust_state?: string;
  capabilities?: string[];
  capability_grants?: string[];
  last_decision?: PluginTrustDecision;
  policy_revision?: string;
}

export interface PluginTrustDecision {
  action: string;
  reason: string;
  risk_score: number;
  policy_revision: string;
  at: string;
}

export interface PluginTrustDetail extends Plugin {
  history: PluginTrustDecision[];
}

// ─── Settings ────────────────────────────────────────────────────

export interface Settings {
  site_name: string;
  site_url: string;
  language: string;
  timezone: string;
  smtp_host: string;
  smtp_port: number;
  smtp_username: string;
  sender_email: string;
}

// ─── Dashboard ───────────────────────────────────────────────────

export interface DashboardStats {
  content_counts: Record<string, number>;
  recent_activity: ActivityItem[];
  system_status: SystemStatus;
}

export interface ActivityItem {
  id: string;
  action: string;
  resource_type: string;
  resource_id: string;
  user_id: string;
  created_at: string;
}

export interface SystemStatus {
  database: 'healthy' | 'degraded' | 'down';
  plugin_count: number;
  cache_hit_ratio: number;
}

// ─── API Tokens ──────────────────────────────────────────────────

export interface ApiToken {
  id: string;
  name: string;
  token_preview: string;
  created_at: string;
  last_used_at?: string;
  expires_at?: string;
}

// ─── Pagination ──────────────────────────────────────────────────

export interface PaginatedResponse<T> {
  data: T[];
  meta: PaginationMeta;
}

export interface PaginationMeta {
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export interface ListParams {
  page?: number;
  per_page?: number;
  sort?: string;
  order?: 'asc' | 'desc';
  filter?: Record<string, unknown>;
  search?: string;
}

// ─── API Request Types ───────────────────────────────────────────

export interface CreateContentTypeRequest {
  name: string;
  display_name: string;
  description: string;
  fields: Omit<Field, 'validation'> & { validation?: ValidationRules }[];
}

export interface UpdateContentTypeRequest {
  display_name?: string;
  description?: string;
  fields?: Field[];
}

export interface CreateUserRequest {
  username: string;
  email: string;
  password: string;
  roles: string[];
}

export interface UpdateUserRequest {
  username?: string;
  email?: string;
  password?: string;
  roles?: string[];
  status?: 'active' | 'inactive' | 'suspended';
}

export interface UpdateRoleRequest {
  description?: string;
  permissions?: Permission[];
}

export interface CreateTokenRequest {
  name: string;
  expires_at?: string;
}

// ─── Auth ────────────────────────────────────────────────────────

export interface AuthTokens {
  access_token: string;
  refresh_token: string;
}

export interface RefreshTokenResponse {
  access_token: string;
}

// ─── API Error ───────────────────────────────────────────────────

export interface ApiErrorDetail {
  code: string;
  message: string;
  details?: Record<string, unknown>;
}

export interface ApiErrorResponse {
  errors: ApiErrorDetail[];
}

// ─── Themes & Appearance ─────────────────────────────────────────

export interface ThemeInfo {
  slug: string;
  name: string;
  version: string;
  author: string;
  description: string;
  engine: string;
  active: boolean;
}

export interface AdminVariant {
  variant: string;
  name: string;
  version: string;
  description: string;
  active: boolean;
}
