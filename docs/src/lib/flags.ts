import { getLatestRelease } from "@lib/github";
import { isReleased } from "@lib/changelog";

export async function isFlagVisible(since: string | undefined): Promise<boolean> {
  if (import.meta.env.DEV) return true;
  if (!since) return true;
  const release = await getLatestRelease("gruntwork-io", "terragrunt");
  const latestVersion = (release?.tag_name ?? "v0.0.0").replace(/^v/, "");
  return isReleased(since, latestVersion);
}
