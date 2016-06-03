package locks

import (
	"time"
	"net"
	"github.com/gruntwork-io/terragrunt/errors"
	"fmt"
)

// Copied from format.go. Not sure why it's not exposed as a variable.
const DEFAULT_TIME_FORMAT = "2006-01-02 15:04:05.999999999 -0700 MST"

// This structure represents useful metadata about the lock, such as who acquired it, when, and from what IP
type LockMetadata struct {
	StateFileId string
	Username    string
	IpAddress   string
	DateCreated time.Time
}

// Create the LockMetadata for the given state file and user
func CreateLockMetadata(stateFileId string, username string) (*LockMetadata, error) {
	ipAddress, err := getIpAddress()
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	dateCreated := time.Now().UTC()

	return &LockMetadata{
		StateFileId: stateFileId,
		Username: username,
		IpAddress: ipAddress,
		DateCreated: dateCreated,
	}, nil
}

// Get the IP address for the current host. This method makes a best effort to read all available interfaces and grab
// teh first one that looks like an IPV4 address.
func getIpAddress() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return "", errors.WithStackTrace(err)
		}

		for _, addr := range addrs {
			switch ip := addr.(type) {
			case *net.IPNet:
				if !ip.IP.IsLoopback() && ip.IP.To4() != nil {
					return ip.IP.String(), nil
				}
			case *net.IPAddr:
				if !ip.IP.IsLoopback() && ip.IP.To4() != nil {
					return ip.IP.String(), nil
				}
			}
		}
	}

	return "", errors.WithStackTrace(NoIpAddressFound)
}

var NoIpAddressFound = fmt.Errorf("Could not find IP address for current machine")