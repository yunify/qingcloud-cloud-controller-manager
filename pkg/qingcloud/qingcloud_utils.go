package qingcloud

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/apis"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/util"
)

func (qc *QingCloud) prepareEip(eipSource *string) (eip *apis.EIP, err error) {

	switch *eipSource {
	case AllocateOnly:
		eip, err = qc.Client.AllocateEIP(nil)
	case UseAvailableOnly:
		eips, err := qc.Client.GetAvaliableEIPs()
		if err != nil {
			return nil, err
		}

		if len(eips) <= 0 {
			return nil, fmt.Errorf("no avaliable eips")
		}

		eip = eips[0]
	case UseAvailableOrAllocateOne:
		eips, err := qc.Client.GetAvaliableEIPs()
		if err != nil {
			return nil, err
		}

		if len(eips) <= 0 {
			eip, err = qc.Client.AllocateEIP(nil)
			if err != nil {
				return nil, err
			}
		} else {
			eip = eips[0]
		}
	}

	if err != nil {
		return nil, err
	} else if eip == nil {
		return nil, fmt.Errorf("has no eip")
	}
	return eip, nil
}

func (qc *QingCloud) diffLBEip(config *LoadBalancerConfig, lb *apis.LoadBalancer) (eipsToAdd, eipsToDel []*string, err error) {

	// lb eip
	lbEipMap := make(map[string]bool)
	if lb.Spec.EIPs != nil {
		for _, lbEipID := range lb.Spec.EIPs {
			lbEipMap[*lbEipID] = true
		}
	}

	// eip/internal --> eip;
	if config.EipIDs != nil {
		// config eip
		configEipMap := make(map[string]bool)
		for _, configEipID := range config.EipIDs {
			configEipMap[*configEipID] = true
			if !lbEipMap[*configEipID] {
				eipsToAdd = append(eipsToAdd, configEipID)
			}
		}

		if config.EipReplace {
			for _, lbEipID := range lb.Spec.EIPs {
				if !configEipMap[*lbEipID] {
					eipsToDel = append(eipsToDel, lbEipID)
				}
			}
		}
	} else if config.EipSource != nil {
		switch *config.EipSource {
		case AllocateOnly, UseAvailableOnly, UseAvailableOrAllocateOne:
			if len(lb.Spec.EIPs) > 0 {
				// lb already has eip, do nothing
				klog.Infof("lb %s already has eip %s, do nothing", *lb.Status.LoadBalancerID, spew.Sdump(lb.Spec.EIPs))
			} else {
				// get or create an available eip from qingcloud and associate this eip to lb
				eip, err := qc.prepareEip(config.EipSource)
				if err != nil {
					return nil, nil, fmt.Errorf("prepare eip for lb %s error: %v", *lb.Status.LoadBalancerID, err)
				}
				eipsToAdd = append(eipsToAdd, eip.Status.EIPID)
			}
		default: // annotation value not correct, do nothing
			return nil, nil, fmt.Errorf("the value of annotation '%s' is mistake", ServiceAnnotationLoadBalancerEipSource)
		}
	} else if config.NetworkType == NetworkModeInternal { // eip/internal --> intertal
		// delete all eip from this lb
		if lb.Spec.EIPs != nil {
			eipsToDel = append(eipsToDel, lb.Spec.EIPs...)
		}
	}
	return
}

func (qc *QingCloud) updateLBEip(config *LoadBalancerConfig, lb *apis.LoadBalancer) (err error) {
	var updated bool
	var eipsToAdd, eipsToDel []*string
	// if reuse lb, do nothing
	if config.ReuseLBID != "" {
		return nil
	}

	eipsToAdd, eipsToDel, err = qc.diffLBEip(config, lb)
	if err != nil {
		return err
	}

	// update lb eip
	if len(eipsToAdd) > 0 {
		klog.Infof("associating eips %s to lb %s", spew.Sdump(eipsToAdd), *lb.Status.LoadBalancerID)
		err = qc.Client.AssociateEIPsToLB(lb.Status.LoadBalancerID, eipsToAdd)
		if err != nil {
			return err
		}
		updated = true
	}
	if len(eipsToDel) > 0 {
		klog.Infof("dissociating eips %s from lb %s", spew.Sdump(eipsToDel), *lb.Status.LoadBalancerID)
		err = qc.Client.DissociateEIPsFromLB(lb.Status.LoadBalancerID, eipsToDel)
		if err != nil {
			return err
		}
		updated = true
	}

	// update lb status
	if updated {
		lbNew, err := qc.Client.GetLoadBalancerByName(config.LoadBalancerName)
		if err != nil {
			return fmt.Errorf("get loadbalancer by name error: %v", err)
		}
		lb.Status = lbNew.Status
	}

	return nil
}

func (qc *QingCloud) diffBackend(listener *apis.LoadBalancerListener, nodes []*v1.Node, conf *LoadBalancerConfig, svc *v1.Service) (toDelete []*string, toAdd []*v1.Node) {
	var backendLeftID []*string
	for _, backend := range listener.Status.LoadBalancerBackends {
		if !nodesHasBackend(*backend.Spec.LoadBalancerBackendName, nodes) {
			toDelete = append(toDelete, backend.Status.LoadBalancerBackendID)
		} else {
			backendLeftID = append(backendLeftID, backend.Status.LoadBalancerBackendID)
		}
	}

	// filter backend nodes by count config
	if svc.Spec.ExternalTrafficPolicy == v1.ServiceExternalTrafficPolicyTypeCluster && conf.BackendCountResult != 0 {
		backendLeftCount := len(listener.Status.LoadBalancerBackends) - len(toDelete)
		if backendLeftCount > conf.BackendCountResult {
			// delete some
			toDelete = append(toDelete, util.GetRandomItems(backendLeftID, backendLeftCount-conf.BackendCountResult)...)
		} else {
			// add some
			var nodeLeft []*v1.Node
			for _, node := range nodes {
				if !backendsHasNode(node, listener.Status.LoadBalancerBackends) {
					nodeLeft = append(nodeLeft, node)
				}
			}
			toAdd = append(toAdd, getRandomNodes(nodeLeft, conf.BackendCountResult-backendLeftCount)...)
		}
	} else {
		for _, node := range nodes {
			if !backendsHasNode(node, listener.Status.LoadBalancerBackends) {
				toAdd = append(toAdd, node)
			}
		}
	}

	return
}

// this method only used to get lb before delete lb(service type changed: Loadbalancer -> ClusterIP/NodePort)
// new service may deleted lb annotation, if so,  not return error,
// annother controller(cloud-service-controller) will deleted the lb according to the last version service annotation
func (qc *QingCloud) getLoadBalancerBeforeDelete(svc *v1.Service) (conf *LoadBalancerConfig, lb *apis.LoadBalancer, err error) {

	conf = qc.parseServiceLBAndEIP(svc)
	if conf == nil {
		return nil, nil, nil
	}

	if conf.ReuseLBID != "" {
		lb, err = qc.Client.GetLoadBalancerByID(conf.ReuseLBID)
	} else if conf.LoadBalancerName != "" {
		lb, err = qc.Client.GetLoadBalancerByName(conf.LoadBalancerName)
	} else {
		return conf, nil, fmt.Errorf("cannot found loadbalance id or name")
	}
	return conf, lb, err
}

// only used to get lb and eip config, return nil if has no lb and eip annotation
// we think the service auto create lb as default; but if the service config reuse lb id, we use lb id first
func (qc *QingCloud) parseServiceLBAndEIP(svc *v1.Service) (conf *LoadBalancerConfig) {
	conf = new(LoadBalancerConfig)

	conf.listenerName = fmt.Sprintf("listener_%s_%s_", svc.Namespace, svc.Name)
	conf.sgName = conf.LoadBalancerName
	conf.LoadBalancerName = fmt.Sprintf("k8s_lb_%s_%s_%s_%s", qc.Config.ClusterID, svc.Namespace, svc.Name, util.GetFirstUID(string(svc.UID)))
	if len(svc.Annotations) > 0 {
		if svc.Annotations[ServiceAnnotationLoadBalancerPolicy] == ReuseExistingLB {
			conf.ReuseLBID = svc.Annotations[ServiceAnnotationLoadBalancerID]
		}
	}
	return
}
