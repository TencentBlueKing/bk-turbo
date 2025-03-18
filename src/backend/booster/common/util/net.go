/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package util

import (
	"net"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/static"
)

var (
	_, classA, _  = wrapParse(static.InnerIPClassA, static.InnerIPClassDefault)
	_, classA1, _ = wrapParse(static.InnerIPClassA1, static.InnerIPClassDefault)
	_, classAa, _ = wrapParse(static.InnerIPClassAa, static.InnerIPClassDefault)
	_, classB, _  = wrapParse(static.InnerIPClassB, static.InnerIPClassDefault)
	_, classC, _  = wrapParse(static.InnerIPClassC, static.InnerIPClassDefault)
)

func wrapParse(expectip, defaultip string) (net.IP, *net.IPNet, error) {
	if expectip != "" {
		return net.ParseCIDR(expectip)
	}

	return net.ParseCIDR(defaultip)
}

// GetIPAddress get local usable inner ip address
func GetIPAddress() (addrList []string) {
	address, err := net.InterfaceAddrs()
	if err != nil {
		return addrList
	}
	for _, addr := range address {
		if ip, ok := addr.(*net.IPNet); ok && !ip.IP.IsLoopback() && ip.IP.To4() != nil {
			if classA.Contains(ip.IP) {
				addrList = append(addrList, ip.IP.String())
				continue
			}
			if classA1.Contains(ip.IP) {
				addrList = append(addrList, ip.IP.String())
				continue
			}
			if classAa.Contains(ip.IP) {
				addrList = append(addrList, ip.IP.String())
				continue
			}
			if classB.Contains(ip.IP) {
				addrList = append(addrList, ip.IP.String())
				continue
			}
			if classC.Contains(ip.IP) {
				addrList = append(addrList, ip.IP.String())
				continue
			}
		}
	}
	return addrList
}

// IsLocalIP check whether ip is local
func IsLocalIP(ip net.IP) bool {
	if classA.Contains(ip) {
		return true
	}
	if classA1.Contains(ip) {
		return true
	}
	if classAa.Contains(ip) {
		return true
	}
	if classB.Contains(ip) {
		return true
	}
	if classC.Contains(ip) {
		return true
	}

	return false
}
