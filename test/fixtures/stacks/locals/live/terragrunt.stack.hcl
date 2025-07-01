locals {
	chicken = "../units/chicken"
	chick = "units/chick"
	repo_path = "${get_repo_root()}"
}

unit "mother" {
	source = local.chicken
	path   = "mother"
}

unit "father" {
	source = local.chicken
	path   = "father"
}

unit "chick_1" {
	source = "../${local.chick}"
	path   = "chicks/chick-1"
}

unit "chick_2" {
	source = "${local.repo_path}/fixtures/stacks/locals/${local.chick}"
	path   = "chicks/chick-2"
}

