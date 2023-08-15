package aws_helper

import "encoding/json"

// Policy - representation of the policy for AWS
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

func UnmarshalPolicy(policy string) (Policy, error) {
	var p Policy
	err := json.Unmarshal([]byte(policy), &p)
	if err != nil {
		return p, err
	}

	return p, nil
}

func MarshalPolicy(policy Policy) ([]byte, error) {
	policyJson, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}

	return policyJson, nil
}
