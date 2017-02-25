package configstack

import (
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

// Check for dependency cycles in the given list of modules and return an error if one is found
func CheckForCycles(modules []*TerraformModule) error {
	visitedPaths := []string{}
	currentTraversalPaths := []string{}

	for _, module := range modules {
		err := checkForCyclesUsingDepthFirstSearch(module, &visitedPaths, &currentTraversalPaths)
		if err != nil {
			return err
		}
	}

	return nil
}

// Check for cycles using a depth-first-search as described here:
// https://en.wikipedia.org/wiki/Topological_sorting#Depth-first_search
//
// Note that this method uses two lists, visitedPaths, and currentTraversalPaths, to track what nodes have already been
// seen. We need to use lists to maintain ordering so we can show the proper order of paths in a cycle. Of course, a
// list doesn't perform well with repeated contains() and remove() checks, so ideally we'd use an ordered Map (e.g.
// Java's LinkedHashMap), but since Go doesn't have such a data structure built-in, and our lists are going to be very
// small (at most, a few dozen paths), there is no point in worrying about performance.
func checkForCyclesUsingDepthFirstSearch(module *TerraformModule, visitedPaths *[]string, currentTraversalPaths *[]string) error {
	if util.ListContainsElement(*visitedPaths, module.Path) {
		return nil
	}

	if util.ListContainsElement(*currentTraversalPaths, module.Path) {
		return errors.WithStackTrace(DependencyCycle(append(*currentTraversalPaths, module.Path)))
	}

	*currentTraversalPaths = append(*currentTraversalPaths, module.Path)
	for _, dependency := range module.Dependencies {
		if err := checkForCyclesUsingDepthFirstSearch(dependency, visitedPaths, currentTraversalPaths); err != nil {
			return err
		}
	}

	*visitedPaths = append(*visitedPaths, module.Path)
	*currentTraversalPaths = util.RemoveElementFromList(*currentTraversalPaths, module.Path)

	return nil
}
