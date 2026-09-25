import { useEffect, useReducer } from 'react';

// Display timezone for every timestamp in the terminal. Operators work across
// regions: a signal stamped in UTC is meaningless to someone trading in UTC+8,
// so the choice is explicit and persists across reloads.

const STORAGE_KEY = 'simple_trader_timezone';

const DEFAULT_TZ = 'local';

export const TIMEZONES: { value: string; label: string }[] = [
  { value: 'local', label: 'Local' },
  { value: 'UTC', label: 'UTC' },
  { value: 'America/New_York', label: 'New York (ET)' },
  { value: 'America/Chicago', label: 'Chicago (CT)' },
  { value: 'America/Los_Angeles', label: 'Los Angeles (PT)' },
  { value: 'Europe/London', label: 'London (GMT/BST)' },
  { value: 'Europe/Berlin', label: 'Berlin (CET)' },
  { value: 'Europe/Moscow', label: 'Moscow (MSK)' },
  { value: 'Asia/Dubai', label: 'Dubai (GST)' },
  { value: 'Asia/Kolkata', label: 'Kolkata (IST)' },
  { value: 'Asia/Singapore', label: 'Singapore (SGT)' },
  { value: 'Asia/Shanghai', label: 'Shanghai (CST)' },
  { value: 'Asia/Tokyo', label: 'Tokyo (JST)' },
  { value: 'Australia/Sydney', label: 'Sydney (AEST)' },
];

function readStored(): string {
  if (typeof window === 'undefined') return DEFAULT_TZ;
  const stored = window.localStorage.getItem(STORAGE_KEY);
  if (!stored) return DEFAULT_TZ;
  // Reject values the runtime does not know, otherwise Intl throws at format time
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: stored === 'local' ? undefined : stored });
    return stored;
  } catch {
    return DEFAULT_TZ;
  }
}

let timezone = readStored();
const listeners = new Set<() => void>();

export function getTimezone(): string {
  return timezone;
}

export function setTimezone(next: string): void {
  timezone = next;
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(STORAGE_KEY, next);
  }
  listeners.forEach((notify) => notify());
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Re-renders the component whenever the display timezone changes. */
export function useTimezone(): string {
  const [, forceUpdate] = useReducer((tick: number) => tick + 1, 0);
  useEffect(() => subscribe(forceUpdate), []);
  return timezone;
}

function options(): Intl.DateTimeFormatOptions {
  const tz = timezone === 'local' ? undefined : timezone;
  return { timeZone: tz };
}

/** "08:11 AM" in the selected timezone. */
export function formatTime(value: string | number | Date): string {
  return new Date(value).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    ...options(),
  });
}

/** "2026-09-25 08:11 AM" in the selected timezone. */
export function formatDateTime(value: string | number | Date): string {
  return new Date(value).toLocaleString([], {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    ...options(),
  });
}

/** Short zone label for headers, e.g. "CST" or "UTC+8". */
export function timezoneLabel(): string {
  if (timezone === 'local') {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'local';
  }
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZoneName: 'shortOffset',
    ...options(),
  }).formatToParts(new Date());
  const offset = parts.find((part) => part.type === 'timeZoneName')?.value;
  return offset || timezone;
}
