unit "project-1-app1" {
	source = "../units/app1"
	path   = "project-1-app1"
}

unit "project-1-app2" {
	source = "../units/app2"
	path   = "project-1-app2"
}

stack "stack-to-include" {
	source = "../stack-to-include"
	path = "stack-to-include"
}
