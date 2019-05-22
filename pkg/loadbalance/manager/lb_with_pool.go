package manager

import (
	"fmt"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/executor"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/util"
	corev1 "k8s.io/api/core/v1"
)

var _ LoadBalancerManager = &lbWithPool{}

type lbWithPool struct {
	lbExec executor.QingCloudLoadBalancerExecutor
}

func (lb *lbWithPool) ValidateAnnotations(service *corev1.Service) error {
	annotation := service.GetAnnotations()
	if annotation == nil {
		return errors.NewFieldRequired("annotations")
	}
	if _, ok := annotation[ServiceAnnotationLoadBalancerType]; !ok {
		return errors.NewFieldInvalidValue(ServiceAnnotationLoadBalancerType)
	}
	if _, ok := annotation[ServiceAnnotationLoadBalancerEipIds]; ok {
		return errors.NewFieldInvalidValueWithReason(ServiceAnnotationLoadBalancerEipIds, "Cannot specify eip in pool mode")
	}
	if strategy, ok := annotation[ServiceAnnotationLoadBalancerEipStrategy]; ok {
		if strategy == string(ReuseEIP) {
			if _, ok := annotation[ServiceAnnotationLoadBalancerReuseGroup]; !ok {
				return errors.NewFieldRequired(ServiceAnnotationLoadBalancerReuseGroup)
			}
		} else if strategy != string(Exclusive) {
			return errors.NewFieldInvalidValueWithReason(ServiceAnnotationLoadBalancerEipStrategy, "only 'reuse' and 'exclusive' are valid inputs ")
		}
	}
	return nil
}

func (lb *lbWithPool) GetLoadBalancerName(clusterName string, service *corev1.Service) string {
	defaultName := fmt.Sprintf("k8s_lb_%s_%s_%s", clusterName, service.Name, util.GetFirstUID(string(service.UID)))
	annotation := service.GetAnnotations()
	if annotation == nil {
		return defaultName
	}
	if strategy, ok := annotation[ServiceAnnotationLoadBalancerEipStrategy]; ok {
		if strategy == string(ReuseEIP) {
			return fmt.Sprintf("k8s_lb_%s_%s", clusterName, annotation[ServiceAnnotationLoadBalancerReuseGroup])
		}
	}
	return defaultName
}

func NewLBManagerWithPool() LoadBalancerManager {
	return &lbWithPool{}
}
