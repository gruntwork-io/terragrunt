interface PostHogClient {
  capture(event: string, properties: Record<string, unknown>): void;
}

interface PendingEvent {
  event: string;
  properties: Record<string, unknown>;
}

const PRODUCTION_HOSTNAME = "docs.terragrunt.com";
const LOCAL_HOSTNAMES = new Set(["localhost", "127.0.0.1"]);

const PENDING_LIMIT = 20;
const CLIENT_POLL_MS = 500;
const CLIENT_TIMEOUT_MS = 30_000;

const pending: PendingEvent[] = [];

let pollingForClient = 0;

/**
 * Reports an event to PostHog.
 */
export function capture(event: string, properties: Record<string, unknown> = {}): void {
  if (import.meta.env.DEV) {
    console.debug(`[analytics] ${event}`, properties);
    return;
  }

  const reported = { ...properties, docs_environment: environment() };

  const posthog = client();
  if (posthog) {
    posthog.capture(event, reported);
    return;
  }

  if (pending.length < PENDING_LIMIT) pending.push({ event, properties: reported });
  waitForClient();
}

function client(): PostHogClient | undefined {
  return (window as unknown as { posthog?: PostHogClient }).posthog;
}

function waitForClient(): void {
  if (pollingForClient) return;

  const deadline = Date.now() + CLIENT_TIMEOUT_MS;

  pollingForClient = window.setInterval(() => {
    const posthog = client();
    if (!posthog && Date.now() < deadline) return;

    window.clearInterval(pollingForClient);
    pollingForClient = 0;

    const held = pending.splice(0);
    if (!posthog) return;

    for (const { event, properties } of held) posthog.capture(event, properties);
  }, CLIENT_POLL_MS);
}

function environment(): string {
  if (location.hostname === PRODUCTION_HOSTNAME) return "production";
  if (LOCAL_HOSTNAMES.has(location.hostname)) return "local";

  return "preview";
}
