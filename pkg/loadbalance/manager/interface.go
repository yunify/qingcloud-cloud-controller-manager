package manager

import (
	"strconv"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/loadbalance/annotations"
	corev1 "k8s.io/api/core/v1"
)

type LoadBalancerManager interface {
	ValidateAnnotations(service *corev1.Service) error
	GetLoadBalancerName(clusterName string, service *corev1.Service) string
}

func sharedValidateMethod(annotation map[string]string) error {
	if annotation == nil {
		return errors.NewFieldRequired("annotations")
	}
	if lbType, ok := annotation[annotations.ServiceAnnotationLoadBalancerType]; !ok {
		return errors.NewFieldInvalidValue(annotations.ServiceAnnotationLoadBalancerType)
	} else {
		t, err := strconv.Atoi(lbType)
		if err != nil || (t > 3 || t < 0) {
			return errors.NewFieldInvalidValueWithReason(annotations.ServiceAnnotationLoadBalancerType, "Pls spec a valid value of loadBalancer for service, acceptable values are '0-3'")
		}
	}
	return nil
}
