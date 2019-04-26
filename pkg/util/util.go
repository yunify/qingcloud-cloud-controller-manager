// Copyright 2017 Yunify Inc. All rights reserved.
// Use of this source code is governed by a Apache license
// that can be found in the LICENSE file.

package util

import (
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

const (
	waitInterval         = 10 * time.Second
	operationWaitTimeout = 180 * time.Second
	pageLimt             = 100
)

// GetPortsOfService return service ports and nodeports
func GetPortsOfService(service *v1.Service) ([]int, []int) {
	k8sTCPPorts := []int{}
	k8sNodePorts := []int{}
	for _, port := range service.Spec.Ports {
		if port.Protocol == v1.ProtocolUDP {
			klog.Warningf("qingcloud not support udp port, skip [%v]", port.Port)
		} else {
			k8sTCPPorts = append(k8sTCPPorts, int(port.Port))
			k8sNodePorts = append(k8sNodePorts, int(port.NodePort))
		}
	}
	return k8sTCPPorts, k8sNodePorts
}

func GetNodePort(service *v1.Service, port int32, protocol v1.Protocol) (nodePort int32, found bool) {
	if service == nil {
		return
	}

	for _, servicePort := range service.Spec.Ports {
		if servicePort.Port == port && servicePort.Protocol == protocol {
			nodePort = servicePort.NodePort
			found = true
			return
		}
	}

	return
}

func StringIndex(vs []string, t string) int {
	for i, v := range vs {
		if v == t {
			return i
		}
	}
	return -1
}

func IntIndex(vs []int, t int) int {
	for i, v := range vs {
		if v == t {
			return i
		}
	}
	return -1
}

func GetFirstUID(uid string) string {
	s := strings.Split(uid, "-")
	return s[0]
}

func TwoArrayEqual(a []int, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	for _, n := range b {
		if IntIndex(b, n) == -1 {
			return false
		}
	}
	return true
}
