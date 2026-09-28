export type CaptureFn = (event: string, properties: Record<string, unknown>) => void;

export const SEARCH_EVENTS = {
  opened: "docs_search_opened",
  query: "docs_search_query",
  resultClicked: "docs_search_result_clicked",
  abandoned: "docs_search_abandoned",
} as const;

export const KAPA_EVENT_NAMES = {
  onModalOpen: "kapa_modal_opened",
  onModalClose: "kapa_modal_closed",
  onModeSwitch: "kapa_mode_switched",
  onAskAIQuerySubmit: "kapa_question_asked",
  onAskAIExampleQuerySubmit: "kapa_example_question_asked",
  onAskAIAnswerCompleted: "kapa_answer_completed",
  onAskAIFeedbackSubmit: "kapa_answer_feedback_submitted",
  onAskAIAnswerCopy: "kapa_answer_copied",
  onAskAIGenerationStop: "kapa_generation_stopped",
  onAskAIConversationReset: "kapa_conversation_reset",
  onAskAILinkClick: "kapa_link_clicked",
  onAskAISourceClick: "kapa_source_clicked",
  onAskAIHandoffOpen: "kapa_handoff_opened",
  onAskAIHandoffSubmit: "kapa_handoff_submitted",
  onAskAIHandoffCancel: "kapa_handoff_cancelled",
  onSearchResultsCompleted: "kapa_search_results_returned",
  onSearchResultClick: "kapa_search_result_clicked",
} as const;

export interface SearchResultClick {
  url: string;
  position: number;
  isSubResult: boolean;
}

export interface SearchTracker {
  open(trigger: string): void;
  recordQuery(rawQuery: string, resultCount: number): void;
  recordResultClick(click: SearchResultClick): void;
  close(): void;
}

/**
 * Tracks one reader's journey through the search modal and reports it through `capture`.
 *
 * The caller drives it from DOM events; this holds the state needed to attribute a result click to
 * the query that produced it, and to tell an abandoned search from a successful one.
 */
export function createSearchTracker(capture: CaptureFn): SearchTracker {
  let isOpen = false;
  let query = "";
  let reportedQuery = "";
  let resultCount = 0;
  let clicked = false;

  return {
    open(trigger) {
      if (isOpen) return;

      isOpen = true;
      query = "";
      reportedQuery = "";
      resultCount = 0;
      clicked = false;

      capture(SEARCH_EVENTS.opened, { trigger });
    },

    recordQuery(rawQuery, count) {
      const normalized = normalizeQuery(rawQuery);
      if (!normalized) return;

      if (normalized === query) return;

      query = normalized;
      reportedQuery = redact(normalized);
      resultCount = count;

      capture(SEARCH_EVENTS.query, {
        query: reportedQuery,
        query_length: normalized.length,
        result_count: count,
        has_results: count > 0,
      });
    },

    recordResultClick(click) {
      clicked = true;

      capture(SEARCH_EVENTS.resultClicked, {
        query: reportedQuery,
        result_count: resultCount,
        url: click.url,
        position: click.position,
        is_sub_result: click.isSubResult,
      });
    },

    close() {
      isOpen = false;
      if (!query || clicked) return;

      capture(SEARCH_EVENTS.abandoned, {
        query: reportedQuery,
        query_length: query.length,
        result_count: resultCount,
      });
    },
  };
}

export function normalizeQuery(rawQuery: string): string {
  return rawQuery.trim().replace(/\s+/g, " ");
}

const REDACTED = "[redacted]";

/**
 * Patterns are ordered from the most specific to the most general, so that a credential is described
 * by the narrowest rule that recognizes it.
 */
const REDACTIONS: ReadonlyArray<readonly [RegExp, string]> = [
  [/[^\s@]+@[^\s@]+\.[^\s@]+/g, "[email]"],
  [/-----BEGIN[^-]*PRIVATE KEY-----/gi, REDACTED],
  [/\b(?:AKIA|ASIA|ABIA|ACCA|AGPA|AIDA|AIPA|ANPA|ANVA|AROA)[A-Z0-9]{16}\b/g, REDACTED],
  [/\bgh[pousr]_[A-Za-z0-9]{20,}/g, REDACTED],
  [/\bxox[baprs]-[A-Za-z0-9-]{10,}/g, REDACTED],
  [/\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}[A-Za-z0-9._-]*/g, REDACTED],
  [
    /\b(password|passwd|secret|token|api[_-]?key|access[_-]?key|credentials?)(\s*[:=]\s*)\S+/gi,
    `$1$2${REDACTED}`,
  ],
  [
    /\b(?=[A-Za-z0-9+/=_-]*[a-z])(?=[A-Za-z0-9+/=_-]*[A-Z])(?=[A-Za-z0-9+/=_-]*\d)[A-Za-z0-9+/=_-]{32,}/g,
    REDACTED,
  ],
];

/**
 * Removes credentials and addresses from text a reader typed. Search boxes attract pasted material,
 * and a query is worth reporting for the words in it rather than for whatever came along with them.
 */
export function redact(text: string): string {
  return REDACTIONS.reduce((redacted, [pattern, replacement]) => {
    return redacted.replace(pattern, replacement);
  }, text);
}

const SEARCH_TERM_PLACEHOLDER = "[SEARCH_TERM]";

// Pagefind's own wording, which is what it renders unless the site overrides it. Starlight passes
// through only the strings a site has customized, so this default is what readers here see.
const DEFAULT_SEARCHING_MESSAGE = "Searching for [SEARCH_TERM]...";

/**
 * Pulls the fixed part of Pagefind's "still searching" message out of the translations Starlight
 * serializes onto the search element, so that a translated site is read in its own language.
 */
export function searchingPrefix(translations: string | null): string {
  const searching = parseTranslations(translations).searching;
  const template = typeof searching === "string" ? searching : DEFAULT_SEARCHING_MESSAGE;

  return template.split(SEARCH_TERM_PLACEHOLDER)[0] ?? "";
}

/**
 * Decides whether the results on screen belong to `query` and are final. Pagefind names the search
 * term both while it is loading its index and once it has answered, so recognizing the message it
 * shows in the meantime is what keeps a search that has not happened yet from being reported as a
 * search that found nothing.
 */
export function hasSettled(message: string | null, query: string, loadingPrefix: string): boolean {
  if (!message) return false;
  if (!message.toLowerCase().includes(query.toLowerCase())) return false;
  if (/^\s*\d/.test(message)) return true;

  return loadingPrefix.length === 0 || !message.startsWith(loadingPrefix);
}

function parseTranslations(translations: string | null): Record<string, unknown> {
  if (!translations) return {};

  try {
    const parsed: unknown = JSON.parse(translations);
    if (typeof parsed !== "object" || parsed === null) return {};

    return parsed as Record<string, unknown>;
  } catch {
    return {};
  }
}

/**
 * Reads the result count from Pagefind's status message, falling back to the number of results on
 * screen. Pagefind paginates, so the rendered count undercounts whenever the message is missing or
 * reports an alternative spelling instead of a total.
 */
export function parseResultCount(message: string | null, renderedCount: number): number {
  const total = message?.match(/^\s*(\d+)/);
  if (total) return Number(total[1]);

  return renderedCount;
}

const KAPA_BULKY_FIELDS = new Set(["answer", "conversation", "history", "sources"]);

const MAX_PROPERTY_LENGTH = 1000;

/**
 * Flattens a kapa.ai widget event payload into PostHog properties, keeping the scalars and dropping
 * generated prose.
 */
export function kapaEventProperties(payload: unknown): Record<string, unknown> {
  const properties: Record<string, unknown> = {};
  if (typeof payload !== "object" || payload === null) return properties;

  for (const [key, value] of Object.entries(payload)) {
    if (key === "answer" && typeof value === "string") {
      properties.answer_length = value.length;
      continue;
    }

    if (KAPA_BULKY_FIELDS.has(key)) continue;

    if (typeof value === "string") {
      properties[snakeCase(key)] = redact(value).slice(0, MAX_PROPERTY_LENGTH);
      continue;
    }

    if (typeof value === "number" || typeof value === "boolean") {
      properties[snakeCase(key)] = value;
    }
  }

  return properties;
}

function snakeCase(key: string): string {
  return key.replace(/([a-z0-9])([A-Z])/g, "$1_$2").toLowerCase();
}
