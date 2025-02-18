import type { CollectionEntry } from "astro:content";
import type { SidebarItem } from "node_modules/@astrojs/starlight/schemas/sidebar";

export function getSidebar(commands: CollectionEntry<'commands'>[]): SidebarItem[] {
	const sidebar: SidebarItem[] = commands.map((command) => {
		const data = command.data;

		const sideBarItem = {
			label: data.name,
			link: `/docs/reference/cli/commands/${data.path}`,
		} as SidebarItem;

		if (data.experiment) {
			sideBarItem.badge = {
				variant: 'tip',
				text: 'exp',
			};
		}

		return sideBarItem;
	});

	return sidebar;
}

