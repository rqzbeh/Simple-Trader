export function getAuthToken(): string {
  try {
    return (
      sessionStorage.getItem('simple_trader_session_token') ||
      localStorage.getItem('simple_trader_session_token') ||
      ''
    );
  } catch {
    return '';
  }
}

export async function apiFetch<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> {
  const headers = new Headers(options.headers || {});
  if (!headers.has('Content-Type') && options.body && typeof options.body === 'string') {
    headers.set('Content-Type', 'application/json');
  }
  const token = getAuthToken();
  if (token && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${token}`);
  }

  const res = await fetch(endpoint, {
    ...options,
    headers,
    credentials: options.credentials || 'include',
  });

  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const errorMsg =
      (data as { error?: string })?.error ||
      `HTTP ${res.status}: Request to ${endpoint} failed`;
    throw new Error(errorMsg);
  }

  return data as T;
}
