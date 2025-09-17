import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

// Shadcn has a convention of using the cn function to merge classes
// https://github.com/shadcn-ui/ui/blob/main/apps/www/lib/utils.ts#L5
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
