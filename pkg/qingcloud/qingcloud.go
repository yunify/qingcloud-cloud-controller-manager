// Copyright 2017 Yunify Inc. All rights reserved.
// Use of this source code is governed by a Apache license
// that can be found in the LICENSE file.

package qingcloud

// Please see qingcloud document: https://docs.qingcloud.com/index.html
// and must pay attention to your account resource quota limit.

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/davecgh/go-spew/spew"
	yaml "gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	corev1informer "k8s.io/client-go/informers/core/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	cloudprovider "k8s.io/cloud-provider"
	klog "k8s.io/klog/v2"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/apis"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/executor"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/util"
)

const (
	ProviderName        = "qingcloud"
	QYConfigPath        = "/etc/qingcloud/config.yaml"
	DefaultBackendCount = 3
)

type Config struct {
	Zone              string   `yaml:"zone"`
	DefaultVxNetForLB string   `yaml:"defaultVxNetForLB,omitempty"`
	ClusterID         string   `yaml:"clusterID"`
	IsApp             bool     `yaml:"isApp,omitempty"`
	TagIDs            []string `yaml:"tagIDs,omitempty"`
	InstanceIDs       []string `yaml:"instanceIDs,omitempty"`
	PlaceGroupID      string   `yaml:"placeGroupID,omitempty"`
	SecurityGroupID   string   `yaml:"securityGroupID,omitempty"`
}

// A single Kubernetes cluster can run in multiple zones,
// but only within the same region (and cloud provider).

// QingCloud is the main entry of all interface
type QingCloud struct {
	Config *Config
	Client executor.QingCloudClientInterface

	nodeInformer     corev1informer.NodeInformer
	serviceInformer  corev1informer.ServiceInformer
	endpointInformer corev1informer.EndpointsInformer
	corev1interface  corev1.CoreV1Interface
}

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, func(config io.Reader) (cloudprovider.Interface, error) {
		return NewQingCloud(config)
	})
}

// NewQingCloud returns a new instance of QingCloud cloud provider.
func NewQingCloud(cfg io.Reader) (cloudprovider.Interface, error) {
	if cfg == nil {
		return nil, fmt.Errorf("no qingcloud provider Config file given")
	}

	var (
		config Config
	)
	err := yaml.NewDecoder(cfg).Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Config file, err=%v", err)
	}

	client, err := executor.NewQingCloudClient(&executor.ClientConfig{
		IsAPP:  config.IsApp,
		TagIDs: config.TagIDs,
	}, QYConfigPath)
	if err != nil {
		return nil, err
	}

	qc := QingCloud{
		Config: &config,
		Client: client,
	}

	return &qc, nil
}

func (qc *QingCloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
	clientset := clientBuilder.ClientOrDie("do-shared-informers")
	sharedInformer := informers.NewSharedInformerFactory(clientset, 0)
	nodeinformer := sharedInformer.Core().V1().Nodes()
	go nodeinformer.Informer().Run(stop)
	qc.nodeInformer = nodeinformer

	serviceInformer := sharedInformer.Core().V1().Services()
	go serviceInformer.Informer().Run(stop)
	qc.serviceInformer = serviceInformer

	endpointInformer := sharedInformer.Core().V1().Endpoints()
	go endpointInformer.Informer().Run(stop)
	qc.endpointInformer = endpointInformer

	qc.corev1interface = clientset.CoreV1()
}

func (qc *QingCloud) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

func (qc *QingCloud) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

func (qc *QingCloud) ProviderName() string {
	return ProviderName
}

func (qc *QingCloud) Instances() (cloudprovider.Instances, bool) {
	return nil, false
}

func (qc *QingCloud) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return nil, false
}

func (qc *QingCloud) Zones() (cloudprovider.Zones, bool) {
	return nil, false
}

func (qc *QingCloud) HasClusterID() bool {
	return qc.Config.ClusterID != ""
}

// LoadBalancer returns an implementation of LoadBalancer for QingCloud.
func (qc *QingCloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return qc, true
}

// GetLoadBalancerName returns the name of the load balancer. Implementations must treat the
// *v1.Service parameter as read-only and not modify it.
func (qc *QingCloud) GetLoadBalancerName(_ context.Context, _ string, service *v1.Service) string {
	return ""
}

// GetLoadBalancer returns whether the specified load balancer exists, and
// if so, what its status is.
func (qc *QingCloud) GetLoadBalancer(ctx context.Context, _ string, service *v1.Service) (status *v1.LoadBalancerStatus, exists bool, err error) {
	_, lb, err := qc.getLoadBalancerBeforeDelete(service)

	if errors.IsResourceNotFound(err) || lb == nil {
		return nil, false, nil
	}

	if err == nil {
		status = convertLoadBalancerStatus(&lb.Status)
		exists = true
	}

	return status, exists, err
}

func (qc *QingCloud) getLoadBalancer(service *v1.Service) (*LoadBalancerConfig, *apis.LoadBalancer, error) {
	var (
		lb   *apis.LoadBalancer
		err  error
		conf *LoadBalancerConfig
	)

	conf, err = qc.ParseServiceLBConfig(qc.Config.ClusterID, service)
	if err != nil {
		return nil, nil, err
	}

	if conf.ReuseLBID != "" {
		lb, err = qc.Client.GetLoadBalancerByID(conf.ReuseLBID)
	} else if conf.LoadBalancerName != "" {
		lb, err = qc.Client.GetLoadBalancerByName(conf.LoadBalancerName)
	} else {
		return nil, nil, fmt.Errorf("cannot found loadbalance id or name")
	}

	return conf, lb, err
}

// EnsureLoadBalancer creates a new load balancer 'name', or updates the existing one. Returns the status of the balancer
// Implementations must treat the *v1.Service and *v1.Node
// parameters as read-only and not modify them.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (qc *QingCloud) EnsureLoadBalancer(ctx context.Context, _ string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	var err error
	klog.Infof("====== EnsureLoadBalancer start handle service %s/%s ======", service.Namespace, service.Name)
	defer func() {
		if err != nil {
			klog.Errorf("EnsureLoadBalancer handle service %s/%s error: %v", service.Namespace, service.Name, err)
		}
		klog.Infof("====== EnsureLoadBalancer finished handle service %s/%s ======", service.Namespace, service.Name)
	}()
	lb, err := qc.ensureLoadBalancer(ctx, "", service, nodes)
	return lb, err
}
func (qc *QingCloud) ensureLoadBalancer(ctx context.Context, _ string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	conf, lb, err := qc.getLoadBalancer(service)
	klog.V(4).Infof("EnsureLoadBalancer lb %s config %s", spew.Sdump(lb), spew.Sdump(conf))
	if err != nil {
		if errors.IsResourceNotFound(err) && conf.Policy != ReuseExistingLB && conf.Policy != Shared {
			// will auto create lb with assigned eip or vxnet
			klog.Infof("lb not found for service %s/%s, going to create lb with assigned eip or vxnet ", service.Namespace, service.Name)
		} else {
			return nil, fmt.Errorf("getLoadBalancer error: %v", err)
		}
	}
	var lbID string

	// filter nodes by service externalTrafficPolicy
	nodes, e := qc.filterNodes(ctx, service, nodes, conf)
	if e != nil {
		return nil, fmt.Errorf("filterNodes with externalTrafficPolicy %s error: %v", service.Spec.ExternalTrafficPolicy, e)
	}

	//1. ensure & update lb
	if err == nil {
		//The configuration of the load balancer will be independent, so we'll just create
		//it for now, not update it.
		lbID = *lb.Status.LoadBalancerID
		//need modify attribute
		modify := false
		if result := needUpdateAttr(conf, lb); result != nil {
			if err = qc.Client.ModifyLB(result); err != nil {
				return nil, fmt.Errorf("modify lb %s error: %v", lbID, err)
			}
			modify = true
		}

		// update eips
		err = qc.updateLBEip(conf, lb)
		if err != nil {
			return nil, fmt.Errorf("update eip for lb %s error %v", lbID, err)
		}

		//update listener
		listenerIDs := filterListeners(lb.Status.LoadBalancerListeners, conf.listenerName)

		if len(listenerIDs) <= 0 {
			klog.Infof("listener is not exists for service %s/%s, creating listeners for lb %s with service ports %v", service.Namespace, service.Name, lbID, util.GetPortsSlice(service.Spec.Ports))
			if err = qc.createListenersAndBackends(conf, lb, service.Spec.Ports, nodes, service); err != nil {
				return nil, fmt.Errorf("createListenersAndBackends for lb %s error: %v", lbID, err)
			}
			modify = true
		} else {
			klog.Infof("the lb %s has the following listeners %s for service %s/%s", lbID, util.CoverPointSliceToStr(listenerIDs), service.Namespace, service.Name)
			listeners, err := qc.Client.GetListeners(listenerIDs)
			if err != nil {
				return nil, fmt.Errorf("get listeners %v error: %v", util.CoverPointSliceToStr(listenerIDs), err)
			}

			//update listerner
			toDelete, toAdd, toKeep := diffListeners(listeners, conf, service.Spec.Ports)
			if len(toDelete) > 0 {
				klog.Infof("listeners %s will be deleted for lb %s ", util.CoverPointSliceToStr(toDelete), lbID)
				err = qc.Client.DeleteListener(toDelete)
				if err != nil {
					return nil, fmt.Errorf("delete listeners %v error: %v", util.CoverPointSliceToStr(toDelete), err)
				}
				modify = true
			}

			if len(toAdd) > 0 {
				klog.Infof("listeners on ports %v will be added for lb %s", util.GetPortsSlice(toAdd), lbID)
				err = qc.createListenersAndBackends(conf, lb, toAdd, nodes, service)
				if err != nil {
					return nil, fmt.Errorf("createListenersAndBackends error: %v", err)
				}
				modify = true
			}

			//update backend; for example, service annotation for backend label changed
			for _, listener := range toKeep {
				listenerID := *listener.Spec.LoadBalancerListenerID
				listenerName := *listener.Spec.LoadBalancerListenerName
				toDelete, toAdd := qc.diffBackend(listener, nodes, conf, service)

				if len(toDelete) > 0 {
					klog.Infof("%d backends will be deleted for listener %s(id:%s,port:%d) of lb %s", len(toDelete), listenerName, listenerID, *listener.Spec.ListenerPort, lbID)
					klog.V(4).Infof("backends(count %d) %v will be deleted for listener %s(id:%s,port:%d) of lb %s", len(toDelete), util.CoverPointSliceToStr(toDelete), listenerName, listenerID, *listener.Spec.ListenerPort, lbID)
					err = qc.Client.DeleteBackends(toDelete)
					if err != nil {
						return nil, fmt.Errorf("delete backends %v error: %v", util.CoverPointSliceToStr(toDelete), err)
					}
					modify = true
				}

				if len(toAdd) > 0 {
					toAddBackends := generateLoadBalancerBackends(toAdd, listener, service.Spec.Ports)
					klog.Infof("%d backends will be added for listener %s(id:%s,port:%d) of lb %s", len(toAddBackends), listenerName, listenerID, *listener.Spec.ListenerPort, lbID)
					klog.V(4).Infof("backends(count %d) %v will be added for listener %s(id:%s,port:%d) of lb %s", len(toAdd), util.GetNodesName(toAdd), listenerName, listenerID, *listener.Spec.ListenerPort, lbID)

					_, err = qc.Client.CreateBackends(toAddBackends)
					if err != nil {
						return nil, fmt.Errorf("add backends error: %v", err)
					}
					modify = true
				}

			}

			if !modify {
				klog.Infof("no change for listeners %v of service %s/%s, skip UpdateLB %s", util.CoverPointSliceToStr(listenerIDs), service.Namespace, service.Name, lbID)
				return convertLoadBalancerStatus(&lb.Status), nil
			}
		}

	} else if errors.IsResourceNotFound(err) {
		//1. create lb
		//1.1 prepare eip
		if len(conf.EipIDs) <= 0 && conf.EipSource != nil {
			eip, err := qc.prepareEip(conf.EipSource)
			if err != nil {
				return nil, fmt.Errorf("prepare eip error: %v", err)
			}
			conf.EipIDs = []*string{eip.Status.EIPID}
		}
		//1.2 prepare sg
		//default sg set by Client auto
		//1.3 create lb
		klog.Infof("creating lb for service %s/%s", service.Namespace, service.Name)
		lb, err = qc.Client.CreateLB(&apis.LoadBalancer{
			Spec: apis.LoadBalancerSpec{
				LoadBalancerName: &conf.LoadBalancerName,
				LoadBalancerType: conf.LoadBalancerType,
				NodeCount:        conf.NodeCount,
				VxNetID:          conf.VxNetID,
				PrivateIPs:       []*string{conf.InternalIP},
				EIPs:             conf.EipIDs,
				PlaceGroupID:     conf.PlaceGroupID,
				SecurityGroups:   conf.SecurityGroupID,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("create lb error: %v", err)
		}

		lbID = *lb.Status.LoadBalancerID
		//create listener
		klog.Infof("creating listener %s on ports %v for lb %s", conf.listenerName, util.GetPortsSlice(service.Spec.Ports), lbID)
		if err = qc.createListenersAndBackends(conf, lb, service.Spec.Ports, nodes, service); err != nil {
			return nil, fmt.Errorf("create listener %s on ports %v for lb %s error: %v", conf.listenerName, util.GetPortsSlice(service.Spec.Ports), lbID, err)
		}
	} else {
		return nil, err
	}

	if len(lb.Status.VIP) <= 0 {
		return nil, fmt.Errorf("loadbalance has no vip, please spec it")
	}

	err = qc.Client.UpdateLB(lb.Status.LoadBalancerID)
	if err != nil {
		return nil, fmt.Errorf("update lb %s error: %v", lbID, err)
	}
	klog.Infof("update lb %s success for service %s/%s", lbID, service.Namespace, service.Name)

	return convertLoadBalancerStatus(&lb.Status), nil
}

// UpdateLoadBalancer updates hosts under the specified load balancer.
// Implementations must treat the *v1.Service and *v1.Node
// parameters as read-only and not modify them.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (qc *QingCloud) UpdateLoadBalancer(ctx context.Context, _ string, service *v1.Service, nodes []*v1.Node) error {
	var err error
	klog.Infof("====== UpdateLoadBalancer start handle service %s/%s ======", service.Namespace, service.Name)
	defer func() {
		if err != nil {
			klog.Errorf("UpdateLoadBalancer handle service %s/%s error: %v", service.Namespace, service.Name, err)
		}
		klog.Infof("====== UpdateLoadBalancer finished handle service %s/%s ======", service.Namespace, service.Name)
	}()
	err = qc.updateLoadBalancer(ctx, "", service, nodes)
	return err
}
func (qc *QingCloud) updateLoadBalancer(ctx context.Context, _ string, service *v1.Service, nodes []*v1.Node) error {

	var modify bool

	conf, lb, err := qc.getLoadBalancer(service)
	klog.V(4).Infof("UpdateLoadBalancer lb %s config %s", spew.Sdump(lb), spew.Sdump(conf))
	if err != nil {
		return fmt.Errorf("getLoadBalancer error: %v", err)
	}
	lbID := *lb.Status.LoadBalancerID

	nodes, err = qc.filterNodes(ctx, service, nodes, conf)
	if err != nil {
		return fmt.Errorf("filterNodes with externalTrafficPolicy %s error: %v", service.Spec.ExternalTrafficPolicy, err)
	}

	//update backend
	listenerIDs := filterListeners(lb.Status.LoadBalancerListeners, conf.listenerName)
	if len(listenerIDs) <= 0 {
		klog.Infof("listener is not exists for service %s/%s, skip update lb %s", service.Namespace, service.Name, lbID)
		return nil
	}

	listeners, err := qc.Client.GetListeners(listenerIDs)
	if err != nil {
		return fmt.Errorf("get listeners %v error: %v", util.CoverPointSliceToStr(listenerIDs), err)
	}

	for _, listener := range listeners {
		// toDelete, toAdd := diffBackend(listener, nodes)
		toDelete, toAdd := qc.diffBackend(listener, nodes, conf, service)

		if len(toDelete) > 0 {
			klog.Infof("%d backends will be deleted for listener %s(id:%s,port:%d) of lb %s", len(toDelete), *listener.Spec.LoadBalancerListenerName, *listener.Spec.LoadBalancerListenerID, *listener.Spec.ListenerPort, lbID)
			klog.V(4).Infof("backends(count %d) %v will be deleted for listener %s(id:%s,port:%d) of lb %s", len(toDelete), util.CoverPointSliceToStr(toDelete), *listener.Spec.LoadBalancerListenerName, *listener.Spec.LoadBalancerListenerID, *listener.Spec.ListenerPort, lbID)
			err = qc.Client.DeleteBackends(toDelete)
			if err != nil {
				return fmt.Errorf("delete backends %v error: %v", util.CoverPointSliceToStr(toDelete), err)
			}
			modify = true
		}

		toAddBackends := generateLoadBalancerBackends(toAdd, listener, service.Spec.Ports)
		if len(toAddBackends) > 0 {
			klog.Infof("%d backends will be added for listener %s(id:%s,port:%d) of lb %s", len(toAdd), *listener.Spec.LoadBalancerListenerName, *listener.Spec.LoadBalancerListenerID, *listener.Spec.ListenerPort, lbID)
			klog.V(4).Infof("backends(count %d) %s will be added for listener %s(id:%s,port:%d) of lb %s", len(toAdd), util.GetNodesName(toAdd), *listener.Spec.LoadBalancerListenerName, *listener.Spec.LoadBalancerListenerID, *listener.Spec.ListenerPort, lbID)
			_, err = qc.Client.CreateBackends(toAddBackends)
			if err != nil {
				return fmt.Errorf("add backends error: %v", err)
			}
			modify = true
		}
	}

	if !modify {
		klog.Infof("no backend change for listeners %v of service %s/%s, skip UpdateLB %s", util.CoverPointSliceToStr(listenerIDs), service.Namespace, service.Name, lbID)
		return nil
	}

	err = qc.Client.UpdateLB(lb.Status.LoadBalancerID)
	if err != nil {
		return fmt.Errorf("update lb %s error: %v", lbID, err)
	}
	klog.Infof("update lb %s success for service %s/%s", lbID, service.Namespace, service.Name)
	return nil
}

func (qc *QingCloud) createListenersAndBackends(conf *LoadBalancerConfig, status *apis.LoadBalancer, ports []v1.ServicePort, nodes []*v1.Node, svc *v1.Service) error {
	listeners, err := generateLoadBalancerListeners(conf, status, ports)
	if err != nil {
		return fmt.Errorf("generateLoadBalancerListeners for loadbalancer %s error: %v", *status.Status.LoadBalancerID, err)
	}
	listeners, err = qc.Client.CreateListener(listeners)
	if err != nil {
		return fmt.Errorf("CreateListener for loadbalancer %s error: %v", *status.Status.LoadBalancerID, err)
	}

	// filter backend nodes by count config
	if svc.Spec.ExternalTrafficPolicy == v1.ServiceExternalTrafficPolicyTypeCluster && conf.BackendCountResult != 0 {
		klog.Infof("try to get %d random nodes as backend for service %s/%s on ports %v", conf.BackendCountResult, svc.Namespace, svc.Name, util.GetPortsSlice(ports))
		nodes = getRandomNodes(nodes, conf.BackendCountResult)

		var resultNames []string
		for _, node := range nodes {
			resultNames = append(resultNames, node.Name)
		}
		klog.V(4).Infof("get random nodes result for service %s/%s: %v", svc.Namespace, svc.Name, resultNames)
	}

	// create backend
	for _, listener := range listeners {
		backends := generateLoadBalancerBackends(nodes, listener, ports)
		_, err = qc.Client.CreateBackends(backends)
		if err != nil {
			return fmt.Errorf("CreateBackends for loadbalancer %s error: %v", *status.Status.LoadBalancerID, err)
		}

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
func (qc *QingCloud) EnsureLoadBalancerDeleted(ctx context.Context, _ string, service *v1.Service) error {
	var err error
	klog.Infof("====== EnsureLoadBalancerDeleted start handle service %s/%s ======", service.Namespace, service.Name)
	defer func() {
		if err != nil {
			klog.Errorf("EnsureLoadBalancerDeleted handle service %s/%s error: %v", service.Namespace, service.Name, err)
		}
		klog.Infof("====== EnsureLoadBalancerDeleted finished handle service %s/%s ======", service.Namespace, service.Name)
	}()

	err = qc.ensureLoadBalancerDeleted(ctx, "", service)
	return err
}
func (qc *QingCloud) ensureLoadBalancerDeleted(ctx context.Context, _ string, service *v1.Service) error {
	lbConfig, lb, err := qc.getLoadBalancerBeforeDelete(service)
	klog.V(4).Infof("EnsureLoadBalancerDeleted lb %s config %s", spew.Sdump(lb), spew.Sdump(lbConfig))
	if err != nil {
		if errors.IsResourceNotFound(err) {
			klog.Infof("not found lb for service %s/%s, skip delete lb", service.Namespace, service.Name)
			return nil
		} else {
			return fmt.Errorf("get lb error: %v", err)
		}
	}

	if lb == nil {
		klog.Infof("there is no lb or eip annotation of service %s/%s, skip delete lb", service.Namespace, service.Name)
		return nil
	}

	lbID := *lb.Status.LoadBalancerID
	listeners := filterListeners(lb.Status.LoadBalancerListeners, lbConfig.listenerName)
	//reuse lb or  auto create lb but has other listeners, only delete listener for this service
	if lbConfig.ReuseLBID != "" || len(lb.Status.LoadBalancerListeners)-len(listeners) > 0 {
		klog.Infof("service %s/%s reuse lb %s or lb has other listeners, try to delete listener %s", service.Namespace, service.Name, lbID, lbConfig.listenerName)
		if len(listeners) <= 0 {
			klog.Infof("listener is not exists for service %s/%s on lb %s, skip delete listener", service.Namespace, service.Name, lbID)
			return nil
		}

		err = qc.Client.DeleteListener(listeners)
		if err != nil {
			return fmt.Errorf("delete listener %v error: %v", util.CoverPointSliceToStr(listeners), err)
		}
		klog.Infof("delete listener %v success for service %s/%s ", util.CoverPointSliceToStr(listeners), service.Namespace, service.Name)

		err = qc.Client.UpdateLB(lb.Status.LoadBalancerID)
		if err != nil {
			return fmt.Errorf("update lb %s error: %v", lbID, err)
		}
		klog.Infof("update lb %s success for service %s/%s", lbID, service.Namespace, service.Name)
		return nil
	}

	//delete lb
	err = qc.Client.DeleteLB(lb.Status.LoadBalancerID)
	if err != nil {
		return fmt.Errorf("delete lb %s error: %v", *lb.Status.LoadBalancerID, err)

	}
	klog.Infof("delete lb %s success for service %s/%s", lbID, service.Namespace, service.Name)

	//delete eip
	if len(lb.Status.CreatedEIPs) > 0 {
		err = qc.Client.ReleaseEIP(lb.Status.CreatedEIPs)
		if err != nil {
			return fmt.Errorf("release eip %v error: %v", util.CoverPointSliceToStr(lb.Status.CreatedEIPs), err)
		}
		klog.Infof("delete eips %v success for service %s/%s", util.CoverPointSliceToStr(lb.Status.CreatedEIPs), service.Namespace, service.Name)
	}

	//delete sg

	return nil
}

// filterNodes filter nodes by service externalTrafficPolicy and service backend label annotation
func (qc *QingCloud) filterNodes(ctx context.Context, svc *v1.Service, nodes []*v1.Node, lbconfog *LoadBalancerConfig) (newNodes []*v1.Node, err error) {

	if svc.Spec.ExternalTrafficPolicy == v1.ServiceExternalTrafficPolicyTypeLocal {
		klog.Infof("filter nodes for service %s/%s by real worker nodes", svc.Namespace, svc.Name)

		// 1. get endpoint to get real worker node
		nodeExists := make(map[string]bool)
		ep, err := qc.endpointInformer.Lister().Endpoints(svc.Namespace).Get(svc.Name)
		if err != nil {
			return nil, fmt.Errorf("get endpoint %s/%s error: %v", svc.Namespace, svc.Name, err)
		}
		for _, sub := range ep.Subsets {
			for _, addr := range sub.Addresses {
				nodeExists[*addr.NodeName] = true
			}
		}

		// 2. filter node
		for _, node := range nodes {
			if nodeExists[node.Name] {
				newNodes = append(newNodes, node)
			}
		}
	} else {
		var backendCountResult int
		if lbconfog.BackendLabel != "" { // filter by node label
			klog.Infof("filter nodes for service %s/%s by backend label: %s", svc.Namespace, svc.Name, lbconfog.BackendLabel)

			// filter by label
			labelMap, err := parseBackendLabel(lbconfog)
			if err != nil {
				return nil, fmt.Errorf("parseBackendLabel error: %v", err)
			}

			for i, node := range nodes {
				count := 0
				for k, v := range labelMap {
					l, ok := node.Labels[k]
					if ok && (l == v) {
						count++
					} else {
						break
					}
				}
				if count == len(labelMap) {
					newNodes = append(newNodes, nodes[i])
				}
			}
			// if there are no available nodes , use default backend count value
			if len(newNodes) == 0 {
				backendCountResult = getDefaultBackendCount(nodes)
				klog.Infof("there are no available nodes filter by label %s for service %s/%s, use default backend count: %d",
					lbconfog.BackendLabel, svc.Namespace, svc.Name, backendCountResult)
			}

		} else if lbconfog.BackendCountConfig != "" { //filter by backend count config
			klog.Infof("filter nodes for service %s/%s by backend count config: %s", svc.Namespace, svc.Name, lbconfog.BackendCountConfig)

			backendCountConfig, _ := strconv.Atoi(lbconfog.BackendCountConfig)
			if backendCountConfig > 0 && backendCountConfig <= len(nodes) {
				backendCountResult = backendCountConfig
			} else {
				backendCountResult = getDefaultBackendCount(nodes)
				klog.Infof("invalid backend count config %s for service %s/%s, use default backend count: %d",
					lbconfog.BackendCountConfig, svc.Namespace, svc.Name, backendCountResult)
			}
		} else {
			// no need to filter or for other filter policy in the future
		}

		if len(newNodes) == 0 {
			//no need to filter, use all nodes in params
			newNodes = nodes
		}
		lbconfog.BackendCountResult = backendCountResult
	}

	var resultNames []string
	for _, node := range newNodes {
		resultNames = append(resultNames, node.Name)
	}
	klog.V(4).Infof("filter node result for service %s/%s: %v", svc.Namespace, svc.Name, resultNames)

	return newNodes, nil
}
