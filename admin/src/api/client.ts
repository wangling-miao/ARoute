import type { ApiErrorResponse } from '@/types';

const BASE_URL = '/api/v1';
const LS_ACCESS  = 'aroute_access_token';
const LS_REFRESH = 'aroute_refresh_token';

// Restore tokens from localStorage on page load so refreshSession works after reload.
let accessToken: string | null  = localStorage.getItem(LS_ACCESS);
let refreshToken: string | null = localStorage.getItem(LS_REFRESH);

export function setTokens(access: string, refresh: string): void {
  accessToken  = access;
  refreshToken = refresh;
  localStorage.setItem(LS_ACCESS,  access);
  localStorage.setItem(LS_REFRESH, refresh);
}

export function getAccessToken(): string | null {
  return accessToken;
}

export function clearTokens(): void {
  accessToken  = null;
  refreshToken = null;
  localStorage.removeItem(LS_ACCESS);
  localStorage.removeItem(LS_REFRESH);
}

export function hasRefreshToken(): boolean {
  return refreshToken !== null;
}

class ApiError extends Error {
  code: string;
  details?: Record<string, unknown>;

  constructor(code: string, message: string, details?: Record<string, unknown>) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.details = details;
  }
}

export { ApiError };

let onAuthFailure: (() => void) | null = null;

export function setOnAuthFailure(callback: () => void): void {
  onAuthFailure = callback;
}

async function refreshAccessToken(): Promise<string> {
  if (!refreshToken) {
    throw new ApiError('NO_REFRESH_TOKEN', 'No refresh token available');
  }

  const response = await fetch(`${BASE_URL}/auth/refresh`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token: refreshToken }),
  });

  if (!response.ok) {
    clearTokens();
    onAuthFailure?.();
    throw new ApiError('REFRESH_FAILED', 'Token refresh failed');
  }

  const result = await response.json();
  const newAccessToken: string = result.data.access_token;
  accessToken = newAccessToken;
  return newAccessToken;
}

async function request<T>(
  path: string,
  options: RequestInit = {},
  retry = true,
): Promise<T> {
  const headers = new Headers(options.headers);

  if (!(options.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json');
  }

  if (accessToken) {
    headers.set('Authorization', `Bearer ${accessToken}`);
  }

  const response = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers,
  });

  if (response.status === 401 && retry && refreshToken) {
    try {
      const newToken = await refreshAccessToken();
      const retryHeaders = new Headers(options.headers);
      if (!(options.body instanceof FormData)) {
        retryHeaders.set('Content-Type', 'application/json');
      }
      retryHeaders.set('Authorization', `Bearer ${newToken}`);
      const retryResponse = await fetch(`${BASE_URL}${path}`, {
        ...options,
        headers: retryHeaders,
      });
      return handleResponse<T>(retryResponse);
    } catch {
      clearTokens();
      onAuthFailure?.();
      throw new ApiError('SESSION_EXPIRED', 'Session expired. Please log in again.');
    }
  }

  return handleResponse<T>(response);
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (response.status === 204) {
    return undefined as T;
  }

  const body = await response.json();

  if (!response.ok) {
    const errorResp = body as ApiErrorResponse;
    if (errorResp.errors && errorResp.errors.length > 0) {
      const first = errorResp.errors[0];
      throw new ApiError(first.code, first.message, first.details);
    }
    throw new ApiError('UNKNOWN_ERROR', `Request failed with status ${response.status}`);
  }

  // The API always wraps responses as { data: T, meta: {...} }.
  // Paginated list endpoints have a non-empty meta (page, total, …) — in that
  // case we return the full envelope so callers can access res.data[] and res.meta.
  // Single-resource endpoints have meta: {} — we unwrap and return body.data directly.
  if (
    body !== null &&
    typeof body === 'object' &&
    'meta' in body &&
    'data' in body &&
    typeof body.meta === 'object' &&
    body.meta !== null &&
    Object.keys(body.meta as object).length > 0
  ) {
    // Paginated response — return the whole envelope { data: [], meta: {…} }
    return body as T;
  }
  return (body.data ?? body) as T;
}

export const fetchClient = {
  get<T>(path: string, params?: Record<string, unknown>): Promise<T> {
    const url = params
      ? `${path}?${buildQuery(params)}`
      : path;
    return request<T>(url, { method: 'GET' });
  },

  post<T>(path: string, body?: unknown): Promise<T> {
    return request<T>(path, {
      method: 'POST',
      body: body instanceof FormData ? body : JSON.stringify(body),
    });
  },

  put<T>(path: string, body?: unknown): Promise<T> {
    return request<T>(path, {
      method: 'PUT',
      body: JSON.stringify(body),
    });
  },

  delete<T = void>(path: string): Promise<T> {
    return request<T>(path, { method: 'DELETE' });
  },

  upload<T>(path: string, file: File, onProgress?: (pct: number) => void): Promise<T> {
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open('POST', `${BASE_URL}${path}`);

      if (accessToken) {
        xhr.setRequestHeader('Authorization', `Bearer ${accessToken}`);
      }

      xhr.upload.addEventListener('progress', (e) => {
        if (e.lengthComputable && onProgress) {
          onProgress(Math.round((e.loaded / e.total) * 100));
        }
      });

      xhr.addEventListener('load', () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            const body = JSON.parse(xhr.responseText);
            resolve(body.data as T);
          } catch {
            reject(new ApiError('PARSE_ERROR', 'Failed to parse response'));
          }
        } else {
          try {
            const body = JSON.parse(xhr.responseText) as ApiErrorResponse;
            if (body.errors?.[0]) {
              reject(new ApiError(body.errors[0].code, body.errors[0].message));
            } else {
              reject(new ApiError('UPLOAD_FAILED', `Upload failed with status ${xhr.status}`));
            }
          } catch {
            reject(new ApiError('UPLOAD_FAILED', `Upload failed with status ${xhr.status}`));
          }
        }
      });

      xhr.addEventListener('error', () => {
        reject(new ApiError('NETWORK_ERROR', 'Network error during upload'));
      });

      const formData = new FormData();
      formData.append('file', file);
      xhr.send(formData);
    });
  },
};

function buildQuery(params: Record<string, unknown>): string {
  const parts: string[] = [];
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '') continue;
    if (typeof value === 'object') {
      parts.push(`${encodeURIComponent(key)}=${encodeURIComponent(JSON.stringify(value))}`);
    } else {
      parts.push(`${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`);
    }
  }
  return parts.join('&');
}
