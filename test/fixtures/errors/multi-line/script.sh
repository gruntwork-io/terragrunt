#!/bin/bash

echo "Error: creating Route in Route Table (rtb-46521694) with destination (10.0.0.0/8): operation error EC2: CreateRoute, https response error StatusCode: 400, RequestID: JD40-14127-2022, api error InvalidTransitGatewayID.NotFound: The transitGateway ID 'tgw-xxxxxxxxxxxxxx' does not exist."
exit 1
