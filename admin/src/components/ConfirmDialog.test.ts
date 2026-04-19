import { describe, it, expect, vi, beforeEach } from 'vitest';

const { mockConfirm } = vi.hoisted(() => ({
  mockConfirm: vi.fn(),
}));

vi.mock('@arco-design/web-react', () => ({
  Modal: { confirm: mockConfirm },
}));

import { confirm } from './ConfirmDialog';

describe('ConfirmDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('calls Modal.confirm with title, content, and callbacks', () => {
    const onConfirm = vi.fn();
    confirm({ title: 'Delete?', message: 'Are you sure?', onConfirm });

    expect(mockConfirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: 'Delete?',
        content: 'Are you sure?',
        onOk: onConfirm,
        maskClosable: false,
        autoFocus: true,
      }),
    );
  });

  it('passes danger status when danger=true', () => {
    confirm({ title: 'T', message: 'M', onConfirm: vi.fn(), danger: true });
    expect(mockConfirm).toHaveBeenCalledWith(
      expect.objectContaining({
        okButtonProps: { status: 'danger' },
      }),
    );
  });

  it('omits okButtonProps when danger is false', () => {
    confirm({ title: 'T', message: 'M', onConfirm: vi.fn() });
    const call = mockConfirm.mock.calls[0][0];
    expect(call.okButtonProps).toBeUndefined();
  });

  it('uses custom confirm and cancel text', () => {
    confirm({ title: 'T', message: 'M', onConfirm: vi.fn(), confirmText: 'Yes', cancelText: 'No' });
    expect(mockConfirm).toHaveBeenCalledWith(
      expect.objectContaining({
        okText: 'Yes',
        cancelText: 'No',
      }),
    );
  });

  it('onOk callback is the provided onConfirm function', () => {
    const onConfirm = vi.fn();
    confirm({ title: 'T', message: 'M', onConfirm });
    const { onOk } = mockConfirm.mock.calls[0][0];
    onOk();
    expect(onConfirm).toHaveBeenCalled();
  });
});
