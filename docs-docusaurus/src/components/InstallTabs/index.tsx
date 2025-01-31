import { React } from 'react';
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';
import Install from '../Install';

export default function InstallTabs({ version }) {
	return (
		<Tabs groupId="operating-systems">
			<TabItem value="linux-amd64" label="Linux (x86)">
				<Install os="linux" arch="amd64" version={version} />
			</TabItem>
			<TabItem value="macos-arm64" label="macOS (ARM)">
				<Install os="darwin" arch="arm64" version={version} />
			</TabItem>
			<TabItem value="windows" label="Windows">
				<Install os="windows" arch="amd64" version={version} />
			</TabItem>
			<TabItem value="linux-arm64" label="Linux (ARM)">
				<Install os="linux" arch="arm64" version={version} />
			</TabItem>
			<TabItem value="macos-amd64" label="macOS (x86)">
				<Install os="darwin" arch="amd64" version={version} />
			</TabItem>
		</Tabs>
	);
}

