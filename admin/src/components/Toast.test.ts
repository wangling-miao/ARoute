import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('@arco-design/web-react', () => ({
  Message: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
    config: vi.fn(),
  },
}));

import { Message } from '@arco-design/web-react';
import { showSuccess, showError, showWarning } from './Toast';

describe('Toast helpers', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('showSuccess calls Message.success with content and 3s duration', () => {
    showSuccess('Saved!');
    expect(Message.success).toHaveBeenCalledWith({ content: 'Saved!', duration: 3000 });
  });

  it('showError calls Message.error with content and 8s duration', () => {
    showError('Something failed');
    expect(Message.error).toHaveBeenCalledWith({ content: 'Something failed', duration: 8000 });
  });

  it('showWarning calls Message.warning with content and 5s duration', () => {
    showWarning('Check this');
    expect(Message.warning).toHaveBeenCalledWith({ content: 'Check this', duration: 5000 });
  });
});
