/// <reference types="astro/client" />
/// <reference types="bun" />

// `src/components/SiteTitle.astro` overrides Starlight's component and (like Starlight's own)
// imports the configured logos from `virtual:starlight/user-images`. That module's types live in
// Starlight's internal `virtual-internal.d.ts`, which isn't exposed to consumer projects, so
// `astro check` can't resolve the import (it resolves fine at build time via Starlight's Vite
// plugin). Redeclare it here to close the type gap.
declare module 'virtual:starlight/user-images' {
	type ImageMetadata = import('astro').ImageMetadata;
	export const logos: {
		dark?: ImageMetadata;
		light?: ImageMetadata;
	};
}
