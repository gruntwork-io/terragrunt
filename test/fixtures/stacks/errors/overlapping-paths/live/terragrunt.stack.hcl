
stack "parent" {
	source = "../units/app"
	path   = "shared"
}

stack "nested" {
	source = "../units/app"
	path   = "shared/inner"
}
