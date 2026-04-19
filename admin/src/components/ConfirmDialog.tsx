import { Modal } from '@arco-design/web-react';
import '@arco-design/web-react/es/Modal/style/css.js';

interface ConfirmOptions {
  title: string;
  message: string;
  onConfirm: () => void | Promise<void>;
  danger?: boolean;
  confirmText?: string;
  cancelText?: string;
}

export function confirm({
  title,
  message,
  onConfirm,
  danger = false,
  confirmText = 'Confirm',
  cancelText = 'Cancel',
}: ConfirmOptions): void {
  Modal.confirm({
    title,
    content: message,
    okText: confirmText,
    cancelText,
    okButtonProps: danger
      ? { status: 'danger' }
      : undefined,
    onOk: onConfirm,
    simple: false,
    autoFocus: true,
    maskClosable: false,
  });
}
