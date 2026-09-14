/**
 * Take a marketing form submission and create the HubSpot contact. The whole
 * path, in one place.
 *
 * POSTING TO HUBSPOT HERE IS WHAT MAKES THE STATUS CODE MEAN ANYTHING. The
 * browser shows its success state on a 204 and nothing else, so this function
 * sends 204 only once HubSpot has accepted the submission. An endpoint that
 * acknowledges before doing the work cannot tell a delivered lead from a lost
 * one, and a lead path that reaches nobody then looks green to the visitor and
 * to every check.
 *
 * NOTHING QUEUES OR RETRIES, so a failure is reported to the visitor — who can
 * resend, which a silently queued lead cannot — AND THESE LOGS ARE THE ONLY
 * OTHER RECORD. Every line is single-line JSON with an `evt` and the Vercel
 * request id. Two rules decide what goes in one:
 *
 *   - ON SUCCESS: masked email, form, HubSpot form id, route, duration. No
 *     field values — a log is not a second copy of the CRM.
 *   - ON FAILURE the line is the only remaining copy of the lead, so it carries
 *     the full field set.
 *
 * NO SECRET IS INVOLVED: HubSpot's Forms submission endpoint is public and
 * unauthenticated, which is how a marketing form on any static page works.
 * `/ftag` stays same-origin anyway — an ad blocker that breaks a pageview is an
 * annoyance and one that breaks LEAD CAPTURE is not.
 */

const HUBSPOT_PORTAL_ID = process.env.HUBSPOT_PORTAL_ID || "8376079";

/**
 * One HubSpot form per site form, keyed by the `data-tg-form` value the page
 * sends. Set the matching env var to point a form at a different HubSpot form
 * without a deploy of this file.
 *
 * TODO(DEV-1683): confirm these GUIDs against the Terragrunt HubSpot portal
 * before cutover. Until they are confirmed, every submission lands in the
 * default Landing Pages form, which `form.unmapped` warns about but does not
 * reject — losing a lead is worse than filing it in the wrong place.
 */
const HUBSPOT_FORMS = {
  "Terragrunt Contact Form": process.env.HUBSPOT_FORM_CONTACT || "",
  "Ambassador Contact Form": process.env.HUBSPOT_FORM_AMBASSADOR || "",
  "Newsletter Form": process.env.HUBSPOT_FORM_NEWSLETTER || "",
  "Pipelines Contact Form": process.env.HUBSPOT_FORM_LANDING || "",
  "Keychron Contact Form": process.env.HUBSPOT_FORM_LANDING || "",
  _default: process.env.HUBSPOT_FORM_DEFAULT || "",
};

/** A submission is a few hundred bytes. Room to spare, not a real limit. */
const MAX_BYTES = 128 * 1024;

/** HubSpot is on the visitor's critical path, so it does not get to hang the request. */
const HUBSPOT_TIMEOUT_MS = 10_000;

/** "ada LOVELACE" -> "Ada Lovelace". */
const titleCase = (w) => (w ? w[0].toUpperCase() + w.slice(1).toLowerCase() : "");

/**
 * Read a submitted field by any of its spellings. The forms disagree on case
 * and wording — `Email` vs `email`, `Source` vs `how-did-you-hear` — so a
 * branch reading one spelling rejects the other as having no email at all,
 * which is a 400 on a real lead.
 */
function field(data, ...names) {
  for (const n of names) if (data[n]) return data[n];
  const lower = {};
  for (const [k, v] of Object.entries(data || {})) lower[k.toLowerCase()] = v;
  for (const n of names) if (lower[n.toLowerCase()]) return lower[n.toLowerCase()];
  return "";
}

function hubspotFields(formName, data) {
  const pageUrl = field(data, "landing_page_url", "pageUri");
  const email = field(data, "Email", "email");

  if (formName === "Newsletter Form") {
    return { email, is_newsletter_subscriber: "Yes" };
  }

  // Both contact forms send the whole name in one field, which HubSpot wants split.
  const parts = String(field(data, "Name", "name") || "")
    .trim()
    .split(/\s+/)
    .filter(Boolean);

  const base = {
    firstname: titleCase(parts[0] || ""),
    lastname: parts.slice(1).map(titleCase).join(" "),
    email,
    phone: field(data, "Phone", "phone"),
    how_did_you_hear_about_us_: field(data, "Source", "how-did-you-hear"),
    message: field(data, "Message", "How-can-we-help"),
    landing_page_url: pageUrl,
  };

  if (formName === "Terragrunt Contact Form") {
    return { ...base, company: field(data, "Company") };
  }

  return base;
}

/**
 * `hutk` is the `hubspotutk` cookie the page reads, tying this submission to
 * that visitor's earlier browsing. Omitted rather than sent empty: HubSpot
 * rejects a blank one. `ipAddress` is the one field a browser cannot know, so
 * it comes from the forwarded address.
 */
function hubspotContext(payload, data, req) {
  const context = {
    pageUri: data.pageUri || payload.pageUrl || "",
    pageName: data.pageName || payload.publishedPath || "",
  };
  if (data.hutk) context.hutk = data.hutk;

  const forwarded = req.headers["x-forwarded-for"];
  const ip = Array.isArray(forwarded)
    ? forwarded[0]
    : String(forwarded || "")
        .split(",")[0]
        .trim();
  if (ip) context.ipAddress = ip;

  return context;
}

/**
 * Single-line JSON so a log drain can filter on `evt` rather than parse prose.
 * `console.error` for anything a human should act on, so Vercel's own error
 * filter surfaces it.
 */
function log(level, evt, fields) {
  const line = JSON.stringify({ t: new Date().toISOString(), evt, ...fields });
  if (level === "error") console.error(line);
  else console.log(line);
}

/**
 * `jane@corp.com` -> `j***@corp.com`. Enough to recognise a specific lead and
 * to triage a spam wave by domain, without printing an address list into the
 * logs.
 */
function maskEmail(email) {
  const at = String(email || "").indexOf("@");
  if (at < 1) return "(none)";
  return `${email[0]}***${email.slice(at)}`;
}

export default async function handler(req, res) {
  const rid = req.headers["x-vercel-id"] || null;
  const started = Date.now();
  const done = (status, evt, level, fields) => {
    log(level, evt, { rid, status, ms: Date.now() - started, ...fields });
    return status === 204
      ? res.status(204).end()
      : res.status(status).json({ error: evt });
  };

  try {
    if (req.method !== "POST") {
      res.setHeader("Allow", "POST");
      return done(405, "rejected.method", "warn", { method: req.method });
    }

    /**
     * `req.body` PARSES LAZILY AND THROWS ON ACCESS when the declared
     * content-type is JSON and the body is not, so this read is guarded
     * separately from the parse below. Malformed input is a client error;
     * reporting it as ours sends someone hunting a bug in this file.
     */
    let rawBody;
    try {
      rawBody = req.body;
    } catch {
      return done(400, "rejected.not_json", "warn", {});
    }

    const raw = typeof rawBody === "string" ? rawBody : JSON.stringify(rawBody ?? {});
    const bytes = Buffer.byteLength(raw, "utf8");
    if (bytes > MAX_BYTES) return done(413, "rejected.too_large", "warn", { bytes });

    let payload;
    try {
      payload = typeof rawBody === "string" ? JSON.parse(rawBody) : (rawBody ?? {});
    } catch {
      return done(400, "rejected.not_json", "warn", { bytes });
    }

    const formName = payload.name ?? null;
    const data = payload.data ?? {};
    const fields = hubspotFields(formName, data);
    const mapped = Object.prototype.hasOwnProperty.call(HUBSPOT_FORMS, formName);
    const guid = (mapped && HUBSPOT_FORMS[formName]) || HUBSPOT_FORMS._default;

    // Identity of the submission, on every line about it.
    const who = {
      form: formName,
      guid,
      route: payload.publishedPath ?? null,
      email: maskEmail(fields.email),
    };

    // An email is what makes a submission a lead. HubSpot would 400 without
    // one, but this endpoint is reachable by anyone and someone else's
    // validation is not our guard.
    if (!fields.email) return done(400, "rejected.no_email", "warn", { ...who, bytes });

    /*
     * With no GUID there is nowhere to send this, and answering 204 would show
     * the visitor a success state for a lead that reached nobody. Fail loudly,
     * and log the whole lead — this line is the only copy of it.
     */
    if (!guid) {
      return done(503, "hubspot.unconfigured", "error", {
        ...who,
        lead: fields,
      });
    }

    // An unknown form name still submits, to the default form, but warns: the
    // likeliest cause is a new form nobody mapped, whose leads then land in the
    // wrong HubSpot form.
    if (!mapped) log("warn", "form.unmapped", { rid, ...who });

    log("info", "received", { rid, ...who, bytes });

    const url = `https://api.hsforms.com/submissions/v3/integration/submit/${HUBSPOT_PORTAL_ID}/${guid}`;
    const body = {
      fields: Object.entries(fields)
        .filter(([, value]) => value !== "" && value != null)
        .map(([name, value]) => ({ name, value: String(value) })),
      context: hubspotContext(payload, data, req),
    };

    let upstream, text;
    try {
      upstream = await fetch(url, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(body),
        signal: AbortSignal.timeout(HUBSPOT_TIMEOUT_MS),
      });
      text = await upstream.text();
    } catch (err) {
      // Unreachable or timed out. Nothing holds this lead but this line.
      return done(502, "hubspot.unreachable", "error", {
        ...who,
        reason:
          err?.name === "TimeoutError"
            ? `timeout after ${HUBSPOT_TIMEOUT_MS}ms`
            : String(err),
        lead: body,
      });
    }

    if (!upstream.ok) {
      // HubSpot's status is logged rather than returned: a 400 surfacing in the
      // browser would read as the VISITOR having done something wrong, which is
      // the wrong story entirely.
      return done(502, "hubspot.rejected", "error", {
        ...who,
        hubspotStatus: upstream.status,
        hubspotBody: text?.slice(0, 2000) ?? null,
        lead: body,
      });
    }

    return done(204, "submitted", "info", { ...who, hubspotStatus: upstream.status });
  } catch (err) {
    // A crash here loses a lead silently, the one outcome this file exists to prevent.
    log("error", "unhandled", {
      rid,
      ms: Date.now() - started,
      error: String(err),
      stack: err?.stack,
    });
    return res.status(500).json({ error: "Internal error" });
  }
}
