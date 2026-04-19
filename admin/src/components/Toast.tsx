import { Message } from '@arco-design/web-react';
import '@arco-design/web-react/es/Message/style/css.js';

Message.config({
  maxCount: 3,
  duration: 5000,
});

export function showSuccess(message: string): void {
  Message.success({
    content: message,
    duration: 3000,
  });
}

export function showError(message: string): void {
  Message.error({
    content: message,
    duration: 8000,
  });
}

export function showWarning(message: string): void {
  Message.warning({
    content: message,
    duration: 5000,
  });
}

export function showInfo(message: string): void {
  Message.info({
    content: message,
    duration: 5000,
  });
}
