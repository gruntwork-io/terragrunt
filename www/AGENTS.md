# terragrunt.com

The marketing site, ported off Webflow. Astro, static output, deployed to Vercel.
`docs/` in this repo is a separate app serving `docs.terragrunt.com`; the two
share a palette and a wordmark and nothing else.

## Development

```
bun install
bun run dev        # http://localhost:4321
bun run gates      # what CI runs
```

`bun run gates` is the verification entry point: `vercel:check` reads the config,
then a build, then `routes:check` and `analytics:check` read `dist/`.

## Content lives in three places, by kind

**Collections** — `src/content/{faq,comparisons,guides}/<slug>.md`. One file per
page, frontmatter validated by `src/content.config.ts`. **The filename IS the
route**, and there is deliberately no `slug` field: Astro's glob loader would
take one as the entry id, which makes it load-bearing, and a second copy of the
id is a second thing that can drift from a URL a crawler already knows.

**Page copy** — `src/data/*.ts`, for pages whose layout is bespoke (homepage,
Terragrunt Scale, ambassadors). Prices, plan names and feature lists are data
rather than markup so that the parts most likely to change are findable.

**Everything else** is the page itself, under `src/pages/`.

## Things that fail silently

**`/mtag/gtm.js` must be listed BEFORE `/mtag/:path*` in `vercel.json`.** Vercel
takes the first matching rewrite, and the catch-all matches `gtm.js`. Listed the
wrong way round, the container is served as a plain proxy with
`www.googletagmanager.com` still baked into it, the GA4 library is then fetched
straight from Google where a blocker stops it, and nothing about the page looks
different. `vercel:check` asserts the order.

**Do not move the Google tag into a web worker.** Partytown was tried on
gruntwork.io and removed: a worker has no `localStorage`, so it proxies every
synchronous browser API call through a service worker, one unbatched HTTP round
trip each. A HAR of one page load showed 9,140 of 9,159 requests were Partytown
proxy calls. `docs/` in this repo still runs it.

**`/ftag` answers 204 only when HubSpot accepted the submission**, and the form
script treats every other status as a failure. That narrowness is the point:
when a check's only signal is a status code the far end controls, it is not a
check, and an endpoint that acknowledges before doing the work makes a lead path
that reaches nobody look green to the visitor and to every gate.

**`api/ftag.js` logs are the only other record of a lead.** Nothing queues or
retries. On success the line carries a masked email and no field values — a log
is not a second copy of the CRM. On failure it carries the full field set,
because that line is all that is left.

**`trailingSlash` is `never` in Astro and `false` in `vercel.json`.** Webflow
301'd the slashed form of every URL. Flipping it turns every inbound link into a
redirect hop and splits link equity across two spellings.

**`cleanUrls` is explicitly `false`, not omitted.** Enabling it injects a 308
matching `^/(.*)\.html/?$` ahead of the redirects array, shadowing rules below.

**`vite.build.cssTarget` is pinned.** Left to its default the minifier rewrites
`(max-width: 991px)` into range syntax needing Chrome 104 / Firefox 102 / Safari
16.4, below which the query matches nothing and a phone gets the desktop layout.
Valid CSS, clean build, silent failure.

## Matching the live site

The pages here are rebuilt, not ported, so they are not pixel-identical — but
every difference should be a consequence of that decision, not an invention.
Values below were read off the live DOM with `getComputedStyle`, never guessed.

**Two heading scales exist.** `.heading` is 42px/46.67 at weight 500 (homepage,
Terragrunt Scale); `.headline` is 42px/1.5em (ambassador, checkout). Long-form
`h2` is 30px/37.5. `SectionHeading` takes `leading` to pick between them.

**Three prose scales exist.** Long-form bodies (FAQ, comparisons, guides) are
20px/34 over an 800px measure — including the lede, which is NOT larger.
`/ai-info-page` is Webflow's default 14px/20px under 42px headings, which is
what `.prose-compact` is for. Putting it on the 20px scale ran that page 78%
long.

**Section rhythm is 76px top and bottom** (`.section`), the CTA band has 88px
margins and 115px of bottom padding, and the feature grids are 850px wide.

**The closing bands differ per route** and are explicit props on `PageLayout`:
`/` has all three; `/faq/*` CTA + newsletter + open-source; `/comparisons/*`
drops the CTA; `/guides/*` keeps only the newsletter; `/ai-info-page` only
open-source; `/faqs-overview`, both index pages and `/contact-tgs` end at the
footer. Assuming one tail added ~1,000px to pages that have none.

**Only `/faq/*` carries the hero mascot**, and only `/faqs-overview` and
`/contact-tgs` omit the dashed column overlay.

**`/contact-tgs` and `/terragrunt-ambassador/contact` are dark pages**
(`body.dark`), as is the Join the Program band and the whole Scale pricing
block. Building a dark band light is the single largest diff a page can carry.

**A `hidden` utility passed into a component as `class` cannot beat that
component's own `inline-flex`.** Tailwind orders the two by its output, not by
attribute order, so the mobile nav's CTAs rendered on top of the search box.
Toggle display on a wrapper instead.

**Astro collapses whitespace between a closing tag and the next expression**, so
`<span>{a}</span>{b}` renders as "ab". Split headings need an explicit `{" "}`.

**Check for horizontal overflow after any nav or hero change.** At 390px the
document must be 390px wide. Three of the live pages overflow their own
viewport on mobile; ours should not copy that.

## Gates

A gate earns its place twice: it has to guard a failure something can still
cause, and one a careful reader of the diff could not have caught.

| Gate | Catches |
| --- | --- |
| `vercel:check` | rewrite order that silently kills GA4; a redirect shadowed by a page; trailing-slash or `cleanUrls` drift |
| `routes:check` | a published URL no longer served, walking `dist/` rather than a manifest |
| `analytics:check` | a page with no Google tag, or one loading it straight from Google |

Each refuses to pass when `dist/` is missing rather than reasoning from an empty
set. All three were verified to fail on a deliberately introduced fault.

`manifest/routes.json` is every URL in Webflow's sitemap at the time of the port.
When a page is deliberately retired, remove it there in the same commit.

## Before cutover

- **Set the HubSpot form GUIDs.** `api/ftag.js` reads them from env
  (`HUBSPOT_FORM_CONTACT`, `_AMBASSADOR`, `_NEWSLETTER`, `_LANDING`,
  `_DEFAULT`). With none set the function answers 503 and logs the whole lead
  rather than showing a visitor a success state for a submission that reached
  nobody.
- **Check the Vimeo embed's domain allowlist** covers the Vercel preview host
  and `terragrunt.com`; the player is restricted by referrer.
- **Point the GTM container's vendor tags at the same-origin paths**
  (`/vtag`, `/htag`, `/stag`, `/btag`, `/ctag`) if those vendors should also be
  first-party. The rewrites exist and are inert until the container uses them —
  that part is edited in GTM, not here.
