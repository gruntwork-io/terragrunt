import { React, useState, useEffect } from 'react';
import InstallMd from '../md/install.mdx';

export default function InstallWrapper() {
	const [version, setVersion] = useState('v0.72.5');

	useEffect(() => {
		fetch('https://api.github.com/repos/gruntwork-io/terragrunt/releases/latest')
			.then((response) => response.json())
			.then((data) => {
				if (data.tag_name) {
					setVersion(data.tag_name);
				}
			})
			.catch((error) => {
				console.error('Error:', error);
			});
	}, []);

	return (
		<InstallMd version={version} />
	);
}
