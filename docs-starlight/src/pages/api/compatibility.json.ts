import type { APIRoute } from 'astro';

const tofuCompatibility = [
	{
		"tofu": "1.9.x",
		"terragrunt": ">= 0.72.0"
	},
	{
		"tofu": "1.8.x",
		"terragrunt": ">= 0.66.0"
	},
	{
		"tofu": "1.7.x",
		"terragrunt": ">= 0.58.0"
	},
	{
		"tofu": "1.6.x",
		"terragrunt": ">= 0.52.0"
	}
]

const terraformCompatibility = [
	{
		"terraform": "1.9.x",
		"terragrunt": ">= 0.60.0"
	},
	{
		"terraform": "1.8.x",
		"terragrunt": ">= 0.57.0"
	},
	{
		"terraform": "1.7.x",
		"terragrunt": ">= 0.56.0"
	},
	{
		"terraform": "1.6.x",
		"terragrunt": ">= 0.53.0"
	},
	{
		"terraform": "1.5.x",
		"terragrunt": ">= 0.48.0"
	},
	{
		"terraform": "1.4.x",
		"terragrunt": ">= 0.45.0"
	},
	{
		"terraform": "1.3.x",
		"terragrunt": ">= 0.40.0"
	},
	{
		"terraform": "1.2.x",
		"terragrunt": ">= 0.38.0"
	},
	{
		"terraform": "1.1.x",
		"terragrunt": ">= 0.36.0"
	},
	{
		"terraform": "1.0.x",
		"terragrunt": ">= 0.31.0"
	},
	{
		"terraform": "0.15.x",
		"terragrunt": ">= 0.29.0"
	},
	{
		"terraform": "0.14.x",
		"terragrunt": ">= 0.27.0"
	},
	{
		"terraform": "0.13.x",
		"terragrunt": ">= 0.25.0"
	},
	{
		"terraform": "0.12.x",
		"terragrunt": "0.19.0 - 0.24.4"
	},
	{
		"terraform": "0.11.x",
		"terragrunt": "0.14.0 - 0.18.7"
	},
]

const compatibility = [...tofuCompatibility, ...terraformCompatibility]

export const GET: APIRoute = ({ params, request }) => {
	// Parse the request.url to get the query parameters
	const url = new URL(request.url)
	const searchParams = Object.fromEntries(url.searchParams.entries())

	// Only return the tofu compatibility table
	// if the user asks for it with the "tf" query parameter
	if (searchParams.tf && searchParams.tf === "tofu") {
		return new Response(
			JSON.stringify(tofuCompatibility)
		)
	}

	// Only return the terraform compatibility table
	// if the user asks for it with the "tf" query parameter
	if (searchParams.tf && searchParams.tf === "terraform") {
		return new Response(
			JSON.stringify(terraformCompatibility)
		)
	}

	return new Response(
		JSON.stringify(compatibility)
	)
}
