unit "app_1" {
	source = "${get_repo_root()}/units/app"
	path   = "app_1"
	values = {
		data = "app_1"
	}
}

unit "app_2" {
	source = "${get_repo_root()}/units/app"
	path   = "app_2"
	values = {
		data = "app_2"
	}
}

stack "root_stack_1" {
	source = "${get_repo_root()}/stacks/v1"
	path   = "stack_1"
}

stack "root_stack_2" {
	source = "${get_repo_root()}/stacks/v2"
	path   = "stack_2"
}

stack "root_stack_3" {
	source = "${get_repo_root()}/stacks/v3"
	path   = "stack_3"
}

