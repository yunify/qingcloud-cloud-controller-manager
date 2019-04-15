// Copyright 2017 Yunify Inc. All rights reserved.
// Use of this source code is governed by a Apache license
// that can be found in the LICENSE file.

package qingcloud

import (
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
)

const (
	waitInterval         = 10 * time.Second
	operationWaitTimeout = 180 * time.Second
	pageLimt             = 100
)

// Make sure qingcloud instance hostname or override-hostname (if provided) is equal to InstanceId
// Recommended to use override-hostname
func (qc *QingCloud) NodeNameToInstanceID(name types.NodeName) string {
	node, err := qc.k8sclient.CoreV1().Nodes().Get(string(name), metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get instance id of node %s, err:", name, err.Error())
		return ""
	}
	if instanceid, ok := node.GetAnnotations()[NodeAnnotationInstanceID]; ok {
		return instanceid
	}
	return node.Name
}

func getNodePort(service *v1.Service, port int32, protocol v1.Protocol) (nodePort int32, found bool) {
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

func stringIndex(vs []string, t string) int {
	for i, v := range vs {
		if v == t {
			return i
		}
	}
	return -1
}

func intIndex(vs []int, t int) int {
	for i, v := range vs {
		if v == t {
			return i
		}
	}
	return -1
}
