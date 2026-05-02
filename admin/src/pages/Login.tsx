import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Input, Button, Checkbox } from '@arco-design/web-react';
import { Mail, Lock, AlertCircle } from 'lucide-react';
import { useAuth } from '@/contexts/AuthContext';
import { ApiError } from '@/api/client';
import styles from './Login.module.css';

export default function Login() {
  const { t } = useTranslation();
  const { login } = useAuth();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [rememberMe, setRememberMe] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!email.trim() || !password) return;

    setError('');
    setLoading(true);

    try {
      await login(email.trim(), password, rememberMe);
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.code === 'NETWORK_ERROR') {
          setError(t('login.error_network'));
        } else if (err.code === 'FORBIDDEN') {
          setError(err.message || t('login.error_forbidden'));
        } else {
          setError(t('login.error_invalid'));
        }
      } else {
        setError(t('login.error_invalid'));
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className={`${styles.loginPage} animate-fade-in-up`}>
      <div className={`${styles.bgOrb} ${styles.bgOrb1}`} />
      <div className={`${styles.bgOrb} ${styles.bgOrb2}`} />
      <div className={`${styles.bgOrb} ${styles.bgOrb3}`} />

      <div className={styles.loginCard}>
        <div className={styles.brandSection}>
          <div className={styles.brandLogo}>ARoute</div>
          <p className={styles.brandTagline}>{t('login.subtitle')}</p>
        </div>

        {error && (
          <div className={`${styles.errorBox} animate-scale-in`}>
            <AlertCircle size={16} className={styles.errorBoxIcon} />
            <span>{error}</span>
          </div>
        )}

        <form onSubmit={handleSubmit}>
          <div className={styles.formField}>
            <label className={styles.formLabel} htmlFor="login-email">{t('login.email')}</label>
            <div className={styles.inputWrap}>
              <Mail size={18} className={styles.inputIcon} />
              <Input
                id="login-email"
                size="large"
                value={email}
                onChange={setEmail}
                placeholder="admin@example.com"
                autoComplete="email"
              />
            </div>
          </div>

          <div className={styles.formField}>
            <label className={styles.formLabel} htmlFor="login-password">{t('login.password')}</label>
            <div className={styles.inputWrap}>
              <Lock size={18} className={styles.inputIcon} />
              <Input.Password
                id="login-password"
                size="large"
                value={password}
                onChange={setPassword}
                placeholder="••••••••"
                autoComplete="current-password"
              />
            </div>
          </div>

          <div className={styles.formActions}>
            <Checkbox
              checked={rememberMe}
              onChange={(val: boolean) => setRememberMe(val)}
            >
              {t('login.remember_me')}
            </Checkbox>
          </div>

          <Button
            type="primary"
            htmlType="submit"
            size="large"
            long
            loading={loading}
            className={styles.submitBtn}
          >
            {t('login.submit')}
          </Button>
        </form>
      </div>
    </div>
  );
}
