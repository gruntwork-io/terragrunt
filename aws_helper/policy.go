package aws_helper

import "encoding/json"

// A representation of the polciy for AWS
type Policy struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}

type Statement struct {
	Sid       string                  `json:"Sid"`
	Effect    string                  `json:"Effect"`
	Principal interface{}             `json:"Principal"`
	Action    string                  `json:"Action"`
	Resource  []string                `json:"Resource"`
	Condition *map[string]interface{} `json:"Condition,omitempty"`
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
