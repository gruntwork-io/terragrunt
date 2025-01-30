export default function Command({ description, children }) {
	return (
		<div>
			<p>{description}</p>
			{children}
		</div>
	);
}
