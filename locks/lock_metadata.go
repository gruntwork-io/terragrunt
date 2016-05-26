package locks

import (
	"time"
	"fmt"
	"net"
	"os/user"
)

// Copied from format.go. Not sure why it's not exposed as a variable.
const DEFAULT_TIME_FORMAT = "2006-01-02 15:04:05.999999999 -0700 MST"

type LockMetadata struct {
	Username    string
	IpAddress   string
	DateCreated time.Time
}

func CreateLockMetadata() (*LockMetadata, error) {
	user, err := user.Current()
	if err != nil {
		return nil, err
	}

	ipAddress, err := getIpAddress()
	if err != nil {
		return nil, err
	}

	dateCreated := time.Now().UTC()

	return &LockMetadata{Username: user.Username, IpAddress: ipAddress, DateCreated: dateCreated}, nil
}

func getIpAddress() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
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

	return "", fmt.Errorf("Could not find IP address for current machine")
}
