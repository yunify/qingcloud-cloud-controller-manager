// Copyright 2017 Yunify Inc. All rights reserved.
// Use of this source code is governed by a Apache license
// that can be found in the LICENSE file.

package util

import (
	"crypto/rand"
	"math/big"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
)

const (
	waitInterval         = 10 * time.Second
	operationWaitTimeout = 180 * time.Second
	pageLimt             = 100
)

// GetSvcPortsAndNodePorts return service ports and nodeports
func GetSvcPortsAndNodePorts(service *v1.Service) ([]int, []int) {
	k8sPorts := []int{}
	k8sNodePorts := []int{}
	for _, port := range service.Spec.Ports {
		k8sPorts = append(k8sPorts, int(port.Port))
		k8sNodePorts = append(k8sNodePorts, int(port.NodePort))
	}
	return k8sPorts, k8sNodePorts
}

func GetSvcPorts(service *v1.Service) []int {
	k8sPorts := []int{}
	for _, port := range service.Spec.Ports {
		k8sPorts = append(k8sPorts, int(port.Port))
	}
	return k8sPorts
}

func GetPortsSlice(ports []v1.ServicePort) []int {
	k8sPorts := []int{}
	for _, port := range ports {
		k8sPorts = append(k8sPorts, int(port.Port))
	}
	return k8sPorts
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

func GetRandomItems(items []*string, count int) (result []*string) {
	resultMap := make(map[int64]bool)
	length := int64(len(items))

	for i := 0; i < count; {
		r, _ := rand.Int(rand.Reader, big.NewInt(length))
		if !resultMap[r.Int64()] {
			result = append(result, items[r.Int64()])
			resultMap[r.Int64()] = true
			i++
		}
	}
	return
}

func CoverPointSliceToStr[T string | int](ps []*T) (result []T) {
	for _, v := range ps {
		if v != nil {
			result = append(result, *v)
		}
	}
	return
}

func GetNodesName(nodes []*v1.Node) (names []string) {
	for _, node := range nodes {
		if node != nil {
			names = append(names, node.Name)
		}
	}
	return
}
