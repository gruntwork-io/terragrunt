
stack "dev" {
	source = "${get_repo_root()}/stacks/dev"
	path = "dev"
}


stack "prod" {
	source = "${get_repo_root()}/stacks/prod"
	path = "prod"
}
