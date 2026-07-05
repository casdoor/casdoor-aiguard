// Copyright 2025 The Casdoor Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux

package proxy

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// getOriginalDestination reads SO_ORIGINAL_DST out of an iptables REDIRECT'd
// connection, giving us the host:port the agent actually intended to reach
// so the request can be forwarded there after inspection.
func getOriginalDestination(conn *net.TCPConn) (string, error) {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return "", err
	}

	var addr string
	var sockErr error
	err = rawConn.Control(func(fd uintptr) {
		addr4, err4 := unix.GetsockoptIPv6Mreq(int(fd), unix.IPPROTO_IP, unix.SO_ORIGINAL_DST)
		if err4 == nil {
			ip := net.IPv4(addr4.Multiaddr[4], addr4.Multiaddr[5], addr4.Multiaddr[6], addr4.Multiaddr[7])
			port := int(addr4.Multiaddr[2])<<8 | int(addr4.Multiaddr[3])
			addr = fmt.Sprintf("%s:%d", ip.String(), port)
			return
		}
		sockErr = err4
	})
	if err != nil {
		return "", err
	}
	if sockErr != nil {
		return "", sockErr
	}
	return addr, nil
}
