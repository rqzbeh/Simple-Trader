import { useEffect, useReducer } from 'react';

// Display timezone for every timestamp in the terminal. Operators work across
// regions: a signal stamped in UTC is meaningless to someone trading in UTC+8,
// so the choice is explicit and persists across reloads.

const STORAGE_KEY = 'simple_trader_timezone';

const DEFAULT_TZ = 'local';

export const TIMEZONES: { value: string; label: string }[] = [
  { value: 'local', label: 'Local' },
  { value: 'UTC', label: 'UTC+00:00' },
  { value: 'America/Los_Angeles', label: 'UTC-08:00 Los Angeles' },
  { value: 'America/Phoenix', label: 'UTC-07:00 Phoenix' },
  { value: 'America/Denver', label: 'UTC-07:00 Denver' },
  { value: 'America/Chicago', label: 'UTC-06:00 Chicago' },
  { value: 'America/New_York', label: 'UTC-05:00 New York' },
  { value: 'America/Caracas', label: 'UTC-04:00 Caracas' },
  { value: 'America/Sao_Paulo', label: 'UTC-03:00 Sao Paulo' },
  { value: 'America/St_Johns', label: 'UTC-03:30 St Johns' },
  { value: 'America/Noronha', label: 'UTC-02:00 Noronha' },
  { value: 'Atlantic/Azores', label: 'UTC-01:00 Azores' },
  { value: 'Europe/London', label: 'UTC+00:00 London' },
  { value: 'Europe/Lisbon', label: 'UTC+00:00 Lisbon' },
  { value: 'Europe/Paris', label: 'UTC+01:00 Paris' },
  { value: 'Europe/Berlin', label: 'UTC+01:00 Berlin' },
  { value: 'Europe/Warsaw', label: 'UTC+01:00 Warsaw' },
  { value: 'Europe/Athens', label: 'UTC+02:00 Athens' },
  { value: 'Africa/Cairo', label: 'UTC+02:00 Cairo' },
  { value: 'Africa/Johannesburg', label: 'UTC+02:00 Johannesburg' },
  { value: 'Europe/Istanbul', label: 'UTC+03:00 Istanbul' },
  { value: 'Europe/Moscow', label: 'UTC+03:00 Moscow' },
  { value: 'Africa/Nairobi', label: 'UTC+03:00 Nairobi' },
  { value: 'Asia/Tehran', label: 'UTC+03:30 Tehran' },
  { value: 'Asia/Dubai', label: 'UTC+04:00 Dubai' },
  { value: 'Asia/Riyadh', label: 'UTC+04:00 Riyadh' },
  { value: 'Asia/Kabul', label: 'UTC+04:30 Kabul' },
  { value: 'Asia/Karachi', label: 'UTC+05:00 Karachi' },
  { value: 'Asia/Tashkent', label: 'UTC+05:00 Tashkent' },
  { value: 'Asia/Kolkata', label: 'UTC+05:30 Kolkata (India)' },
  { value: 'Asia/Kathmandu', label: 'UTC+05:45 Kathmandu' },
  { value: 'Asia/Dhaka', label: 'UTC+06:00 Dhaka' },
  { value: 'Asia/Almaty', label: 'UTC+06:00 Almaty' },
  { value: 'Asia/Yangon', label: 'UTC+06:30 Yangon' },
  { value: 'Asia/Bangkok', label: 'UTC+07:00 Bangkok' },
  { value: 'Asia/Ho_Chi_Minh', label: 'UTC+07:00 Ho Chi Minh' },
  { value: 'Asia/Singapore', label: 'UTC+08:00 Singapore' },
  { value: 'Asia/Shanghai', label: 'UTC+08:00 Shanghai' },
  { value: 'Asia/Hong_Kong', label: 'UTC+08:00 Hong Kong' },
  { value: 'Asia/Taipei', label: 'UTC+08:00 Taipei' },
  { value: 'Asia/Manila', label: 'UTC+08:00 Manila' },
  { value: 'Asia/Tokyo', label: 'UTC+09:00 Tokyo' },
  { value: 'Australia/Perth', label: 'UTC+08:00 Perth' },
  { value: 'Australia/Sydney', label: 'UTC+10:00 Sydney' },
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


/**
 * lightweight-charts renders its time axis in UTC (it formats with getUTC*
 * internally), so candles showed UTC hours while every other timestamp on the
 * page followed the display timezone. These formatters put the chart in the
 * same zone as everything else.
 */

/** lightweight-charts passes UTCTimestamp in seconds, or a BusinessDay object. */
type ChartTimeValue = number | { year: number; month: number; day: number };

function chartDate(time: ChartTimeValue): Date | null {
  if (typeof time === 'number') return new Date(time * 1000);
  if (time && typeof time === 'object' && 'year' in time) {
    return new Date(Date.UTC(time.year, time.month - 1, time.day));
  }
  return null;
}

function zoneOptions(): Intl.DateTimeFormatOptions {
  return timezone === 'local' ? {} : { timeZone: timezone };
}

/** Axis tick label in the selected timezone (24h clock, trading convention). */
export function formatChartTick(time: ChartTimeValue, tickMarkType: number): string {
  const date = chartDate(time);
  if (!date) return '';
  const opts = zoneOptions();
  switch (tickMarkType) {
    case 0: // Year
      return date.toLocaleDateString('en-GB', { year: 'numeric', ...opts });
    case 1: // Month
      return date.toLocaleDateString('en-GB', { month: 'short', ...opts });
    case 2: // Day of month
      return date.toLocaleDateString('en-GB', { day: '2-digit', ...opts });
    default: // Time, TimeWithSeconds
      return date.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', ...opts });
  }
}

/** Crosshair label in the selected timezone. */
export function formatChartTime(time: ChartTimeValue): string {
  const date = chartDate(time);
  if (!date) return '';
  return date.toLocaleString('en-GB', {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    ...zoneOptions(),
  });
}
