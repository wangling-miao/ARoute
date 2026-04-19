import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('./client', () => ({
  fetchClient: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
    upload: vi.fn(),
  },
  setTokens: vi.fn(),
  clearTokens: vi.fn(),
}));

import { fetchClient, setTokens, clearTokens } from './client';
import { auth, content, contentTypes, media, users, roles, plugins, settings, dashboard, apiTokens } from './endpoints';

beforeEach(() => {
  vi.clearAllMocks();
});

describe('auth endpoints', () => {
  it('login calls POST /auth/login and sets tokens', async () => {
    const tokens = { access_token: 'a', refresh_token: 'r' };
    (fetchClient.post as ReturnType<typeof vi.fn>).mockResolvedValueOnce(tokens);

    const result = await auth.login('admin@test.com', 'password');
    expect(fetchClient.post).toHaveBeenCalledWith('/auth/login', { email: 'admin@test.com', password: 'password' });
    expect(setTokens).toHaveBeenCalledWith('a', 'r');
    expect(result).toEqual(tokens);
  });

  it('refreshToken calls POST /auth/refresh', async () => {
    const resp = { access_token: 'new' };
    (fetchClient.post as ReturnType<typeof vi.fn>).mockResolvedValueOnce(resp);

    const result = await auth.refreshToken('old-refresh');
    expect(fetchClient.post).toHaveBeenCalledWith('/auth/refresh', { refresh_token: 'old-refresh' });
    expect(result).toEqual(resp);
  });

  it('getCurrentUser calls GET /auth/me', async () => {
    const user = { id: '1', username: 'admin' };
    (fetchClient.get as ReturnType<typeof vi.fn>).mockResolvedValueOnce(user);

    const result = await auth.getCurrentUser();
    expect(fetchClient.get).toHaveBeenCalledWith('/auth/me');
    expect(result).toEqual(user);
  });

  it('logout calls clearTokens', () => {
    auth.logout();
    expect(clearTokens).toHaveBeenCalled();
  });
});

describe('content endpoints', () => {
  it('list calls GET /content/:type with params', async () => {
    const resp = { data: [], meta: { total: 0, page: 1, per_page: 20, total_pages: 0 } };
    (fetchClient.get as ReturnType<typeof vi.fn>).mockResolvedValueOnce(resp);

    await content.list('posts', { page: 1, per_page: 20 });
    expect(fetchClient.get).toHaveBeenCalledWith('/content/posts', { page: 1, per_page: 20 });
  });

  it('get calls GET /content/:type/:id', async () => {
    (fetchClient.get as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ id: '1' });
    await content.get('posts', '1');
    expect(fetchClient.get).toHaveBeenCalledWith('/content/posts/1');
  });

  it('create calls POST /content/:type', async () => {
    (fetchClient.post as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ id: '2' });
    await content.create('posts', { title: 'Hello' });
    expect(fetchClient.post).toHaveBeenCalledWith('/content/posts', { title: 'Hello' });
  });

  it('update calls PUT /content/:type/:id', async () => {
    (fetchClient.put as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ id: '1' });
    await content.update('posts', '1', { title: 'Updated' });
    expect(fetchClient.put).toHaveBeenCalledWith('/content/posts/1', { title: 'Updated' });
  });

  it('delete calls DELETE /content/:type/:id', async () => {
    (fetchClient.delete as ReturnType<typeof vi.fn>).mockResolvedValueOnce(undefined);
    await content.delete('posts', '1');
    expect(fetchClient.delete).toHaveBeenCalledWith('/content/posts/1');
  });
});

describe('contentTypes endpoints', () => {
  it('list calls GET /content-types', async () => {
    (fetchClient.get as ReturnType<typeof vi.fn>).mockResolvedValueOnce([]);
    await contentTypes.list();
    expect(fetchClient.get).toHaveBeenCalledWith('/content-types');
  });

  it('create calls POST /content-types', async () => {
    (fetchClient.post as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ name: 'posts' });
    await contentTypes.create({ name: 'posts', display_name: 'Posts', description: '', fields: [] as any });
    expect(fetchClient.post).toHaveBeenCalledWith('/content-types', expect.objectContaining({ name: 'posts' }));
  });
});

describe('media endpoints', () => {
  it('upload calls fetchClient.upload', async () => {
    const file = new File(['x'], 'test.png', { type: 'image/png' });
    const onProgress = vi.fn();
    (fetchClient.upload as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ id: '1' });

    await media.upload(file, onProgress);
    expect(fetchClient.upload).toHaveBeenCalledWith('/media', file, onProgress);
  });

  it('list calls GET /media', async () => {
    (fetchClient.get as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ data: [], meta: {} });
    await media.list({ page: 1 });
    expect(fetchClient.get).toHaveBeenCalledWith('/media', { page: 1 });
  });

  it('delete calls DELETE /media/:id', async () => {
    (fetchClient.delete as ReturnType<typeof vi.fn>).mockResolvedValueOnce(undefined);
    await media.delete('1');
    expect(fetchClient.delete).toHaveBeenCalledWith('/media/1');
  });
});

describe('users endpoints', () => {
  it('list calls GET /users', async () => {
    (fetchClient.get as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ data: [], meta: {} });
    await users.list();
    expect(fetchClient.get).toHaveBeenCalledWith('/users', undefined);
  });

  it('create calls POST /users', async () => {
    (fetchClient.post as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ id: '1' });
    await users.create({ username: 'u', email: 'e@e.com', password: 'p', roles: [] });
    expect(fetchClient.post).toHaveBeenCalledWith('/users', expect.objectContaining({ username: 'u' }));
  });
});

describe('roles endpoints', () => {
  it('list calls GET /roles', async () => {
    (fetchClient.get as ReturnType<typeof vi.fn>).mockResolvedValueOnce([]);
    await roles.list();
    expect(fetchClient.get).toHaveBeenCalledWith('/roles');
  });

  it('update calls PUT /roles/:id', async () => {
    (fetchClient.put as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ id: '1' });
    await roles.update('1', { description: 'updated' });
    expect(fetchClient.put).toHaveBeenCalledWith('/roles/1', { description: 'updated' });
  });
});

describe('plugins endpoints', () => {
  it('list calls GET /plugins', async () => {
    (fetchClient.get as ReturnType<typeof vi.fn>).mockResolvedValueOnce([]);
    await plugins.list();
    expect(fetchClient.get).toHaveBeenCalledWith('/plugins');
  });

  it('enable calls POST /plugins/:name/enable', async () => {
    (fetchClient.post as ReturnType<typeof vi.fn>).mockResolvedValueOnce(undefined);
    await plugins.enable('my-plugin');
    expect(fetchClient.post).toHaveBeenCalledWith('/plugins/my-plugin/enable');
  });

  it('disable calls POST /plugins/:name/disable', async () => {
    (fetchClient.post as ReturnType<typeof vi.fn>).mockResolvedValueOnce(undefined);
    await plugins.disable('my-plugin');
    expect(fetchClient.post).toHaveBeenCalledWith('/plugins/my-plugin/disable');
  });
});

describe('settings endpoints', () => {
  it('get calls GET /settings', async () => {
    (fetchClient.get as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ site_name: 'Test' });
    const result = await settings.get();
    expect(fetchClient.get).toHaveBeenCalledWith('/settings');
    expect(result).toEqual({ site_name: 'Test' });
  });

  it('update calls PUT /settings', async () => {
    (fetchClient.put as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ site_name: 'Updated' });
    await settings.update({ site_name: 'Updated' });
    expect(fetchClient.put).toHaveBeenCalledWith('/settings', { site_name: 'Updated' });
  });
});

describe('dashboard endpoints', () => {
  it('getStats calls GET /dashboard/stats', async () => {
    const stats = { content_counts: {}, recent_activity: [], system_status: { database: 'healthy', plugin_count: 5, cache_hit_ratio: 0.9 } };
    (fetchClient.get as ReturnType<typeof vi.fn>).mockResolvedValueOnce(stats);
    const result = await dashboard.getStats();
    expect(fetchClient.get).toHaveBeenCalledWith('/dashboard/stats');
    expect(result).toEqual(stats);
  });
});

describe('apiTokens endpoints', () => {
  it('list calls GET /api-tokens', async () => {
    (fetchClient.get as ReturnType<typeof vi.fn>).mockResolvedValueOnce([]);
    await apiTokens.list();
    expect(fetchClient.get).toHaveBeenCalledWith('/api-tokens');
  });

  it('create calls POST /api-tokens', async () => {
    (fetchClient.post as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ token: 'tok_abc' });
    const result = await apiTokens.create({ name: 'CI' });
    expect(fetchClient.post).toHaveBeenCalledWith('/api-tokens', { name: 'CI' });
    expect(result).toEqual({ token: 'tok_abc' });
  });

  it('revoke calls DELETE /api-tokens/:id', async () => {
    (fetchClient.delete as ReturnType<typeof vi.fn>).mockResolvedValueOnce(undefined);
    await apiTokens.revoke('1');
    expect(fetchClient.delete).toHaveBeenCalledWith('/api-tokens/1');
  });
});
