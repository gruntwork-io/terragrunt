# This fixture tests the validation of unit paths in stack configurations.
# It includes units with empty paths (app1, app2) and a valid path (app3)
# to verify the validation logic.

unit "app1_empty_path" {
	source = "../units/app"
	path   = ""
}

unit "app2_empty_path" {
	source = "../units/app"
	path   = ""
}

unit "app3_not_empty_path" {
	source = "../units/app"
	path   = "app3"
}


