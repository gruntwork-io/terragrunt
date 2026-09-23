/**
 * Serve the GTM container with its tag-serving host pointed back at us —
 * first-party mode, done here instead of in Google's console.
 *
 * THE PROBLEM THIS SOLVES. `/mtag/:path*` proxies anything the browser ASKS us
 * for, and that half works: `/mtag/gtag/js?id=G-…` returns Google's GA4 library
 * byte-identical. But nothing asks. The container builds that URL itself, as
 * `"https://" + E(3) + "/gtag/js"`, where `E(3)` is a config value Google bakes
 * in and ships as `"3":"www.googletagmanager.com"`. So the page loads the
 * container first-party and then fetches the library straight from Google,
 * where a blocker stops it.
 *
 * WHAT IT DOES. Fetch the real container, rewrite that one config value to
 * `<our host>/mtag`, and serve it. The container then builds
 * `https://<our host>/mtag/gtag/js?id=…`, which the rewrite in `vercel.json`
 * forwards to Google. Same mechanism Google's own first-party mode uses.
 *
 * THE HOST COMES FROM THE REQUEST, not a constant, because the value is
 * absolute and this deploys to more than one hostname: a preview URL per pull
 * request as well as docs.terragrunt.com. Hard-coding it would silently send
 * preview traffic to production's path.
 *
 * IT FAILS OPEN. `"3"` is a numbered slot in a minified container Google can
 * renumber without telling anyone. If the marker is not found exactly once,
 * this serves the container UNMODIFIED rather than a guess: the tag then loads
 * the library from Google, which is merely the behaviour without this function,
 * where serving a mangled container would break analytics outright.
 * `x-mtag-firstparty` says which happened, so a check can assert it rather than
 * infer it.
 */

/** The config slot GTM reads as its tag-serving host — `E(3)` in the container. */
const HOST_SLOT = '"3":"www.googletagmanager.com"';

const UPSTREAM = "https://www.googletagmanager.com/gtm.js";

export default async function handler(req, res) {
  const url = new URL(req.url, "https://placeholder.invalid");

  let response;
  try {
    response = await fetch(`${UPSTREAM}${url.search}`, {
      headers: {
        /** Google varies the container on these; passing them through keeps the response correct. */
        "user-agent": req.headers["user-agent"] ?? "",
        "accept-language": req.headers["accept-language"] ?? "",
      },
    });
  } catch (error) {
    console.log(
      JSON.stringify({
        evt: "mtag.upstream_error",
        message: String(error).slice(0, 200),
      }),
    );
    res.status(502).send("// container unavailable");
    return;
  }

  if (!response.ok) {
    console.log(
      JSON.stringify({ evt: "mtag.upstream_status", status: response.status }),
    );
    res.status(response.status).send("// container unavailable");
    return;
  }

  const body = await response.text();

  /**
   * The host the browser asked us for. `x-forwarded-host` is what Vercel sets
   * in front of the function; `host` is the fallback for a direct invocation.
   */
  const self = String(req.headers["x-forwarded-host"] ?? req.headers.host ?? "")
    .split(",")[0]
    .trim();

  const occurrences = body.split(HOST_SLOT).length - 1;
  const rewritten = occurrences === 1 && Boolean(self);
  const out = rewritten ? body.replace(HOST_SLOT, `"3":"${self}/mtag"`) : body;

  if (!rewritten) {
    console.log(
      JSON.stringify({
        evt: "mtag.marker_missing",
        occurrences,
        host: self,
        bytes: body.length,
      }),
    );
  }

  res.setHeader("content-type", "application/javascript; charset=UTF-8");
  /**
   * Cached at the edge so a page view does not cost an origin fetch of the
   * whole container, and revalidated often because the container is how a
   * marketer's change reaches the site.
   */
  res.setHeader(
    "cache-control",
    "public, max-age=0, s-maxage=300, stale-while-revalidate=600",
  );
  res.setHeader("x-mtag-firstparty", rewritten ? "1" : "0");
  res.status(200).send(out);
}
