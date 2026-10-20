import { afterEach, beforeEach, describe, expect, mock, test } from "bun:test";
import {
  clearDiscordCache,
  DISCORD_INVITE_CODE,
  formatMemberCount,
  getDiscordMemberCount,
} from "./discord";

const originalFetch = globalThis.fetch;
const originalError = console.error;

function mockFetch(response: Response) {
  const fn = mock(async () => response);
  globalThis.fetch = fn as unknown as typeof fetch;
  return fn;
}

beforeEach(() => {
  clearDiscordCache();
  console.error = () => {};
});

afterEach(() => {
  globalThis.fetch = originalFetch;
  console.error = originalError;
});

describe("formatMemberCount", () => {
  test("shows counts below 1000 as the plain number", () => {
    expect(formatMemberCount(0)).toBe("0");
    expect(formatMemberCount(845)).toBe("845");
    expect(formatMemberCount(999)).toBe("999");
  });

  test("adds a thousands separator from 1000 up, like terragrunt.com", () => {
    expect(formatMemberCount(1000)).toBe("1,000");
    expect(formatMemberCount(1204)).toBe("1,204");
    expect(formatMemberCount(43210)).toBe("43,210");
  });
});

describe("getDiscordMemberCount", () => {
  test("returns approximate_member_count from the invite lookup", async () => {
    const fetchMock = mockFetch(
      Response.json({ code: DISCORD_INVITE_CODE, approximate_member_count: 845 })
    );

    expect(await getDiscordMemberCount(DISCORD_INVITE_CODE)).toBe(845);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe(
      `https://discord.com/api/v10/invites/${DISCORD_INVITE_CODE}?with_counts=true`
    );
  });

  test("caches a successful count", async () => {
    const fetchMock = mockFetch(Response.json({ approximate_member_count: 845 }));

    await getDiscordMemberCount(DISCORD_INVITE_CODE);
    expect(await getDiscordMemberCount(DISCORD_INVITE_CODE)).toBe(845);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  test("returns null on a 404", async () => {
    mockFetch(
      new Response(JSON.stringify({ message: "Unknown Invite", code: 10006 }), {
        status: 404,
      })
    );

    expect(await getDiscordMemberCount("not-a-real-invite")).toBeNull();
  });

  test("returns null when the response has no count", async () => {
    mockFetch(Response.json({ code: DISCORD_INVITE_CODE }));

    expect(await getDiscordMemberCount(DISCORD_INVITE_CODE)).toBeNull();
  });

  test("returns null when fetch throws", async () => {
    globalThis.fetch = mock(async () => {
      throw new Error("network down");
    }) as unknown as typeof fetch;

    expect(await getDiscordMemberCount(DISCORD_INVITE_CODE)).toBeNull();
  });

  test("does not cache failures", async () => {
    mockFetch(new Response("", { status: 500 }));
    expect(await getDiscordMemberCount(DISCORD_INVITE_CODE)).toBeNull();

    mockFetch(Response.json({ approximate_member_count: 845 }));
    expect(await getDiscordMemberCount(DISCORD_INVITE_CODE)).toBe(845);
  });
});
