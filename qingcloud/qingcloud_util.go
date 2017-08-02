package qingcloud

import (
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/pkg/api/v1"
)

const (
	waitInterval         = 10 * time.Second
	operationWaitTimeout = 180 * time.Second
	pageLimt             = 100
)

// Make sure qingcloud instance hostname or override-hostname (if provided) is equal to InstanceId
// Recommended to use override-hostname
func NodeNameToInstanceID(name types.NodeName) string {
	return string(name)
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

func stringPtr(str string) *string {
	return &str
}

func intPtr(i int) *int {
	return &i
}

func stringArrayPtr(strs []string) []*string{
	results := make([]*string, len(strs))
	for i :=0; i < len(strs); i++ {
		results[i] = &strs[i]
	}
	return results
}