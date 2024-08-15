package awshelper

import (
	"encoding/json"
	"fmt"
)

// Policy - representation of the policy for AWS.
type Policy struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}

// Statement - AWS policy statement
// Action and Resource - can be string OR array of strings
// https://docs.aws.amazon.com/IAM//latest/UserGuide/reference_policies_elements_action.html
// https://docs.aws.amazon.com/IAM//latest/UserGuide/reference_policies_elements_resource.html
type Statement struct {
	Sid          string                  `json:"Sid"`
	Effect       string                  `json:"Effect"`
	Principal    interface{}             `json:"Principal,omitempty"`
	NotPrincipal interface{}             `json:"NotPrincipal,omitempty"`
	Action       interface{}             `json:"Action"`
	Resource     interface{}             `json:"Resource"`
	Condition    *map[string]interface{} `json:"Condition,omitempty"`
}

// UnmarshalPolicy - unmarshal policy from string.
func UnmarshalPolicy(policy string) (Policy, error) {
	var p Policy

	err := json.Unmarshal([]byte(policy), &p)
	if err != nil {
		return p, fmt.Errorf("error unmarshalling policy: %w", err)
	}

	return p, nil
}

// MarshalPolicy - marshal policy to string.
func MarshalPolicy(policy Policy) ([]byte, error) {
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return nil, fmt.Errorf("error marshalling policy: %w", err)
	}

	return policyJSON, nil
}
