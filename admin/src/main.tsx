import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { setCreateRoot } from '@arco-design/web-react/es/_util/react-dom';
import App from './App';

// Patch Arco Design's internal render shim to use React 19's createRoot API.
// This MUST run before any Arco imperative APIs (Message.error, Modal.confirm…)
// are called. The old side-effect-only import can be tree-shaken by Vite;
// calling setCreateRoot() explicitly is guaranteed to execute.
setCreateRoot(createRoot);

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
