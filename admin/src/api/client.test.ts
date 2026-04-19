import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { fetchClient, ApiError, setTokens, clearTokens, getAccessToken, hasRefreshToken, setOnAuthFailure } from './client';

describe('client', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    clearTokens();
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  function mockFetchOnce(data: unknown, status = 200) {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: status >= 200 && status < 300,
      status,
      json: () => Promise.resolve(data),
    });
  }

  describe('setTokens / clearTokens / getAccessToken / hasRefreshToken', () => {
    it('stores and clears tokens', () => {
      expect(getAccessToken()).toBeNull();
      expect(hasRefreshToken()).toBe(false);

      setTokens('access-123', 'refresh-456');
      expect(getAccessToken()).toBe('access-123');
      expect(hasRefreshToken()).toBe(true);

      clearTokens();
      expect(getAccessToken()).toBeNull();
      expect(hasRefreshToken()).toBe(false);
    });
  });

  describe('fetchClient.get', () => {
    it('makes GET request and returns data', async () => {
      mockFetchOnce({ data: { id: '1', name: 'test' } });

      const result = await fetchClient.get<{ id: string; name: string }>('/items');
      expect(result).toEqual({ id: '1', name: 'test' });
      expect(globalThis.fetch).toHaveBeenCalledWith(
        '/api/v1/items',
        expect.objectContaining({ method: 'GET' }),
      );
    });

    it('appends query params when provided', async () => {
      mockFetchOnce({ data: [] });

      await fetchClient.get('/items', { page: 1, search: 'hello' });
      const calledUrl = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
      expect(calledUrl).toContain('page=1');
      expect(calledUrl).toContain('search=hello');
    });

    it('includes Authorization header when token is set', async () => {
      setTokens('my-access', 'my-refresh');
      mockFetchOnce({ data: {} });

      await fetchClient.get('/items');
      const opts = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][1] as RequestInit;
      expect((opts.headers as Headers).get('Authorization')).toBe('Bearer my-access');
    });

    it('skips null/undefined/empty params in query string', async () => {
      mockFetchOnce({ data: [] });

      await fetchClient.get('/items', { a: undefined, b: null, c: '', d: 'keep' });
      const calledUrl = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
      expect(calledUrl).not.toContain('a=');
      expect(calledUrl).not.toContain('b=');
      expect(calledUrl).not.toContain('c=');
      expect(calledUrl).toContain('d=keep');
    });

    it('serializes object params as JSON', async () => {
      mockFetchOnce({ data: [] });

      await fetchClient.get('/items', { filter: { status: 'active' } });
      const calledUrl = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
      expect(calledUrl).toContain('filter=');
      const filterVal = calledUrl.split('filter=')[1];
      expect(JSON.parse(decodeURIComponent(filterVal))).toEqual({ status: 'active' });
    });
  });

  describe('fetchClient.post', () => {
    it('makes POST request with JSON body', async () => {
      mockFetchOnce({ data: { id: '1' } });

      const result = await fetchClient.post<{ id: string }>('/items', { name: 'test' });
      expect(result).toEqual({ id: '1' });
      const opts = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][1] as RequestInit;
      expect(opts.method).toBe('POST');
      expect(opts.body).toBe(JSON.stringify({ name: 'test' }));
    });

    it('sends FormData without Content-Type header override', async () => {
      mockFetchOnce({ data: { id: '1' } });
      const fd = new FormData();
      fd.append('file', 'data');

      await fetchClient.post('/upload', fd);
      const opts = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][1] as RequestInit;
      expect(opts.body).toBe(fd);
    });
  });

  describe('fetchClient.put', () => {
    it('makes PUT request with JSON body', async () => {
      mockFetchOnce({ data: { id: '1', name: 'updated' } });

      const result = await fetchClient.put<{ id: string; name: string }>('/items/1', { name: 'updated' });
      expect(result).toEqual({ id: '1', name: 'updated' });
      const opts = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][1] as RequestInit;
      expect(opts.method).toBe('PUT');
    });
  });

  describe('fetchClient.delete', () => {
    it('makes DELETE request', async () => {
      mockFetchOnce({ data: null });

      await fetchClient.delete('/items/1');
      const opts = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][1] as RequestInit;
      expect(opts.method).toBe('DELETE');
    });
  });

  describe('ApiError', () => {
    it('is thrown on error response with code and message', async () => {
      (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
        ok: false,
        status: 400,
        json: () => Promise.resolve({
          errors: [{ code: 'VALIDATION_ERROR', message: 'Invalid input', details: { field: 'email' } }],
        }),
      });

      try {
        await fetchClient.get('/items');
        expect.fail('Should have thrown');
      } catch (err) {
        expect(err).toBeInstanceOf(ApiError);
        expect((err as ApiError).code).toBe('VALIDATION_ERROR');
        expect((err as ApiError).message).toBe('Invalid input');
        expect((err as ApiError).details).toEqual({ field: 'email' });
      }
    });

    it('is thrown with UNKNOWN_ERROR when errors array is empty', async () => {
      (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ errors: [] }),
      });

      try {
        await fetchClient.get('/items');
        expect.fail('Should have thrown');
      } catch (err) {
        expect(err).toBeInstanceOf(ApiError);
        expect((err as ApiError).code).toBe('UNKNOWN_ERROR');
        expect((err as ApiError).message).toContain('500');
      }
    });

    it('has correct name property', () => {
      const err = new ApiError('TEST', 'test message');
      expect(err.name).toBe('ApiError');
    });
  });

  describe('token refresh on 401', () => {
    it('attempts token refresh on 401 and retries the request', async () => {
      setTokens('expired-token', 'valid-refresh');

      // First call: 401
      (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ errors: [{ code: 'UNAUTHORIZED', message: 'Token expired' }] }),
      });

      // Refresh call
      (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ data: { access_token: 'new-token' } }),
      });

      // Retry call
      (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ data: { id: '1' } }),
      });

      const result = await fetchClient.get('/items');
      expect(result).toEqual({ id: '1' });
      expect(globalThis.fetch).toHaveBeenCalledTimes(3);
      expect(getAccessToken()).toBe('new-token');
    });

    it('clears tokens and fires onAuthFailure when refresh fails', async () => {
      setTokens('expired-token', 'bad-refresh');
      const authFailure = vi.fn();
      setOnAuthFailure(authFailure);

      // First call: 401
      (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ errors: [{ code: 'UNAUTHORIZED', message: 'Token expired' }] }),
      });

      // Refresh call: fails
      (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ errors: [{ code: 'UNAUTHORIZED', message: 'Bad refresh' }] }),
      });

      await expect(fetchClient.get('/items')).rejects.toThrow('Session expired');
      expect(hasRefreshToken()).toBe(false);
      expect(authFailure).toHaveBeenCalled();
    });
  });
});
