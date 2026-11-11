/**
 * Asks the community for feedback on open RFCs.
 *
 * Ranks open issues labelled `rfc` by 👍 reactions and posts the most and
 * least supported ones to a Discord webhook.
 *
 * Loaded by `github-script`, which run on Node 24 and
 * type-strip this file, so imports must carry explicit extensions and the
 * syntax must stay erasable.
 */

export type Core = {
  info(msg: string): void;
  setFailed(msg: string): void;
};

export type Context = {
  repo: { owner: string; repo: string };
};

export type Issue = {
  number: number;
  title: string;
  html_url: string;
  updated_at: string;
  pull_request?: unknown;
  reactions?: { '+1'?: number };
};

export type GithubClient = {
  paginate(method: unknown, opts: unknown): Promise<Issue[]>;
  rest: { issues: { listForRepo: unknown } };
};

export type Fetch = (
  url: string,
  init: { method: string; headers: Record<string, string>; body: string },
) => Promise<{ ok: boolean; status: number }>;

export type RFC = {
  number: number;
  title: string;
  url: string;
  thumbsUp: number;
  updatedAt: string;
};

export type Selection = { most: RFC[]; least: RFC[] };

export const RFC_LABEL = 'rfc';

export const SELECTION_SIZE = 3;

export const MAX_TITLE_LENGTH = 100;

/** Discord's `SUPPRESS_EMBEDS` message flag. */
const SUPPRESS_EMBEDS = 4;

export default async function requestRFCFeedback({
  github,
  context,
  core,
  fetch,
  webhookUrl,
  dryRun,
}: {
  github: GithubClient;
  context: Context;
  core: Core;
  fetch: Fetch;
  webhookUrl: string;
  dryRun: boolean;
}) {
  const { owner, repo } = context.repo;

  try {
    const rfcs = await listOpenRFCs({ github, owner, repo });
    if (rfcs.length === 0) {
      core.info('No open RFCs found. Skipping.');
      return;
    }

    const message = buildMessage(selectRFCs(rfcs));
    core.info(message);

    if (dryRun) {
      core.info('Dry run. Skipping the Discord post.');
      return;
    }

    if (!webhookUrl) {
      core.setFailed('The Discord webhook URL is not set.');
      return;
    }

    await postToDiscord({ fetch, webhookUrl, message });
    core.info(`Asked for feedback on ${rfcs.length} open RFC(s).`);
  } catch (error) {
    core.setFailed(`Failed to request RFC feedback: ${error instanceof Error ? error.message : String(error)}`);
  }
}

/**
 * Lists open issues labelled `rfc` with their 👍 counts.
 */
export async function listOpenRFCs({
  github,
  owner,
  repo,
}: {
  github: GithubClient;
  owner: string;
  repo: string;
}): Promise<RFC[]> {
  const issues = await github.paginate(github.rest.issues.listForRepo, {
    owner,
    repo,
    labels: RFC_LABEL,
    state: 'open',
    per_page: 100,
  });

  return issues
    // The issues API also returns pull requests.
    .filter((issue) => issue.pull_request === undefined)
    .map((issue) => ({
      number: issue.number,
      title: issue.title,
      url: issue.html_url,
      thumbsUp: issue.reactions?.['+1'] ?? 0,
      updatedAt: issue.updated_at,
    }));
}

/**
 * Picks the RFCs with the most and the fewest 👍 reactions.
 *
 * An RFC appears in one list only, and `most` fills first. Among the least
 * supported, a tie goes to the RFC that has gone longest without an update.
 */
export function selectRFCs(rfcs: RFC[]): Selection {
  const most = [...rfcs]
    .sort((a, b) => b.thumbsUp - a.thumbsUp || a.number - b.number)
    .slice(0, SELECTION_SIZE);

  const selected = new Set(most.map((rfc) => rfc.number));

  const least = rfcs
    .filter((rfc) => !selected.has(rfc.number))
    .sort(
      (a, b) =>
        a.thumbsUp - b.thumbsUp ||
        Date.parse(a.updatedAt) - Date.parse(b.updatedAt) ||
        a.number - b.number,
    )
    .slice(0, SELECTION_SIZE);

  return { most, least };
}

/**
 * Builds the Discord message for the selected RFCs.
 */
export function buildMessage({ most, least }: Selection): string {
  const lines = [
    '**RFC feedback wanted**',
    '',
    'These open RFCs have the most 👍. What would make them work for you?',
    ...most.map(formatRFC),
  ];

  if (least.length > 0) {
    lines.push(
      '',
      'These have the fewest. Would you use them?',
      ...least.map(formatRFC),
    );
  }

  lines.push('', 'Add a 👍 or a comment on the issue to weigh in.');

  return lines.join('\n');
}

/**
 * Posts the message to the Discord webhook.
 *
 * Errors omit the webhook URL and the underlying fetch error, which can
 * quote it.
 */
export async function postToDiscord({
  fetch,
  webhookUrl,
  message,
}: {
  fetch: Fetch;
  webhookUrl: string;
  message: string;
}): Promise<void> {
  const body = JSON.stringify({
    content: message,
    flags: SUPPRESS_EMBEDS,
    // Issue titles are user input, so an `@everyone` in one pings nobody.
    allowed_mentions: { parse: [] },
  });

  let response;
  try {
    response = await fetch(webhookUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body,
    });
  } catch {
    throw new Error('request to the Discord webhook failed');
  }

  if (!response.ok) {
    throw new Error(`Discord webhook responded with status ${response.status}`);
  }
}

// The number is the link text so that brackets in a title cannot break the
// link. The title is capped to keep six entries under Discord's 2000
// character limit.
function formatRFC(rfc: RFC): string {
  const title =
    rfc.title.length > MAX_TITLE_LENGTH
      ? `${rfc.title.slice(0, MAX_TITLE_LENGTH - 1)}…`
      : rfc.title;

  return `- [#${rfc.number}](${rfc.url}) ${title} (👍 ${rfc.thumbsUp})`;
}
