import { fetchClient, setTokens, clearTokens } from './client';
import type {
  User,
  ContentItem,
  ContentType,
  CreateContentTypeRequest,
  UpdateContentTypeRequest,
  MediaFile,
  PaginatedResponse,
  ListParams,
  Plugin,
  Role,
  UpdateRoleRequest,
  Settings,
  DashboardStats,
  ApiToken,
  CreateTokenRequest,
  CreateUserRequest,
  UpdateUserRequest,
  AuthTokens,
  RefreshTokenResponse,
} from '@/types';

export type { User, ContentItem, ContentType, MediaFile, Plugin, Role, Settings, DashboardStats, ApiToken };

export const auth = {
  async login(email: string, password: string, remember?: boolean): Promise<AuthTokens> {
    const tokens = await fetchClient.post<AuthTokens>('/auth/login', { email, password });
    setTokens(tokens.access_token, tokens.refresh_token, remember);
    return tokens;
  },

  async refreshToken(refresh_token: string): Promise<RefreshTokenResponse> {
    return fetchClient.post<RefreshTokenResponse>('/auth/refresh', { refresh_token });
  },

  async getCurrentUser(): Promise<User> {
    return fetchClient.get<User>('/auth/me');
  },

  logout(): void {
    clearTokens();
  },
};

export const content = {
  list(contentType: string, params?: ListParams): Promise<PaginatedResponse<ContentItem>> {
    return fetchClient.get<PaginatedResponse<ContentItem>>(`/content/${contentType}`, params as Record<string, unknown>);
  },

  get(contentType: string, id: string): Promise<ContentItem> {
    return fetchClient.get<ContentItem>(`/content/${contentType}/${id}`);
  },

  create(contentType: string, data: Record<string, unknown>): Promise<ContentItem> {
    return fetchClient.post<ContentItem>(`/content/${contentType}`, data);
  },

  update(contentType: string, id: string, data: Record<string, unknown>): Promise<ContentItem> {
    return fetchClient.put<ContentItem>(`/content/${contentType}/${id}`, data);
  },

  delete(contentType: string, id: string): Promise<void> {
    return fetchClient.delete(`/content/${contentType}/${id}`);
  },
};

export const contentTypes = {
  list(): Promise<ContentType[]> {
    return fetchClient.get<ContentType[]>('/content-types');
  },

  get(name: string): Promise<ContentType> {
    return fetchClient.get<ContentType>(`/content-types/${name}`);
  },

  create(data: CreateContentTypeRequest): Promise<ContentType> {
    return fetchClient.post<ContentType>('/content-types', data);
  },

  update(name: string, data: UpdateContentTypeRequest): Promise<ContentType> {
    return fetchClient.put<ContentType>(`/content-types/${name}`, data);
  },

  delete(name: string): Promise<void> {
    return fetchClient.delete(`/content-types/${name}`);
  },
};

export const media = {
  upload(file: File, onProgress?: (pct: number) => void): Promise<MediaFile> {
    return fetchClient.upload<MediaFile>('/media', file, onProgress);
  },

  list(params?: ListParams): Promise<PaginatedResponse<MediaFile>> {
    return fetchClient.get<PaginatedResponse<MediaFile>>('/media', params as Record<string, unknown>);
  },

  delete(id: string): Promise<void> {
    return fetchClient.delete(`/media/${id}`);
  },
};

export const users = {
  list(params?: ListParams): Promise<PaginatedResponse<User>> {
    return fetchClient.get<PaginatedResponse<User>>('/users', params as Record<string, unknown>);
  },

  create(data: CreateUserRequest): Promise<User> {
    return fetchClient.post<User>('/users', data);
  },

  update(id: string, data: UpdateUserRequest): Promise<User> {
    return fetchClient.put<User>(`/users/${id}`, data);
  },

  delete(id: string): Promise<void> {
    return fetchClient.delete(`/users/${id}`);
  },
};

export const roles = {
  list(): Promise<Role[]> {
    return fetchClient.get<Role[]>('/roles');
  },

  update(id: string, data: UpdateRoleRequest): Promise<Role> {
    return fetchClient.put<Role>(`/roles/${id}`, data);
  },
};

export const plugins = {
  list(): Promise<Plugin[]> {
    return fetchClient.get<Plugin[]>('/plugins');
  },

  enable(name: string): Promise<void> {
    return fetchClient.post<void>(`/plugins/${name}/enable`);
  },

  disable(name: string): Promise<void> {
    return fetchClient.post<void>(`/plugins/${name}/disable`);
  },

  upload(file: File, onProgress?: (pct: number) => void): Promise<Plugin> {
    return fetchClient.upload<Plugin>('/plugins/upload', file, onProgress);
  },
};

export const settings = {
  get(): Promise<Settings> {
    return fetchClient.get<Settings>('/settings');
  },

  update(data: Partial<Settings>): Promise<Settings> {
    return fetchClient.put<Settings>('/settings', data);
  },
};

export const site = {
  getInfo(): Promise<{ site_name: string; site_url: string }> {
    return fetchClient.get<{ site_name: string; site_url: string }>('/site/info');
  },
};

export const dashboard = {
  getStats(): Promise<DashboardStats> {
    return fetchClient.get<DashboardStats>('/dashboard/stats');
  },
};

export const apiTokens = {
  list(): Promise<ApiToken[]> {
    return fetchClient.get<ApiToken[]>('/api-tokens');
  },

  create(data: CreateTokenRequest): Promise<{ token: string }> {
    return fetchClient.post<{ token: string }>('/api-tokens', data);
  },

  revoke(id: string): Promise<void> {
    return fetchClient.delete(`/api-tokens/${id}`);
  },
};
