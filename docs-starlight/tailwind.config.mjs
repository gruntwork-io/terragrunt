/* This is the global Tailwind config for both Starlight standard pages, custom pages, and components.

   The file contents are notably minimal because we're currently defining most of our styles in global.css. At some point, it would be good
   to migrate the many styles defined there into a proper Tailwind theming defined in this config.
*/

/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./src/**/*.{astro,html,js,jsx,md,mdx,svelte,ts,tsx,vue}",
  ],
  theme: {
    extend: {},
  },
  plugins: [],
}
