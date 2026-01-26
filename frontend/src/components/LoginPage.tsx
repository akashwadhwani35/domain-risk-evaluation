import { useState, FormEvent, useCallback } from 'react';
import { isAxiosError } from 'axios';
import { requestOTP, verifyOTP } from '../lib/api';
import { useAuth } from '../contexts/AuthContext';

type Step = 'email' | 'otp';

const ALLOWED_DOMAIN = '@radix.email';

export default function LoginPage() {
  const { setUser } = useAuth();
  const [step, setStep] = useState<Step>('email');
  const [email, setEmail] = useState('');
  const [code, setCode] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [resendCooldown, setResendCooldown] = useState(0);

  const validateEmail = useCallback((value: string): string | null => {
    const trimmed = value.trim().toLowerCase();
    if (!trimmed) {
      return 'Email is required';
    }
    if (!trimmed.endsWith(ALLOWED_DOMAIN)) {
      return `Email must end with ${ALLOWED_DOMAIN}`;
    }
    return null;
  }, []);

  const handleEmailSubmit = useCallback(async (e: FormEvent) => {
    e.preventDefault();
    setError(null);

    const validationError = validateEmail(email);
    if (validationError) {
      setError(validationError);
      return;
    }

    setLoading(true);
    try {
      await requestOTP(email.trim().toLowerCase());
      setStep('otp');
      setResendCooldown(30);

      // Start cooldown timer
      const interval = setInterval(() => {
        setResendCooldown((prev) => {
          if (prev <= 1) {
            clearInterval(interval);
            return 0;
          }
          return prev - 1;
        });
      }, 1000);
    } catch (err) {
      const message = isAxiosError(err) && err.response?.data?.error
        ? err.response.data.error
        : 'Failed to send verification code';
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [email, validateEmail]);

  const handleOTPSubmit = useCallback(async (e: FormEvent) => {
    e.preventDefault();
    setError(null);

    if (!code || code.length !== 6) {
      setError('Please enter the 6-digit code');
      return;
    }

    setLoading(true);
    try {
      const response = await verifyOTP(email.trim().toLowerCase(), code);
      if (response.user) {
        setUser(response.user);
      }
    } catch (err) {
      const message = isAxiosError(err) && err.response?.data?.error
        ? err.response.data.error
        : 'Invalid or expired code';
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [email, code, setUser]);

  const handleResend = useCallback(async () => {
    if (resendCooldown > 0) return;

    setError(null);
    setLoading(true);
    try {
      await requestOTP(email.trim().toLowerCase());
      setResendCooldown(30);

      const interval = setInterval(() => {
        setResendCooldown((prev) => {
          if (prev <= 1) {
            clearInterval(interval);
            return 0;
          }
          return prev - 1;
        });
      }, 1000);
    } catch (err) {
      const message = isAxiosError(err) && err.response?.data?.error
        ? err.response.data.error
        : 'Failed to resend code';
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [email, resendCooldown]);

  const handleChangeEmail = useCallback(() => {
    setStep('email');
    setCode('');
    setError(null);
  }, []);

  return (
    <div className="min-h-screen flex items-center justify-center bg-[var(--background)] px-4">
      <div className="w-full max-w-md">
        <div className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-8 shadow-lg">
          <div className="text-center mb-8">
            <h1 className="text-2xl font-semibold text-[var(--text)]">Domain Risk Evaluation</h1>
            <p className="text-[var(--muted)] mt-2">
              {step === 'email'
                ? 'Sign in with your @radix.email address'
                : 'Enter the verification code'}
            </p>
          </div>

          {error && (
            <div className="mb-6 rounded-lg bg-red-50 border border-red-200 px-4 py-3 text-red-700 text-sm">
              {error}
            </div>
          )}

          {step === 'email' ? (
            <form onSubmit={handleEmailSubmit} className="space-y-6">
              <div>
                <label htmlFor="email" className="block text-sm font-medium text-[var(--text)] mb-2">
                  Email Address
                </label>
                <input
                  id="email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="you@radix.email"
                  className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-2)] px-4 py-3 text-[var(--text)] placeholder:text-[var(--muted)] focus:border-[var(--accent)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-opacity-20"
                  disabled={loading}
                  autoFocus
                  autoComplete="email"
                />
                <p className="mt-2 text-xs text-[var(--muted)]">
                  Only @radix.email addresses are allowed
                </p>
              </div>

              <button
                type="submit"
                disabled={loading}
                className="w-full rounded-lg bg-[var(--text)] px-4 py-3 text-sm font-medium text-white hover:opacity-90 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {loading ? 'Sending...' : 'Continue'}
              </button>
            </form>
          ) : (
            <form onSubmit={handleOTPSubmit} className="space-y-6">
              <div>
                <label htmlFor="code" className="block text-sm font-medium text-[var(--text)] mb-2">
                  Verification Code
                </label>
                <input
                  id="code"
                  type="text"
                  inputMode="numeric"
                  pattern="[0-9]*"
                  maxLength={6}
                  value={code}
                  onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))}
                  placeholder="000000"
                  className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-2)] px-4 py-3 text-[var(--text)] text-center text-2xl tracking-[0.5em] placeholder:text-[var(--muted)] placeholder:tracking-[0.5em] focus:border-[var(--accent)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-opacity-20 font-mono"
                  disabled={loading}
                  autoFocus
                  autoComplete="one-time-code"
                />
                <p className="mt-2 text-xs text-[var(--muted)]">
                  We sent a code to <span className="font-medium">{email}</span>
                </p>
              </div>

              <button
                type="submit"
                disabled={loading || code.length !== 6}
                className="w-full rounded-lg bg-[var(--text)] px-4 py-3 text-sm font-medium text-white hover:opacity-90 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {loading ? 'Verifying...' : 'Sign In'}
              </button>

              <div className="flex items-center justify-between text-sm">
                <button
                  type="button"
                  onClick={handleChangeEmail}
                  className="text-[var(--muted)] hover:text-[var(--text)] transition-colors"
                >
                  Change email
                </button>
                <button
                  type="button"
                  onClick={handleResend}
                  disabled={resendCooldown > 0 || loading}
                  className="text-[var(--accent)] hover:opacity-80 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {resendCooldown > 0 ? `Resend in ${resendCooldown}s` : 'Resend code'}
                </button>
              </div>
            </form>
          )}
        </div>

        <p className="text-center text-xs text-[var(--muted)] mt-6">
          Trademark and vice risk assessment tool
        </p>
      </div>
    </div>
  );
}
