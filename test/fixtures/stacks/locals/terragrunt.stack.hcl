locals {
	chicken = "units/chicken"
	chick = "units/chick"
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
	source = local.chick
	path   = "chicks/chick-1"
}

unit "chick_2" {
	source = local.chick
	path   = "chicks/chick-2"
}

