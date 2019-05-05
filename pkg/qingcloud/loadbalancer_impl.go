package qingcloud

import (
	"context"
	"time"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/eip"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/executor"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/loadbalance"
	v1 "k8s.io/api/core/v1"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog"
)

var _ cloudprovider.LoadBalancer = &QingCloud{}

func (qc *QingCloud) newLoadBalance(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node, skipCheck bool) (*loadbalance.LoadBalancer, error) {
	lbExec := executor.NewQingCloudLoadBalanceExecutor(qc.lbService, qc.jobService)
	sgExec := executor.NewQingCloudSecurityGroupExecutor(qc.securityGroupService)
	eipHelper := eip.NewEIPHelperOfQingCloud(eip.NewEIPHelperOfQingCloudOption{
		JobAPI: qc.jobService,
		EIPAPI: qc.eipService,
		UserID: qc.userID,
	})
	opt := &loadbalance.NewLoadBalancerOption{
		LbExecutor:  lbExec,
		EipHelper:   eipHelper,
		SgExecutor:  sgExec,
		NodeLister:  qc.nodeInformer.Lister(),
		K8sNodes:    nodes,
		K8sService:  service,
		Context:     ctx,
		ClusterName: clusterName,
		SkipCheck:   skipCheck,
	}
	return loadbalance.NewLoadBalancer(opt)
}

// LoadBalancer returns an implementation of LoadBalancer for QingCloud.
func (qc *QingCloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	klog.V(4).Info("LoadBalancer() called")
	return qc, true
}

// GetLoadBalancer returns whether the specified load balancer exists, and
// if so, what its status is.
func (qc *QingCloud) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (status *v1.LoadBalancerStatus, exists bool, err error) {
	lb, err := qc.newLoadBalance(ctx, clusterName, service, nil, false)
	if err != nil {
		return nil, false, err
	}
	err = lb.GenerateK8sLoadBalancer()
	if err != nil {
		klog.Errorf("Failed to call 'GetLoadBalancer' of service %s", service.Name)
		return nil, false, err
	}
	if lb.Status.K8sLoadBalancerStatus == nil {
		return nil, false, nil
	}
	return lb.Status.K8sLoadBalancerStatus, true, nil
}

// GetLoadBalancerName returns the name of the load balancer. Implementations must treat the
// *v1.Service parameter as read-only and not modify it.
func (qc *QingCloud) GetLoadBalancerName(_ context.Context, clusterName string, service *v1.Service) string {
	return loadbalance.GetLoadBalancerName(clusterName, service)
}

// EnsureLoadBalancer creates a new load balancer 'name', or updates the existing one. Returns the status of the balancer
// Implementations must treat the *v1.Service and *v1.Node
// parameters as read-only and not modify them.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (qc *QingCloud) EnsureLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		klog.V(1).Infof("EnsureLoadBalancer takes total %d seconds", elapsed/time.Second)
	}()
	lb, err := qc.newLoadBalance(ctx, clusterName, service, nodes, false)
	if err != nil {
		return nil, err
	}
	err = lb.LoadQcLoadBalancer()
	if err != nil {
		if err == loadbalance.ErrorLBNotFoundInCloud {
			err = lb.CreateQingCloudLB()
			if err != nil {
				klog.Errorf("Failed to create lb in qingcloud of service %s", service.Name)
				return nil, err
			}
			return lb.Status.K8sLoadBalancerStatus, nil
		} else {
			return nil, err
		}
	}
	err = lb.UpdateQingCloudLB()
	if err != nil {
		klog.Errorf("Failed to update lb %s in qingcloud of service %s", lb.Name, service.Name)
		return nil, err
	}
	return lb.Status.K8sLoadBalancerStatus, nil
}

// UpdateLoadBalancer updates hosts under the specified load balancer.
// Implementations must treat the *v1.Service and *v1.Node
// parameters as read-only and not modify them.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (qc *QingCloud) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) error {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		klog.V(1).Infof("UpdateLoadBalancer takes total %d seconds", elapsed/time.Second)
	}()
	lb, err := qc.newLoadBalance(ctx, clusterName, service, nodes, false)
	if err != nil {
		return err
	}
	err = lb.LoadQcLoadBalancer()
	if err != nil {
		klog.Errorf("Failed to get lb %s in qingcloud of service %s", lb.Name, service.Name)
		return err
	}
	err = lb.LoadListeners()
	if err != nil {
		klog.Errorf("Failed to get listeners of lb %s of service %s", lb.Name, service.Name)
		return err
	}
	listeners := lb.GetListeners()
	for _, listener := range listeners {
		listener.UpdateQingCloudListener()
	}
	return nil
}

// EnsureLoadBalancerDeleted deletes the specified load balancer if it
// exists, returning nil if the load balancer specified either didn't exist or
// was successfully deleted.
// This construction is useful because many cloud providers' load balancers
// have multiple underlying components, meaning a Get could say that the LB
// doesn't exist even if some part of it is still laying around.
// Implementations must treat the *v1.Service parameter as read-only and not modify it.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (qc *QingCloud) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *v1.Service) error {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		klog.V(1).Infof("DeleteLoadBalancer takes total %d seconds", elapsed/time.Second)
	}()
	lb, _ := qc.newLoadBalance(ctx, clusterName, service, nil, true)
	return lb.DeleteQingCloudLB()
}
