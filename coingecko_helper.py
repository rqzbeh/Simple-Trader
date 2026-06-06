def _coingecko_get(self, url: str, params: Optional[Dict[str, any]] = None, timeout: int = 20) -> Optional[requests.Response]:
    """
    Rate-limited GET request to CoinGecko with retry-on-429 and exponential backoff.
    Returns Response on success, None on permanent failure.
    """
    max_retries = 4
    for attempt in range(max_retries):
        # Enforce rate limit via minimum interval between requests
        now = time.time()
        elapsed = now - self._last_coingecko_call
        if elapsed < self._coingecko_min_interval:
            sleep_time = self._coingecko_min_interval - elapsed
            logger.debug("Rate limit: sleeping %.2fs before CoinGecko request", sleep_time)
            time.sleep(sleep_time)
        
        try:
            self._last_coingecko_call = time.time()
            r = self.session.get(url, params=params, timeout=timeout)
            
            # Handle 429 (too many requests) with backoff
            if r.status_code == 429:
                retry_after = r.headers.get("Retry-After", None)
                if retry_after:
                    try:
                        backoff_sec = int(retry_after)
                    except ValueError:
                        # Might be a date string; use exponential backoff instead
                        backoff_sec = 2 ** attempt
                else:
                    backoff_sec = 2 ** attempt
                
                logger.warning("CoinGecko returned 429; backing off for %.1fs (attempt %d/%d)", backoff_sec, attempt + 1, max_retries)
                if attempt < max_retries - 1:
                    time.sleep(backoff_sec)
                    continue
                else:
                    logger.error("CoinGecko 429 persisted after %d retries; giving up", max_retries)
                    return None
            
            # Handle 5xx server errors with backoff
            if r.status_code >= 500:
                backoff_sec = 2 ** attempt
                logger.warning("CoinGecko returned %s; backing off for %.1fs (attempt %d/%d)", r.status_code, backoff_sec, attempt + 1, max_retries)
                if attempt < max_retries - 1:
                    time.sleep(backoff_sec)
                    continue
                else:
                    logger.error("CoinGecko 5xx persisted after %d retries; giving up", max_retries)
                    return None
            
            # Success or client error (not retryable)
            return r
        
        except (requests.Timeout, requests.ConnectionError) as e:
            # Network errors: retry with exponential backoff
            backoff_sec = 2 ** attempt
            logger.warning("CoinGecko request error: %s; backing off for %.1fs (attempt %d/%d)", e, backoff_sec, attempt + 1, max_retries)
            if attempt < max_retries - 1:
                time.sleep(backoff_sec)
                continue
            else:
                logger.error("CoinGecko request failed persistently after %d retries: %s", max_retries, e)
                return None
        
        except Exception as e:
            logger.exception("Unexpected error during CoinGecko request")
            return None
    
    logger.error("CoinGecko GET request exhausted all retries for %s", url)
    return None
