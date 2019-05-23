package manager

import (
	"fmt"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	. "github.com/yunify/qingcloud-cloud-controller-manager/pkg/loadbalance/annotations"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/util"
	corev1 "k8s.io/api/core/v1"
)

type lbWithoutPool struct {
}

func NewLBManagerWithoutPool() LoadBalancerManager {
	return &lbWithoutPool{}
}

func (lb *lbWithoutPool) ValidateAnnotations(service *corev1.Service) error {
	annotation := service.GetAnnotations()
	if err := sharedValidateMethod(annotation); err != nil {
		return err
	}
	if strategy, ok := annotation[ServiceAnnotationLoadBalancerEipStrategy]; ok {
		if strategy == string(ReuseEIP) {
			if _, ok := annotation[ServiceAnnotationLoadBalancerEipIds]; !ok {
				return errors.NewFieldRequired(ServiceAnnotationLoadBalancerEipIds)
			}
		} else if strategy != string(Exclusive) {
			return errors.NewFieldInvalidValueWithReason(ServiceAnnotationLoadBalancerEipStrategy, "only 'reuse' and 'exclusive' are valid inputs ")
		}
	}
	return nil
}

func (lb *lbWithoutPool) GetLoadBalancerName(clusterName string, service *corev1.Service) string {
	defaultName := fmt.Sprintf("k8s_lb_%s_%s_%s", clusterName, service.Name, util.GetFirstUID(string(service.UID)))
	annotation := service.GetAnnotations()
	if annotation == nil {
		return defaultName
	}
	if strategy, ok := annotation[ServiceAnnotationLoadBalancerEipStrategy]; ok {
		if strategy == string(ReuseEIP) {
			return fmt.Sprintf("k8s_lb_%s_%s", clusterName, annotation[ServiceAnnotationLoadBalancerEipIds])
		}
	}
	return defaultName
}
