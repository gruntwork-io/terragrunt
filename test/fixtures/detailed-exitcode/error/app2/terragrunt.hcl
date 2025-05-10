/*
  Sequential dependency on app1 to ensure proper test execution.
  This prevents flaky tests since:
  - Local provider returns exit code 2 for managed resources
  - Local provider returns exit code 1 for data sources
  - Only the last exit code in the sequence can be checked
*/

dependencies {
  paths = ["../app1"]
}
