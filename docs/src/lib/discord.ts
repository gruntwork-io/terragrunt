const CACHE_TTL_MS = 60 * 60 * 1000; // 1 hour in milliseconds
const FETCH_TIMEOUT_MS = 10 * 1000; // 10 second timeout for API requests

/**
 * The invite code that `/community/invite` redirects to (see astro.config.mjs).
 */
export const DISCORD_INVITE_CODE = 'YENaT9h8jh';

interface CacheEntry {
  count: number;
  timestamp: number;
}

const cache = new Map<string, CacheEntry>();

interface DiscordInviteResponse {
  approximate_member_count?: number;
  [key: string]: unknown;
}

/**
 * Fetches the approximate member count for a Discord server from one of its
 * invites. The invite lookup needs no token. Successful results are cached
 * for 1 hour so one build makes one request. Returns null on any failure so
 * the navbar can render the icon without a number.
 */
export async function getDiscordMemberCount(
  inviteCode: string
): Promise<number | null> {
  const now = Date.now();
  const cached = cache.get(inviteCode);
  if (cached && now - cached.timestamp < CACHE_TTL_MS) {
    return cached.count;
  }

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);

  try {
    const response = await fetch(
      `https://discord.com/api/v10/invites/${inviteCode}?with_counts=true`,
      {
        headers: {
          'User-Agent': 'Terragrunt-Docs',
        },
        signal: controller.signal,
      }
    );

    if (!response.ok) {
      console.error(
        `Failed to fetch Discord invite ${inviteCode}:`,
        response.status,
        await response.text()
      );
      return null;
    }

    const data = (await response.json()) as DiscordInviteResponse;
    const count = data.approximate_member_count;
    if (typeof count !== 'number' || !Number.isInteger(count) || count <= 0) {
      console.error(
        `Discord invite ${inviteCode} returned no member count:`,
        count
      );
      return null;
    }

    cache.set(inviteCode, { count, timestamp: now });
    return count;
  } catch (error) {
    console.error(`Error fetching Discord invite ${inviteCode}:`, error);
    return null;
  } finally {
    clearTimeout(timeoutId);
  }
}

/**
 * Formats a member count the way the terragrunt.com home page does: the exact
 * number with a thousands separator (e.g., 845 -> "845", 1204 -> "1,204").
 */
export function formatMemberCount(count: number): string {
  return count.toLocaleString('en-US');
}

/**
 * Clears the in-memory cache. Only used by tests.
 */
export function clearDiscordCache(): void {
  cache.clear();
}
